package indexstate

import (
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
	if e.Phase != PhaseIndexing || e.Action != ActionCreate {
		t.Fatalf("begin: phase=%q action=%q, want indexing/create", e.Phase, e.Action)
	}
	if e.Usable() {
		t.Error("an index still being built reported as usable")
	}

	e = m.Succeed(testTool, "/repo", "local/potion-code-16m-v2", "1.2.0")
	if e.Phase != PhaseReady || !e.Usable() {
		t.Fatalf("succeed: phase=%q usable=%v, want ready/true", e.Phase, e.Usable())
	}
	if e.Embedding != "local/potion-code-16m-v2" || e.ToolVersion != "1.2.0" {
		t.Errorf("succeed did not record what the store was built with: %+v", e)
	}
}

func TestBeginIsASingleRunClaim(t *testing.T) {
	m := New()
	if _, claimed := m.Begin(testTool, "/repo", ActionCreate); !claimed {
		t.Fatal("first claim refused")
	}
	// Two workspace runtimes on the same repository share one store; the second
	// must not start its own run.
	if _, claimed := m.Begin(testTool, "/repo", ActionCreate); claimed {
		t.Fatal("a second run was claimed while one was in flight")
	}
	m.Succeed(testTool, "/repo", "local/m", "1.0.0")
	if _, claimed := m.Begin(testTool, "/repo", ActionRefresh); !claimed {
		t.Fatal("a run after the previous one finished was refused")
	}
}

func TestFailedNeverReadsBackAsReady(t *testing.T) {
	m := New()
	m.Begin(testTool, "/repo", ActionCreate)
	e := m.Fail(testTool, "/repo", "zg exited 1")

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
	e := m.Fail(testTool, "/repo", "   ")
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
	e = m.Succeed(testTool, "/repo", "local/m", "1.0.0")
	if !e.UpdatedAt.Equal(at) {
		t.Fatalf("UpdatedAt=%v, want %v", e.UpdatedAt, at)
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
