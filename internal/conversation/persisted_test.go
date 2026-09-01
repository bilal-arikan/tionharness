package conversation

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// TestEstimatePersistedStepTokensCountsAssistantOnly pins the numerator: only an
// assistant turn carries a persisted trace the CLI keeps in its own thread, so a
// user turn with (impossible, but stored) Steps must not be billed.
func TestEstimatePersistedStepTokensCountsAssistantOnly(t *testing.T) {
	steps := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("x ", 400) + `"}]`
	one, _, err := EstimatePersistedSteps(steps)
	if err != nil {
		t.Fatalf("estimate steps: %v", err)
	}
	if one <= 0 {
		t.Fatalf("fixture produced %d tokens, want > 0", one)
	}

	msgs := []db.Message{
		{Role: providers.RoleUser, Text: "u", Steps: steps},
		{Role: providers.RoleAssistant, Text: "a", Steps: steps},
		{Role: providers.RoleAssistant, Text: "a2", Steps: steps},
		{Role: providers.RoleAssistant, Text: "a3"}, // no trace
	}
	got, err := EstimatePersistedStepTokens(msgs)
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if got != 2*one {
		t.Fatalf("persisted step tokens = %d, want %d (two assistant traces)", got, 2*one)
	}
}

// TestEstimatePersistedStepTokensSurfacesMalformedTrace guards the no-silent-zero
// rule: a corrupt trace is an error, not an implicit 0 that would understate the
// budget exactly like the bug this term fixes.
func TestEstimatePersistedStepTokensSurfacesMalformedTrace(t *testing.T) {
	got, err := EstimatePersistedStepTokens([]db.Message{{Role: providers.RoleAssistant, Steps: "{"}})
	if err == nil {
		t.Fatal("malformed persisted trace must be observable")
	}
	if got != 0 {
		t.Fatalf("tokens on error = %d, want 0", got)
	}
}

// TestPrepareFoldsOnPersistedTraceOverhead is the unit-scale SES2230 regression:
// the visible text sits comfortably under budget while the CLI's retained tool
// trace blows past it. Without the persisted-steps term the gate stays silent
// forever; with it fed in as context overhead the fold fires.
func TestPrepareFoldsOnPersistedTraceOverhead(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agentRow, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "codex-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	bigTrace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 2000) + `"}]`
	history := []db.Message{
		{Role: providers.RoleUser, Text: "u1"}, {Role: providers.RoleAssistant, Text: "a1", Steps: bigTrace},
		{Role: providers.RoleUser, Text: "u2"}, {Role: providers.RoleAssistant, Text: "a2", Steps: bigTrace},
		{Role: providers.RoleUser, Text: "u3"}, {Role: providers.RoleAssistant, Text: "a3", Steps: bigTrace},
	}

	textOnly := EstimateTokens("", history)
	steps, err := EstimatePersistedStepTokens(history)
	if err != nil {
		t.Fatalf("estimate persisted: %v", err)
	}
	budget := textOnly + steps/2 // above text alone, below text + retained trace
	if budget <= textOnly {
		t.Fatalf("fixture trace too small: text=%d steps=%d", textOnly, steps)
	}

	m := NewManager()
	m.SetLimits(budget, 2)

	// Cold turn: nothing is retained provider-side, so no overhead and no fold.
	cold, err := m.Prepare(ctx, d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
	if err != nil {
		t.Fatalf("cold prepare: %v", err)
	}
	if cold.Compacted {
		t.Fatal("cold turn folded: text alone is under budget")
	}

	// Warm CLI thread: the retained trace is fed in as context overhead.
	warm, err := m.Prepare(WithContextOverhead(ctx, steps), d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
	if err != nil {
		t.Fatalf("warm prepare: %v", err)
	}
	if !warm.Compacted {
		t.Fatalf("warm thread did not fold: text=%d steps=%d budget=%d", textOnly, steps, budget)
	}
	if warm.Fold.BeforeTokens <= budget {
		t.Fatalf("BeforeTokens = %d, want > budget %d", warm.Fold.BeforeTokens, budget)
	}
}

// TestPrepareJournalsContextPressure covers the early warning: a turn that fits
// but sits at/above pressureWarnRatio journals a `pressure` event, and a roomy
// turn journals none.
func TestPrepareJournalsContextPressure(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agentRow, err := d.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	history := []db.Message{{Role: providers.RoleUser, Text: "hello there"}}
	used := EstimateTokens("", history)

	// Roomy budget → no warning.
	m := NewManager()
	m.SetLimits(used*10, 8)
	if _, err := m.Prepare(ctx, d, nil, sess, agentRow, history); err != nil {
		t.Fatalf("roomy prepare: %v", err)
	}
	evs, err := d.ReadDebugEvents(ctx, sess.ID, db.DebugPressure, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("pressure events at low usage = %d, want 0", len(evs))
	}

	// Tight budget, still fitting → exactly one warning.
	m.SetLimits(used+1, 8)
	prep, err := m.Prepare(ctx, d, nil, sess, agentRow, history)
	if err != nil {
		t.Fatalf("tight prepare: %v", err)
	}
	if prep.Compacted {
		t.Fatal("did not expect a fold below keepRecent")
	}
	if prep.Pressure < pressureWarnRatio {
		t.Fatalf("pressure = %f, want >= %f", prep.Pressure, pressureWarnRatio)
	}
	evs, err = d.ReadDebugEvents(ctx, sess.ID, db.DebugPressure, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("pressure events = %d, want 1", len(evs))
	}
	if evs[0].Detail != "context pressure detail [redacted]" {
		t.Fatalf("pressure detail = %q, want fail-closed pressure summary", evs[0].Detail)
	}
}
