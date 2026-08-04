package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

// An idle-cut turn is retried exactly once; the second attempt sees the FIRST
// attempt's salvaged fragment so it can continue instead of restart.
func TestRunTurnWithIdleResume_ResumesOnceOnIdle(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	var sawPrev string
	ctx, cancel, out, _, err := r.runTurnWithIdleResume(context.Background(), time.Hour, 10*time.Millisecond,
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			attempts++
			if attempt == 1 {
				// Go idle so the watchdog fires: block until the ctx is cancelled.
				<-attemptCtx.Done()
				return "partial work", nil, context.Canceled
			}
			sawPrev = prevOutput
			return "finished", nil, nil
		})
	defer cancel()

	if attempts != 2 {
		t.Fatalf("idle turn must be attempted exactly twice, got %d", attempts)
	}
	if sawPrev != "partial work" {
		t.Fatalf("resume must receive the prior fragment, got %q", sawPrev)
	}
	if out != "finished" || err != nil {
		t.Fatalf("final result = (%q, %v), want (\"finished\", nil)", out, err)
	}
	if turnHitIdleTimeout(ctx) {
		t.Fatal("final (clean) attempt must not read as an idle cut")
	}
}

// The resume is single-shot: a turn that idles on BOTH attempts stops after the
// second and returns the final attempt's idle-cut context for the caller to report.
func TestRunTurnWithIdleResume_SingleShot(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	ctx, cancel, _, _, _ := r.runTurnWithIdleResume(context.Background(), time.Hour, 10*time.Millisecond,
		func(attemptCtx context.Context, _ context.CancelFunc, _ int, _ string) (string, []TurnStep, error) {
			attempts++
			<-attemptCtx.Done()
			return "still partial", nil, context.Canceled
		})
	defer cancel()

	if attempts != 2 {
		t.Fatalf("resume budget is single-shot: want 2 attempts, got %d", attempts)
	}
	if !turnHitIdleTimeout(ctx) {
		t.Fatal("a twice-idle turn must return an idle-cut context so the caller reports it unfinished")
	}
}

// A HARD wall-clock cut is a real ceiling — it is never resumed (resuming would
// just blow it again).
func TestRunTurnWithIdleResume_HardTimeoutNotResumed(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	ctx, cancel, _, _, _ := r.runTurnWithIdleResume(context.Background(), 10*time.Millisecond, time.Hour,
		func(attemptCtx context.Context, _ context.CancelFunc, _ int, _ string) (string, []TurnStep, error) {
			attempts++
			<-attemptCtx.Done()
			return "partial", nil, context.Canceled
		})
	defer cancel()

	if attempts != 1 {
		t.Fatalf("hard timeout must not be resumed, got %d attempts", attempts)
	}
	if turnHitIdleTimeout(ctx) {
		t.Fatal("hard-cut context must not read as an idle cut")
	}
}

// A clean first attempt runs exactly once — no resume, no wasted turn.
func TestRunTurnWithIdleResume_CleanRunsOnce(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	_, cancel, out, _, err := r.runTurnWithIdleResume(context.Background(), time.Hour, time.Hour,
		func(_ context.Context, _ context.CancelFunc, _ int, _ string) (string, []TurnStep, error) {
			attempts++
			return "done", nil, nil
		})
	defer cancel()

	if attempts != 1 || out != "done" || err != nil {
		t.Fatalf("clean turn: attempts=%d out=%q err=%v", attempts, out, err)
	}
}

func TestResumeContinuationPrompt(t *testing.T) {
	// A non-empty fragment is embedded so the model continues rather than restarts.
	got := resumeContinuationPrompt("do the task", "half-done output")
	if !strings.Contains(got, "half-done output") || !strings.Contains(got, "do the task") {
		t.Fatalf("continuation prompt must carry both fragment and original task, got %q", got)
	}
	// An empty fragment falls back to the original task verbatim.
	if got := resumeContinuationPrompt("do the task", "   "); got != "do the task" {
		t.Fatalf("blank fragment must fall back to the original, got %q", got)
	}
}
