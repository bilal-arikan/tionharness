package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestStallJudgeMemoSkipsUnchangedText pins the per-coordinator verdict memo: the
// turn-end guard and the sweeper both judge the coordinator's latest message, and
// a silent coordinator keeps that message for the whole staleness window — so an
// unchanged text must be classified ONCE, a changed text again, and a judge
// error must never be remembered.
func TestStallJudgeMemoSkipsUnchangedText(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent := db.Agent{ID: "A", Provider: "missing", Model: "m"}

	calls := 0
	fail := false
	rt.stallJudgeFn = func(_ context.Context, _ db.Agent, text string) (bool, error) {
		calls++
		if fail {
			return false, errors.New("judge down")
		}
		return text == "spawned 3 workers", nil
	}

	if v, err := rt.judgeCoordinatorStalled(ctx, "SES-C", agent, "spawned 3 workers"); err != nil || !v {
		t.Fatalf("first verdict = %v, %v; want true", v, err)
	}
	if v, err := rt.judgeCoordinatorStalled(ctx, "SES-C", agent, "spawned 3 workers"); err != nil || !v {
		t.Fatalf("memoised verdict = %v, %v; want true", v, err)
	}
	if calls != 1 {
		t.Fatalf("judge called %d times for an unchanged message, want 1", calls)
	}

	// A different coordinator with the same text is judged on its own.
	if _, err := rt.judgeCoordinatorStalled(ctx, "SES-D", agent, "spawned 3 workers"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("second coordinator must be judged separately, calls=%d", calls)
	}

	// Changed text → judged again, verdict replaced.
	if v, _ := rt.judgeCoordinatorStalled(ctx, "SES-C", agent, "İş bitti."); v {
		t.Fatal("changed text must be re-judged, got stale true")
	}
	if calls != 3 {
		t.Fatalf("changed text must reach the judge, calls=%d", calls)
	}

	// Errors are not memoised: the next call with the same text tries again.
	fail = true
	if _, err := rt.judgeCoordinatorStalled(ctx, "SES-C", agent, "Round 5 opened"); err == nil {
		t.Fatal("judge error must propagate")
	}
	fail = false
	if v, err := rt.judgeCoordinatorStalled(ctx, "SES-C", agent, "Round 5 opened"); err != nil || v {
		t.Fatalf("after an error the same text must be re-judged, got %v, %v", v, err)
	}
	if calls != 5 {
		t.Fatalf("calls=%d, want 5", calls)
	}

	// Empty text short-circuits without touching the judge or the memo.
	if v, err := rt.judgeCoordinatorStalled(ctx, "SES-C", agent, "   "); v || err != nil || calls != 5 {
		t.Fatalf("empty text must not be judged: v=%v err=%v calls=%d", v, err, calls)
	}
}
