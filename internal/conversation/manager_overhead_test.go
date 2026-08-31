package conversation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// overheadFixture builds a warm-CLI-shaped session: alternating turns where every
// assistant message carries a persisted tool trace. Returns the store, the agent,
// the session and the history.
func overheadFixture(t *testing.T, turns int, trace string) (*db.DB, db.Agent, db.Session, []db.Message) {
	t.Helper()
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agentRow, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	history := make([]db.Message, 0, turns*2)
	for i := 0; i < turns; i++ {
		history = append(history,
			db.Message{Role: providers.RoleUser, Text: strings.Repeat("question ", 40)},
			db.Message{Role: providers.RoleAssistant, Text: strings.Repeat("answer ", 40), Steps: trace})
	}
	return d, agentRow, sess, history
}

// TestPrepareDropsFoldedStepsFromReportedOverhead is the SES2153 regression: the
// non-message overhead is measured BEFORE the fold and includes the persisted
// Steps trace of every pending message. Reporting the post-fold footprint with
// that same figure charged the folded messages twice over — once inside the
// summary they were compressed into, once as a trace that is no longer sent. On
// SES2153 that printed after=100201 against a 70000 budget (143%) where the real
// figure was 24778, and the same stale number drove Pressure.
//
// The expected number here is computed from the fixture's own parts (system
// fillers + the trace that SURVIVES the fold), never from Prepare's output.
func TestPrepareDropsFoldedStepsFromReportedOverhead(t *testing.T) {
	ctx := context.Background()
	trace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 300) + `"}]`
	d, agentRow, sess, history := overheadFixture(t, 6, trace)

	const keepRecent = 4
	const systemFillers = 1500 // the non-message, non-trace part of the overhead

	allSteps, err := EstimatePersistedStepTokens(history)
	if err != nil {
		t.Fatalf("estimate persisted: %v", err)
	}
	overhead := systemFillers + allSteps

	m := NewManager()
	// Budget below the true footprint so the fold fires, above the visible text so
	// only the overhead term can trip it.
	configured := EstimateTokens("", history) + allSteps/2
	m.SetLimits(configured, keepRecent)
	fraction, ceil := m.budgetShape()
	budget := EffectiveBudget(agentRow.Provider, agentRow.Model, configured, fraction, ceil)

	turnCtx := WithContextOverheadStepBase(WithContextOverhead(ctx, overhead), 0)
	prep, err := m.Prepare(turnCtx, d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prep.Compacted {
		t.Fatalf("fixture did not fold (overhead=%d budget=%d)", overhead, budget)
	}

	// What the fold left behind: the summary + the kept tail, the system fillers,
	// and ONLY the trace of the messages still on the wire.
	kept := history[len(history)-keepRecent:]
	keptSteps, err := EstimatePersistedStepTokens(kept)
	if err != nil {
		t.Fatalf("estimate kept: %v", err)
	}
	wantAfter := EstimateTokens("ROLLED UP", kept) + systemFillers + keptSteps
	if prep.Fold.AfterTokens != wantAfter {
		t.Fatalf("AfterTokens = %d, want %d (folded steps still charged: %d)",
			prep.Fold.AfterTokens, wantAfter, allSteps-keptSteps)
	}
	if prep.Fold.AfterTokens >= prep.Fold.BeforeTokens {
		t.Fatalf("fold did not shrink the footprint: %d → %d", prep.Fold.BeforeTokens, prep.Fold.AfterTokens)
	}

	// Pressure must be computed from the corrected footprint, not the stale one.
	wantPressure := float64(prep.ContextTokens+systemFillers+keptSteps) / float64(budget)
	if prep.Pressure != wantPressure {
		t.Fatalf("pressure = %f, want %f", prep.Pressure, wantPressure)
	}
}

// TestPrepareReportsBothSidesOnOneFormula pins the reporting contract (TSK602):
// the fold's before/after pair must be produced by a SINGLE formula —
// EstimateTokens(summary, messages) plus the same non-message overhead the gate
// budgeted against — so the printed "X→Y tokens" ratio actually describes what the
// fold removed. A mismatched basis (message-only on one side, messages+overhead on
// the other) made a real session journal "75989→67182" while the post-fold payload
// was a 3.5k summary plus a short tail.
func TestPrepareReportsBothSidesOnOneFormula(t *testing.T) {
	ctx := context.Background()
	d, agentRow, sess, history := overheadFixture(t, 6, "")

	const keepRecent = 4
	const overhead = 5000

	m := NewManager()
	// Budget below the true footprint (messages + overhead) but above the messages
	// alone, so only the overhead term can open the gate.
	m.SetLimits(EstimateTokens("", history)+overhead/2, keepRecent)

	// No step baseline: the overhead is opaque and identical on both sides, so the
	// two figures may differ ONLY by what the fold removed from the messages.
	prep, err := m.Prepare(WithContextOverhead(ctx, overhead), d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prep.Compacted {
		t.Fatal("fixture did not fold")
	}

	wantBefore := EstimateTokens("", history) + overhead
	if prep.Fold.BeforeTokens != wantBefore {
		t.Fatalf("BeforeTokens = %d, want %d (summary+pending+overhead)", prep.Fold.BeforeTokens, wantBefore)
	}
	wantAfter := EstimateTokens("ROLLED UP", history[len(history)-keepRecent:]) + overhead
	if prep.Fold.AfterTokens != wantAfter {
		t.Fatalf("AfterTokens = %d, want %d (new summary+kept tail+overhead)", prep.Fold.AfterTokens, wantAfter)
	}
	// The reported "after" is the same footprint the context meter and the pressure
	// ratio use — one number, not three.
	if got := prep.ContextTokens + overhead; got != prep.Fold.AfterTokens {
		t.Fatalf("ContextTokens+overhead = %d, but AfterTokens = %d", got, prep.Fold.AfterTokens)
	}
}

// TestPrepareKeepsOverheadWhenNoStepBaseline pins the opt-in shape: a caller that
// stamps only the overhead total (or reports no warm thread at all) gets the
// unchanged message-only accounting — the deduction must never guess that an
// opaque overhead contains a step trace.
func TestPrepareKeepsOverheadWhenNoStepBaseline(t *testing.T) {
	ctx := context.Background()
	trace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 300) + `"}]`
	d, agentRow, sess, history := overheadFixture(t, 6, trace)

	const keepRecent = 4
	const overhead = 40000

	m := NewManager()
	m.SetLimits(EstimateTokens("", history)+overhead/2, keepRecent)

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"no baseline stamped", WithContextOverhead(ctx, overhead)},
		{"cold turn (no warm thread)", WithContextOverheadStepBase(WithContextOverhead(ctx, overhead), -1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prep, err := m.Prepare(tc.ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			if !prep.Compacted {
				t.Fatal("fixture did not fold")
			}
			want := EstimateTokens("ROLLED UP", history[len(history)-keepRecent:]) + overhead
			if prep.Fold.AfterTokens != want {
				t.Fatalf("AfterTokens = %d, want %d (overhead must stay whole)", prep.Fold.AfterTokens, want)
			}
		})
	}
}

// TestPrepareSurfacesMalformedFoldedTrace guards the no-silent-zero rule on the
// new deduction path: a corrupt persisted trace inside the folded range fails the
// turn instead of being counted as 0, which would silently under-report the
// footprint — the same class of bug this deduction fixes.
func TestPrepareSurfacesMalformedFoldedTrace(t *testing.T) {
	ctx := context.Background()
	d, agentRow, sess, history := overheadFixture(t, 6, "{")

	m := NewManager()
	m.SetLimits(EstimateTokens("", history)/2, 4)

	turnCtx := WithContextOverheadStepBase(WithContextOverhead(ctx, 5000), 0)
	if _, err := m.Prepare(turnCtx, d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history); err == nil {
		t.Fatal("malformed persisted trace in the folded range must be observable")
	}
}
