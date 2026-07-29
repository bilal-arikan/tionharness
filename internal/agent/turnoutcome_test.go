package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A turn that finished before any watchdog fired and left no terminal marker is
// the only case that may be reported as completed.
func TestClassifyTurnOutcome_Clean(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), time.Minute, time.Minute)
	defer stop()

	got := classifyTurnOutcome(ctx, []TurnStep{{Kind: StepText, Text: "done"}}, time.Minute, time.Minute)
	if got.Status != turnStatusCompleted {
		t.Fatalf("status = %q, want %q", got.Status, turnStatusCompleted)
	}
	if got.Note != "" {
		t.Fatalf("clean turn must carry no note, got %q", got.Note)
	}
	if got.Truncated() {
		t.Fatal("clean turn reported as truncated")
	}
}

// The SES17 regression: the wall-clock ceiling cancels the context, the provider
// salvages partial text and returns nil — the turn must NOT read as completed.
func TestClassifyTurnOutcome_HardTimeout(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), 10*time.Millisecond, time.Minute)
	defer stop()
	<-ctx.Done()

	got := classifyTurnOutcome(ctx, nil, 20*time.Minute, 5*time.Minute)
	if got.Status != turnStatusTimeout {
		t.Fatalf("status = %q, want %q", got.Status, turnStatusTimeout)
	}
	if !strings.Contains(got.Note, "20 dk") {
		t.Fatalf("note must quote the ceiling that fired, got %q", got.Note)
	}
}

// The idle watchdog is a different failure (hung, not slow) and must say so.
func TestClassifyTurnOutcome_IdleTimeout(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), time.Hour, 10*time.Millisecond)
	defer stop()
	<-ctx.Done()

	got := classifyTurnOutcome(ctx, nil, time.Hour, 5*time.Minute)
	if got.Status != turnStatusTimeout {
		t.Fatalf("status = %q, want %q", got.Status, turnStatusTimeout)
	}
	if !strings.Contains(got.Note, "5 dk") {
		t.Fatalf("note must quote the idle window, got %q", got.Note)
	}
}

// A human pressing "Durdur" cancels the same context; the cause must stay plain
// context.Canceled so the caller can still report it as "killed", not a timeout.
func TestClassifyTurnOutcome_UserStopIsNotTimeout(t *testing.T) {
	ctx, stop := withActivityTimeout(context.Background(), time.Hour, time.Hour)
	stop()
	<-ctx.Done()

	got := classifyTurnOutcome(ctx, nil, time.Hour, time.Hour)
	if got.Status != turnStatusCompleted {
		t.Fatalf("user stop must not be classified as a deadline, got %q", got.Status)
	}
}

// Every terminal marker the native tool loop can append means truncated work.
func TestClassifyTurnOutcome_TerminalMarkers(t *testing.T) {
	for _, reason := range []termReason{termMaxIters, termGuardrailHalt, termContextExhausted, termMaxTokenExhausted} {
		steps := []TurnStep{
			{Kind: StepText, Text: "working"},
			{Kind: StepRecovery, Reason: string(reason), Text: "terminal"},
		}
		got := classifyTurnOutcome(context.Background(), steps, time.Hour, time.Hour)
		if got.Status != turnStatusIncomplete {
			t.Fatalf("reason %q: status = %q, want %q", reason, got.Status, turnStatusIncomplete)
		}
		if got.Note == "" {
			t.Fatalf("reason %q: truncated turn must carry a note", reason)
		}
	}
}

// Mid-turn recovery steps (retry/compaction) are NOT terminal and must not
// downgrade an otherwise clean turn.
func TestClassifyTurnOutcome_MidTurnRecoveryIsClean(t *testing.T) {
	steps := []TurnStep{
		{Kind: StepRecovery, Reason: string(contProviderRetry), Text: "retrying"},
		{Kind: StepText, Text: "done"},
	}
	got := classifyTurnOutcome(context.Background(), steps, time.Hour, time.Hour)
	if got.Status != turnStatusCompleted {
		t.Fatalf("status = %q, want %q", got.Status, turnStatusCompleted)
	}
}

// A watchdog cancels the turn from OUTSIDE the loop, so nothing marks the trace
// unless appendOutcomeStep does. The loop's own markers must NOT be duplicated.
func TestAppendOutcomeStep(t *testing.T) {
	timeout := turnOutcome{Status: turnStatusTimeout, Note: "expired"}
	got := appendOutcomeStep([]TurnStep{{Kind: StepText}}, timeout)
	if len(got) != 2 {
		t.Fatalf("timeout must add one step, got %d", len(got))
	}
	if got[1].Kind != StepRecovery || got[1].Reason != string(termTimeout) {
		t.Fatalf("unexpected step: kind=%q reason=%q", got[1].Kind, got[1].Reason)
	}

	// The tool loop already appended its own StepRecovery for these.
	loopMarked := []TurnStep{{Kind: StepRecovery, Reason: string(termMaxIters)}}
	if out := appendOutcomeStep(loopMarked, turnOutcome{Status: turnStatusIncomplete, Note: "n"}); len(out) != 1 {
		t.Fatalf("loop-marked truncation must not be duplicated, got %d steps", len(out))
	}
	if out := appendOutcomeStep(loopMarked, turnOutcome{Status: turnStatusCompleted}); len(out) != 1 {
		t.Fatalf("clean turn must add nothing, got %d steps", len(out))
	}
}

// The salvaged fragment is kept, but the note leads so nobody reads it as a result.
func TestApplyTurnOutcome(t *testing.T) {
	o := turnOutcome{Status: turnStatusTimeout, Note: "NOTE"}
	got := applyTurnOutcome("partial answer", o)
	if !strings.HasPrefix(got, "NOTE") {
		t.Fatalf("note must lead, got %q", got)
	}
	if !strings.Contains(got, "partial answer") {
		t.Fatalf("salvaged text must be preserved, got %q", got)
	}
	if clean := applyTurnOutcome("all good", turnOutcome{Status: turnStatusCompleted}); clean != "all good" {
		t.Fatalf("clean turn text must pass through, got %q", clean)
	}
	if empty := applyTurnOutcome("   ", o); empty != "NOTE" {
		t.Fatalf("empty salvage must yield the bare note, got %q", empty)
	}
}
