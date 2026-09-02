package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// countingErrorProvider always fails and counts how many times it was asked.
// The count is the whole point: the cooldown's contract is "one attempt per
// window", which is only observable as a call that did NOT happen.
type countingErrorProvider struct{ calls int }

func (p *countingErrorProvider) Name() string { return "counting" }
func (p *countingErrorProvider) Complete(_ context.Context, _ providers.Request) (*providers.Response, error) {
	p.calls++
	return nil, errors.New("fold provider is having a bad day")
}

// foldFailureFixture builds a store, an agent and a session, plus a history and
// a budget small enough that every Prepare below is over budget and foldable.
func foldFailureFixture(t *testing.T) (*Manager, *db.DB, db.Agent, db.Session, []db.Message) {
	t.Helper()
	d, agent := foldUsageTestDB(t)
	sess, err := d.CreateSession(context.Background(), db.Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	m := NewManager()
	m.SetLimits(10, 2)
	history := make([]db.Message, 6)
	for i := range history {
		history[i] = db.Message{Role: providers.RoleUser, Text: "a reasonably long message that costs tokens"}
	}
	return m, d, agent, sess, history
}

// TestFoldFailureDoesNotKillTheTurn is the core of the change: a summarizer that
// fails used to propagate out of Prepare, and the API layer turned that into
// failTurn — so one transient 429 on the fold provider destroyed the user's turn.
// Now the turn survives, uncompacted, and says so.
func TestFoldFailureDoesNotKillTheTurn(t *testing.T) {
	m, d, agent, sess, history := foldFailureFixture(t)
	p := &countingErrorProvider{}

	prep, err := m.Prepare(context.Background(), d, p, sess, agent, history)
	if err != nil {
		t.Fatalf("prepare must not fail when only the summary failed: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 (the fold was attempted)", p.calls)
	}
	if !prep.FoldFailed {
		t.Fatal("FoldFailed = false, want true — an unannounced oversized context is the bug")
	}
	if prep.FoldError == "" {
		t.Fatal("FoldError is empty; the on-screen warning has nothing to show")
	}
	if prep.Compacted {
		t.Fatal("Compacted = true after a failed summary")
	}
	if len(prep.Messages) != len(history) {
		t.Fatalf("sent %d messages, want the full uncompacted %d", len(prep.Messages), len(history))
	}
}

// TestFoldFailureCooldownSkipsTheNextAttempt pins the stand-down. A failed fold
// changes nothing about the footprint that triggered it, so without this the very
// next turn would pay for the same failing call again.
func TestFoldFailureCooldownSkipsTheNextAttempt(t *testing.T) {
	m, d, agent, sess, history := foldFailureFixture(t)
	p := &countingErrorProvider{}
	ctx := context.Background()

	if _, err := m.Prepare(ctx, d, p, sess, agent, history); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	prep, err := m.Prepare(ctx, d, p, sess, agent, history)
	if err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("provider calls = %d, want 1 — the cooldown must skip the retry", p.calls)
	}
	if prep.FoldFailed {
		t.Fatal("FoldFailed = true on a cooldown-skipped turn; nothing was attempted, so nothing failed")
	}
	if prep.Compacted {
		t.Fatal("Compacted = true on a cooldown-skipped turn")
	}
}

// TestFoldFailureCooldownExpires: the stand-down is a pause, not a kill switch.
func TestFoldFailureCooldownExpires(t *testing.T) {
	prev := foldFailureCooldown
	// Negative, not a tiny positive: Windows' wall clock granularity is coarse
	// enough (~1ms+) that two time.Now() calls in the same test can be equal, which
	// would leave a 1ns cooldown still "active" and make this test flaky.
	foldFailureCooldown = -time.Second
	t.Cleanup(func() { foldFailureCooldown = prev })

	m, d, agent, sess, history := foldFailureFixture(t)
	p := &countingErrorProvider{}
	ctx := context.Background()

	if _, err := m.Prepare(ctx, d, p, sess, agent, history); err != nil {
		t.Fatalf("first prepare: %v", err)
	}
	if _, err := m.Prepare(ctx, d, p, sess, agent, history); err != nil {
		t.Fatalf("second prepare: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 — an expired cooldown must let the fold retry", p.calls)
	}
}

// TestFoldFailureCooldownIsPerSession guards the map keying: one unhealthy
// session must not stand down every other session sharing the Manager.
func TestFoldFailureCooldownIsPerSession(t *testing.T) {
	m, d, agent, sess, history := foldFailureFixture(t)
	other, err := d.CreateSession(context.Background(), db.Session{AgentID: agent.ID, Title: "T2"})
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	p := &countingErrorProvider{}
	ctx := context.Background()

	if _, err := m.Prepare(ctx, d, p, sess, agent, history); err != nil {
		t.Fatalf("prepare session 1: %v", err)
	}
	if _, err := m.Prepare(ctx, d, p, other, agent, history); err != nil {
		t.Fatalf("prepare session 2: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("provider calls = %d, want 2 — session 2 has its own stand-down", p.calls)
	}
}

// TestClearFoldCooldownReleasesTheStandDown covers the success path's side of the
// contract without needing a whole successful fold: a fold that works drops the
// entry rather than leaving one dead key per session that ever failed.
func TestClearFoldCooldownReleasesTheStandDown(t *testing.T) {
	m := NewManager()
	m.noteFoldFailure("S1")
	if _, cooling := m.foldCoolingDown("S1"); !cooling {
		t.Fatal("expected S1 to be cooling down after a noted failure")
	}
	m.clearFoldCooldown("S1")
	if _, cooling := m.foldCoolingDown("S1"); cooling {
		t.Fatal("clearFoldCooldown did not release the stand-down")
	}
	if len(m.foldFailedUntil) != 0 {
		t.Fatalf("foldFailedUntil still holds %d entries; the map leaks one key per failed session", len(m.foldFailedUntil))
	}
}

// TestOnlyTheSummaryCallIsRecoverable pins the error classification. Prepare
// swallows exactly one failure — the summarizer LLM call — and nothing else, so
// the marker has to travel on the error itself.
func TestOnlyTheSummaryCallIsRecoverable(t *testing.T) {
	m, d, agent, sess, history := foldFailureFixture(t)
	fold, keepTail, newCount, ok := foldBoundary(history, 0, 2)
	if !ok {
		t.Fatal("fixture history is not foldable; the rest of this test is meaningless")
	}
	_, err := m.applyRollingFold(context.Background(), rollingFoldInput{
		database: d, provider: &countingErrorProvider{}, session: sess, agent: agent,
		history: history, fold: fold, keepTail: keepTail, newCount: newCount,
	})
	if err == nil {
		t.Fatal("applyRollingFold must return the provider error")
	}
	if !errors.Is(err, errFoldSummary) {
		t.Fatalf("error %v is not marked errFoldSummary; Prepare would treat it as fatal", err)
	}
}
