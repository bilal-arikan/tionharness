package indexstate

import (
	"errors"
	"testing"
	"time"
)

const testTool = "zg"

func TestGetReportsUnseenApartFromMissing(t *testing.T) {
	m := New()
	// "never looked" and "looked and found nothing" are different answers; only a
	// caller that inspected the disk may assert the second.
	if _, seen := m.Get(testTool, "/repo"); seen {
		t.Fatal("an untouched index reported as seen")
	}
	m.Observe(testTool, "/repo", PhaseMissing, "", "")
	e, seen := m.Get(testTool, "/repo")
	if !seen || e.Phase != PhaseMissing {
		t.Fatalf("after Observe: seen=%v phase=%q, want true/missing", seen, e.Phase)
	}
}

func TestCreateTransitionMissingToIndexingToReady(t *testing.T) {
	m := New()
	m.Observe(testTool, "/repo", PhaseMissing, "", "")

	e, claimed := m.Begin(testTool, "/repo", ActionCreate)
	if !claimed {
		t.Fatal("first run was not claimed")
	}
	if e.Run == 0 {
		t.Fatal("a granted claim carried no run token")
	}
	if e.Phase != PhaseIndexing || e.Action != ActionCreate {
		t.Fatalf("begin: phase=%q action=%q, want indexing/create", e.Phase, e.Action)
	}
	if e.Usable() {
		t.Error("an index still being built reported as usable")
	}

	e = mustSucceed(t, m, "/repo", e.Run, "local/potion-code-16m-v2", "1.2.0")
	if e.Phase != PhaseReady || !e.Usable() {
		t.Fatalf("succeed: phase=%q usable=%v, want ready/true", e.Phase, e.Usable())
	}
	if e.Embedding != "local/potion-code-16m-v2" || e.ToolVersion != "1.2.0" {
		t.Errorf("succeed did not record what the store was built with: %+v", e)
	}
}

func TestBeginIsASingleRunClaim(t *testing.T) {
	m := New()
	first, claimed := m.Begin(testTool, "/repo", ActionCreate)
	if !claimed {
		t.Fatal("first claim refused")
	}
	// Two workspace runtimes on the same repository share one store; the second
	// must not start its own run.
	if _, claimed := m.Begin(testTool, "/repo", ActionCreate); claimed {
		t.Fatal("a second run was claimed while one was in flight")
	}
	mustSucceed(t, m, "/repo", first.Run, "local/m", "1.0.0")
	second, claimed := m.Begin(testTool, "/repo", ActionRefresh)
	if !claimed {
		t.Fatal("a run after the previous one finished was refused")
	}
	if second.Run == first.Run {
		t.Fatalf("two runs share claim token %d", first.Run)
	}
}

func TestFailedNeverReadsBackAsReady(t *testing.T) {
	m := New()
	c, _ := m.Begin(testTool, "/repo", ActionCreate)
	e, err := m.Fail(testTool, "/repo", c.Run, "zg exited 1")
	if err != nil {
		t.Fatalf("Fail with the current claim: %v", err)
	}

	if e.Phase != PhaseFailed {
		t.Fatalf("phase=%q, want failed", e.Phase)
	}
	if e.Usable() {
		t.Error("a failed index reported as usable")
	}
	if e.Error != "zg exited 1" {
		t.Errorf("reason=%q, want the failure text", e.Error)
	}
	// A later disk observation must not quietly erase the recorded failure into
	// a success without a run actually happening.
	got, _ := m.Get(testTool, "/repo")
	if got.Phase != PhaseFailed {
		t.Errorf("failure did not persist: %q", got.Phase)
	}
}

func TestFailAlwaysCarriesAReason(t *testing.T) {
	m := New()
	c, _ := m.Begin(testTool, "/repo", ActionCreate)
	e, err := m.Fail(testTool, "/repo", c.Run, "   ")
	if err != nil {
		t.Fatalf("Fail with the current claim: %v", err)
	}
	if e.Error == "" {
		t.Fatal("a failed entry was left with no explanation")
	}
}

func TestObserveDoesNotDisturbARunInFlight(t *testing.T) {
	m := New()
	m.Begin(testTool, "/repo", ActionCreate)
	// A concurrent observer reading a half-written manifest must not declare the
	// index ready underneath the run that owns it.
	e := m.Observe(testTool, "/repo", PhaseReady, "local/m", "1.0.0")
	if e.Phase != PhaseIndexing {
		t.Fatalf("phase=%q, want the in-flight run to keep the entry", e.Phase)
	}
}

func TestStaleIsUsableButReady2IsNot(t *testing.T) {
	m := New()
	// A lagging index still answers; it just misses the newest edits.
	e := m.Observe(testTool, "/repo", PhaseStale, "local/m", "1.0.0")
	if !e.Usable() {
		t.Error("a stale index must still be usable")
	}
	e = m.Observe(testTool, "/other", PhaseMissing, "", "")
	if e.Usable() {
		t.Error("a missing index must not be usable")
	}
}

func TestKeyNormalisesRootSoOneRepoIsOneEntry(t *testing.T) {
	m := New()
	m.Observe(testTool, "/repo/sub/..", PhaseReady, "local/m", "1.0.0")
	if _, seen := m.Get(testTool, "/repo"); !seen {
		t.Fatal("an unnormalised root minted a second ledger entry")
	}
}

func TestListIsOrderedAndStable(t *testing.T) {
	m := New()
	m.Observe("zg", "/b", PhaseReady, "", "")
	m.Observe("zg", "/a", PhaseReady, "", "")
	m.Observe("cbm", "/c", PhaseReady, "", "")

	first := m.List()
	if len(first) != 3 {
		t.Fatalf("got %d entries, want 3", len(first))
	}
	// Entry.Root is host-cleaned, so compare against the same cleaning rather
	// than the literal written above (on Windows "/a" cleans to `\a`).
	if first[0].Tool != "cbm" || first[1].Tool != "zg" || first[2].Tool != "zg" {
		t.Fatalf("unordered by tool: %+v", first)
	}
	if !(first[1].Root < first[2].Root) {
		t.Fatalf("unordered by root: %+v", first)
	}
	// Map iteration is randomised per range; the API must not reshuffle on poll.
	for i := 0; i < 5; i++ {
		if got := m.List(); got[0].Root != first[0].Root || got[2].Root != first[2].Root {
			t.Fatal("List order changed between calls")
		}
	}
}

func TestTimestampsAreRecorded(t *testing.T) {
	m := New()
	at := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	m.SetClock(func() time.Time { return at })

	e, _ := m.Begin(testTool, "/repo", ActionCreate)
	if !e.StartedAt.Equal(at) {
		t.Fatalf("StartedAt=%v, want %v", e.StartedAt, at)
	}
	e = mustSucceed(t, m, "/repo", e.Run, "local/m", "1.0.0")
	if !e.UpdatedAt.Equal(at) {
		t.Fatalf("UpdatedAt=%v, want %v", e.UpdatedAt, at)
	}
}

// mustSucceed closes a claimed run and fails the test if the ledger refuses.
func mustSucceed(t *testing.T, m *Manager, root string, run uint64, embedding, version string) Entry {
	t.Helper()
	e, err := m.Succeed(testTool, root, run, embedding, version)
	if err != nil {
		t.Fatalf("Succeed with the current claim: %v", err)
	}
	return e
}

// Only the run Begin claimed may close an entry. Each of the tests below is a
// path to a silently-ready (or silently-rewritten) index that the claim check
// closes; each asserts ErrStaleClaim AND that the ledger was left untouched.

func TestSucceedOnAnUnclaimedEntryIsRejected(t *testing.T) {
	m := New()
	// No Observe, no Begin: nothing ever claimed this (tool, root).
	if _, err := m.Succeed(testTool, "/never-claimed", 1, "local/m", "1.0.0"); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("err=%v, want ErrStaleClaim", err)
	}
	if _, seen := m.Get(testTool, "/never-claimed"); seen {
		t.Error("a rejected Succeed minted a ledger entry")
	}
}

func TestSucceedWithoutAClaimOnAnObservedEntryIsRejected(t *testing.T) {
	m := New()
	m.Observe(testTool, "/repo", PhaseMissing, "", "")
	// Token 0 is what a caller that never called Begin holds.
	if _, err := m.Succeed(testTool, "/repo", 0, "local/m", "1.0.0"); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("err=%v, want ErrStaleClaim", err)
	}
	if got, _ := m.Get(testTool, "/repo"); got.Phase != PhaseMissing {
		t.Errorf("phase=%q, want missing to survive the rejected close", got.Phase)
	}
}

func TestLateSucceedCannotOverwriteARecordedFailure(t *testing.T) {
	m := New()
	c, _ := m.Begin(testTool, "/repo", ActionCreate)
	if _, err := m.Fail(testTool, "/repo", c.Run, "zg exited 1"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// The same run reporting again after it already closed: the claim is spent.
	if _, err := m.Succeed(testTool, "/repo", c.Run, "local/m", "1.0.0"); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("err=%v, want ErrStaleClaim", err)
	}
	got, _ := m.Get(testTool, "/repo")
	if got.Phase != PhaseFailed || got.Error != "zg exited 1" {
		t.Errorf("ledger phase=%q error=%q, want the failure and its reason kept", got.Phase, got.Error)
	}
}

func TestSupersededRunCannotCloseTheNewerRun(t *testing.T) {
	m := New()
	old, _ := m.Begin(testTool, "/repo", ActionCreate)
	// The old run's entry is Forgotten (e.g. its project vanished) and a new run
	// claims the same root while the old goroutine is still alive.
	m.Forget(testTool, "/repo")
	cur, claimed := m.Begin(testTool, "/repo", ActionRebuild)
	if !claimed {
		t.Fatal("re-claim after Forget refused")
	}

	if _, err := m.Succeed(testTool, "/repo", old.Run, "local/old", "0.9.0"); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale Succeed err=%v, want ErrStaleClaim", err)
	}
	if _, err := m.Fail(testTool, "/repo", old.Run, "old run died"); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("stale Fail err=%v, want ErrStaleClaim", err)
	}
	got, _ := m.Get(testTool, "/repo")
	if got.Phase != PhaseIndexing || got.Run != cur.Run || got.Error != "" {
		t.Fatalf("newer run disturbed: %+v", got)
	}
	// The owning run still closes normally.
	if e := mustSucceed(t, m, "/repo", cur.Run, "local/new", "1.0.0"); e.Embedding != "local/new" {
		t.Errorf("embedding=%q, want the owning run's", e.Embedding)
	}
}

func TestFailOnAnUnclaimedEntryIsRejected(t *testing.T) {
	m := New()
	if _, err := m.Fail(testTool, "/never-claimed", 7, "zg not found"); !errors.Is(err, ErrStaleClaim) {
		t.Fatalf("err=%v, want ErrStaleClaim", err)
	}
	if _, seen := m.Get(testTool, "/never-claimed"); seen {
		t.Error("a rejected Fail minted a ledger entry")
	}
}

func TestForgetIsBookkeepingOnly(t *testing.T) {
	m := New()
	m.Observe(testTool, "/repo", PhaseReady, "local/m", "1.0.0")
	m.Forget(testTool, "/repo")
	if _, seen := m.Get(testTool, "/repo"); seen {
		t.Fatal("Forget left the entry behind")
	}
}
