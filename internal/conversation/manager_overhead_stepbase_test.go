package conversation

import (
	"context"
	"strings"
	"testing"
)

// TestPrepareDropsFoldedStepsFromPositiveBaseline covers the shape a warm CLI
// thread actually reports: the overhead was charged from a NON-zero index,
// because a previous rolling fold (SummaryMsgCount) or a CLI-side compaction
// (CLICompactMsgCount) already moved the earlier trace out of the provider's
// window — see warmCLIStepBaseline. The sibling tests in
// manager_overhead_test.go only pin base 0 and base -1, so the deduction was
// never exercised against a real offset: an implementation that ignored base and
// deducted history[:newCount] would pass all of them and over-deduct here.
//
// Fixture: 6 messages, base 2, keepRecent 2 → the fold boundary lands on
// newCount 4, so exactly history[2:4] must leave the reported footprint and
// history[4:] must stay. The expected number is derived from the fixture's own
// parts, never from Prepare's output.
func TestPrepareDropsFoldedStepsFromPositiveBaseline(t *testing.T) {
	ctx := context.Background()
	trace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 300) + `"}]`
	d, agentRow, sess, history := overheadFixture(t, 3, trace)

	const keepRecent = 2
	const base = 2
	const systemFillers = 1500 // the non-message, non-trace part of the overhead

	// What contextOverheadTokens would have charged on this session: the fixed
	// fillers plus ONLY the trace from the baseline onward.
	chargedSteps, err := EstimatePersistedStepTokens(history[base:])
	if err != nil {
		t.Fatalf("estimate charged: %v", err)
	}
	overhead := systemFillers + chargedSteps

	m := NewManager()
	// Budget below the true footprint so the fold fires, above the visible text so
	// only the overhead term can trip it.
	m.SetLimits(EstimateTokens("", history)+chargedSteps/2, keepRecent)

	turnCtx := WithContextOverheadStepBase(WithContextOverhead(ctx, overhead), base)
	prep, err := m.Prepare(turnCtx, d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prep.Compacted {
		t.Fatalf("fixture did not fold (overhead=%d)", overhead)
	}

	kept := history[len(history)-keepRecent:]
	keptSteps, err := EstimatePersistedStepTokens(kept)
	if err != nil {
		t.Fatalf("estimate kept: %v", err)
	}
	// Only the overlap history[base:newCount] leaves; the pre-baseline trace was
	// never in the overhead to begin with and must not be deducted a second time.
	wantAfter := EstimateTokens("ROLLED UP", kept) + systemFillers + keptSteps
	if prep.Fold.AfterTokens != wantAfter {
		t.Fatalf("AfterTokens = %d, want %d (deduction must be history[%d:%d] only)",
			prep.Fold.AfterTokens, wantAfter, base, len(history)-keepRecent)
	}
	if prep.Fold.AfterTokens >= prep.Fold.BeforeTokens {
		t.Fatalf("fold did not shrink the footprint: %d → %d", prep.Fold.BeforeTokens, prep.Fold.AfterTokens)
	}
}

// TestPrepareKeepsOverheadWhenBaselineIsPastTheFold pins the other side of the
// same offset: when the baseline sits AFTER the new summary boundary, the folded
// messages' trace was never charged (the CLI had already dropped it), so the
// deduction must be zero. Deducting anything here would under-report the
// footprint and let the gate under-fire — the mirror image of the bug the
// deduction fixes.
func TestPrepareKeepsOverheadWhenBaselineIsPastTheFold(t *testing.T) {
	ctx := context.Background()
	trace := `[{"kind":"tool","tool":"Bash","input":{"cmd":"ls"},"output":"` + strings.Repeat("payload ", 300) + `"}]`
	d, agentRow, sess, history := overheadFixture(t, 3, trace)

	const keepRecent = 2
	const base = 5 // > newCount (4): the folded range carries no charged trace
	const overhead = 40000

	m := NewManager()
	m.SetLimits(EstimateTokens("", history)+overhead/2, keepRecent)

	turnCtx := WithContextOverheadStepBase(WithContextOverhead(ctx, overhead), base)
	prep, err := m.Prepare(turnCtx, d, stubProvider{summary: "ROLLED UP"}, sess, agentRow, history)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if !prep.Compacted {
		t.Fatal("fixture did not fold")
	}
	want := EstimateTokens("ROLLED UP", history[len(history)-keepRecent:]) + overhead
	if prep.Fold.AfterTokens != want {
		t.Fatalf("AfterTokens = %d, want %d (baseline past the fold ⇒ no deduction)", prep.Fold.AfterTokens, want)
	}
}
