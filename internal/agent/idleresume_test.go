package agent

import (
	"context"
	"strings"
	"testing"
	"time"
)

// idleOnce blocks the first `idleFor` attempts on the watchdog (returning a partial
// fragment) and completes cleanly afterwards. Returns an invoke closure plus a
// pointer to the attempt counter so tests can assert how many runs happened.
func idleOnce(idleFor int, attempts *int) func(context.Context, context.CancelFunc, int, string) (string, []TurnStep, error) {
	return func(attemptCtx context.Context, _ context.CancelFunc, attempt int, _ string) (string, []TurnStep, error) {
		*attempts++
		if attempt <= idleFor {
			<-attemptCtx.Done() // go idle so the watchdog fires
			return "partial work", nil, context.Canceled
		}
		return "finished", nil, nil
	}
}

// An idle-cut turn is retried up to the budget; the resumed attempt sees the PRIOR
// attempt's salvaged fragment so it can continue instead of restart.
func TestRunTurnWithIdleResume_ResumesOnceOnIdle(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	var sawPrev string
	ctx, cancel, out, _, err := r.runTurnWithIdleResume(context.Background(), time.Hour, 10*time.Millisecond, 1,
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			attempts++
			if attempt == 1 {
				<-attemptCtx.Done()
				return "partial work", nil, context.Canceled
			}
			sawPrev = prevOutput
			return "finished", nil, nil
		})
	defer cancel()

	if attempts != 2 {
		t.Fatalf("idle turn with budget 1 must be attempted exactly twice, got %d", attempts)
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

// The budget is spent after maxResume resumes: a turn that idles on EVERY attempt
// stops after 1+maxResume runs and returns the final idle-cut context to report.
func TestRunTurnWithIdleResume_BudgetSpent(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	ctx, cancel, _, _, _ := r.runTurnWithIdleResume(context.Background(), time.Hour, 10*time.Millisecond, 1,
		idleOnce(99, &attempts)) // never completes → always idles
	defer cancel()

	if attempts != 2 {
		t.Fatalf("budget 1 = single-shot: want 2 attempts, got %d", attempts)
	}
	if !turnHitIdleTimeout(ctx) {
		t.Fatal("a fully-idle turn must return an idle-cut context so the caller reports it unfinished")
	}
}

// The budget is configurable: budget 2 permits two resumes (3 attempts total).
func TestRunTurnWithIdleResume_BudgetTwo(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	_, cancel, out, _, err := r.runTurnWithIdleResume(context.Background(), time.Hour, 10*time.Millisecond, 2,
		idleOnce(2, &attempts)) // idles twice, then completes on attempt 3
	defer cancel()

	if attempts != 3 {
		t.Fatalf("budget 2 must allow two resumes (3 attempts), got %d", attempts)
	}
	if out != "finished" || err != nil {
		t.Fatalf("final result = (%q, %v), want (\"finished\", nil)", out, err)
	}
}

// Budget 0 disables the resume entirely: a single idle attempt, no retry.
func TestRunTurnWithIdleResume_BudgetZeroDisables(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	ctx, cancel, _, _, _ := r.runTurnWithIdleResume(context.Background(), time.Hour, 10*time.Millisecond, 0,
		idleOnce(99, &attempts))
	defer cancel()

	if attempts != 1 {
		t.Fatalf("budget 0 must disable resume: want 1 attempt, got %d", attempts)
	}
	if !turnHitIdleTimeout(ctx) {
		t.Fatal("the single idle attempt must still surface as an idle cut")
	}
}

// A HARD wall-clock cut is a real ceiling — it is never resumed regardless of budget
// (resuming would just blow it again).
func TestRunTurnWithIdleResume_HardTimeoutNotResumed(t *testing.T) {
	r := testRuntime(t)
	var attempts int
	ctx, cancel, _, _, _ := r.runTurnWithIdleResume(context.Background(), 10*time.Millisecond, time.Hour, 1,
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
	_, cancel, out, _, err := r.runTurnWithIdleResume(context.Background(), time.Hour, time.Hour, 1,
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

// The idle-resume budget is settings-driven: default when unset, disabled at 0, and
// the built-in default restored on a negative "unset" marker.
func TestTunables_IdleResumeMax(t *testing.T) {
	tun := NewTunables()
	if got := tun.IdleResumeMax(); got != DefaultIdleResumeMax {
		t.Fatalf("fresh NewTunables must yield the default %d, got %d", DefaultIdleResumeMax, got)
	}
	tun.SetIdleResumeMax(0)
	if got := tun.IdleResumeMax(); got != 0 {
		t.Fatalf("0 must disable (stay 0), got %d", got)
	}
	tun.SetIdleResumeMax(3)
	if got := tun.IdleResumeMax(); got != 3 {
		t.Fatalf("explicit 3 must pass through, got %d", got)
	}
	tun.SetIdleResumeMax(-1)
	if got := tun.IdleResumeMax(); got != DefaultIdleResumeMax {
		t.Fatalf("negative (unset) must map to the default %d, got %d", DefaultIdleResumeMax, got)
	}
}
