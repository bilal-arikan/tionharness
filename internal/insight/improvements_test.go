package insight

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates parent dirs and writes content (test helper).
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func openStore(t *testing.T) *FindingStore {
	t.Helper()
	s, err := OpenFindingStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

// TestRegressionOnRecurrence: a closed finding that recurs is flagged regressed;
// a fresh user decision clears the flag.
func TestRegressionOnRecurrence(t *testing.T) {
	s := openStore(t)
	f, _ := s.Upsert(Finding{LensID: "tool-errors", Channel: ChannelAppFix, Signature: "sigA", Title: "A", LastSeen: 100})
	if _, err := s.SetStatus(f.ID, StatusDismissed, 110); err != nil {
		t.Fatal(err)
	}
	// Recurrence of the dismissed finding.
	again, _ := s.Upsert(Finding{LensID: "tool-errors", Channel: ChannelAppFix, Signature: "sigA", Title: "A", LastSeen: 200})
	if !again.Regressed || again.RegressedAt != 200 {
		t.Fatalf("recurrence of a dismissed finding must flag regressed: %+v", again)
	}
	if again.Status != StatusDismissed {
		t.Fatalf("status should stay dismissed, got %s", again.Status)
	}
	// User re-decides → regression acknowledged/cleared.
	if _, err := s.SetStatus(again.ID, StatusAccepted, 210); err != nil {
		t.Fatal(err)
	}
	for _, x := range s.List("", "") {
		if x.ID == again.ID && x.Regressed {
			t.Fatal("SetStatus should clear the regressed flag")
		}
	}
}

// TestPriorityOrdering: regressed floats to top, resolved sinks, severity+recurrence rank.
func TestPriorityOrdering(t *testing.T) {
	hi := Finding{Severity: "high", Occurrences: 1}
	lo := Finding{Severity: "low", Occurrences: 1}
	reg := Finding{Severity: "low", Occurrences: 1, Regressed: true}
	dis := Finding{Severity: "high", Occurrences: 20, Status: StatusDismissed}
	if hi.PriorityScore() <= lo.PriorityScore() {
		t.Fatal("high must outrank low")
	}
	if reg.PriorityScore() <= hi.PriorityScore() {
		t.Fatal("regressed must outrank a normal high")
	}
	if dis.PriorityScore() >= lo.PriorityScore() {
		t.Fatal("a dismissed finding must sink below any open one")
	}
}

// TestListRanksByPriority: List returns priority order, not raw recency.
func TestListRanksByPriority(t *testing.T) {
	s := openStore(t)
	s.Upsert(Finding{LensID: "l", Channel: ChannelAppFix, Signature: "low", Title: "low", Severity: "low", LastSeen: 300})
	s.Upsert(Finding{LensID: "l", Channel: ChannelAppFix, Signature: "high", Title: "high", Severity: "high", LastSeen: 100})
	got := s.List("", "")
	if got[0].Title != "high" {
		t.Fatalf("high-severity must rank first despite older LastSeen: %+v", got)
	}
}

// TestMaintain: auto-verify applied-not-recurring; prune old resolved; keep regressed applied.
func TestMaintain(t *testing.T) {
	s := openStore(t)
	now := int64(1_000_000_000)
	old := now - autoVerifyAge - 1
	veryOld := now - pruneAge - 1

	applied, _ := s.Upsert(Finding{LensID: "l", Signature: "a", Title: "applied", Status: StatusApplied, LastSeen: old})
	appliedReg, _ := s.Upsert(Finding{LensID: "l", Signature: "b", Title: "reg", Status: StatusApplied, Regressed: true, LastSeen: old})
	s.Upsert(Finding{LensID: "l", Signature: "c", Title: "dismissed-old", Status: StatusDismissed, LastSeen: veryOld})
	s.Upsert(Finding{LensID: "l", Signature: "d", Title: "new-recent", Status: StatusNew, LastSeen: now})

	res, err := s.Maintain(now)
	if err != nil {
		t.Fatal(err)
	}
	if res.AutoVerified != 1 || res.Pruned != 1 {
		t.Fatalf("expected 1 auto-verified + 1 pruned, got %+v", res)
	}
	byID := map[string]Finding{}
	for _, f := range s.List("", "") {
		byID[f.ID] = f
	}
	if byID[applied.ID].Status != StatusVerified {
		t.Fatal("applied-not-recurring should auto-verify")
	}
	if byID[appliedReg.ID].Status != StatusApplied {
		t.Fatal("a regressed applied finding must NOT auto-verify")
	}
	if len(s.List("", "")) != 3 {
		t.Fatalf("the old dismissed finding should be pruned, remaining=%d", len(s.List("", "")))
	}
}

// TestCanonSigMerge: formatting-only signature variants merge at ingest.
func TestCanonSigMerge(t *testing.T) {
	s := openStore(t)
	s.Upsert(Finding{LensID: "l", Channel: ChannelAppFix, Signature: "tool:get_flow", Title: "x", LastSeen: 1})
	s.Upsert(Finding{LensID: "l", Channel: ChannelAppFix, Signature: "Tool: Get_Flow ", Title: "x", LastSeen: 2})
	if got := s.List("", ""); len(got) != 1 || got[0].Occurrences != 2 {
		t.Fatalf("formatting variants must merge into one ×2 finding: %+v", got)
	}
}

// TestClusterFindings: differently-worded findings about the same thing group.
func TestClusterFindings(t *testing.T) {
	findings := []Finding{
		{Title: "Bash disabled in context but offered", Severity: "high"},
		{Title: "Bash offered to model although disabled in this context"},
		{Title: "Glob times out on the Unity project tree"},
	}
	clusters := ClusterFindings(findings)
	if len(clusters) != 2 {
		t.Fatalf("the two Bash findings should cluster, Glob separate: got %d clusters", len(clusters))
	}
	if clusters[0].Size != 2 {
		t.Fatalf("first cluster should hold the two Bash findings: %+v", clusters[0])
	}
}

// TestRollupAppFix: same bug across workspaces collapses, summing occurrences.
func TestRollupAppFix(t *testing.T) {
	byWs := map[string][]Finding{
		"WS-A": {{Channel: ChannelAppFix, Signature: "tool:bash|disabled", Title: "bash", Severity: "med", Occurrences: 2, LastSeen: 5}},
		"WS-B": {{Channel: ChannelAppFix, Signature: "Tool: Bash | Disabled", Title: "bash", Severity: "high", Occurrences: 3, LastSeen: 9}},
	}
	out := RollupAppFix(byWs)
	if len(out) != 1 {
		t.Fatalf("the same canonical signature across workspaces must merge: %d rows", len(out))
	}
	if out[0].Occurrences != 5 {
		t.Fatalf("occurrences should sum to 5, got %d", out[0].Occurrences)
	}
	if out[0].Severity != "high" {
		t.Fatalf("merged row should keep the worst severity, got %s", out[0].Severity)
	}
	if len(out[0].Workspaces) != 2 {
		t.Fatalf("both workspaces should be recorded, got %v", out[0].Workspaces)
	}
}

// TestRunLogRoundTrip: append then read newest-first.
func TestRunLogRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := AppendRun(root, RunRecord{At: 1, Findings: 3}); err != nil {
		t.Fatal(err)
	}
	if err := AppendRun(root, RunRecord{At: 2, Findings: 7}); err != nil {
		t.Fatal(err)
	}
	runs, err := ReadRuns(root, 10)
	if err != nil || len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d err=%v", len(runs), err)
	}
	if runs[0].At != 2 {
		t.Fatalf("runs must be newest-first, got At=%d", runs[0].At)
	}
}

// TestCheckFilePointer: real file resolves, junk/escape does not.
func TestCheckFilePointer(t *testing.T) {
	repo := t.TempDir()
	if err := writeFile(filepath.Join(repo, "internal", "x.go"), "package x"); err != nil {
		t.Fatal(err)
	}
	if !CheckFilePointer(repo, "internal/x.go") {
		t.Fatal("existing file should validate")
	}
	if CheckFilePointer(repo, "internal/nope.go") {
		t.Fatal("missing file must not validate")
	}
	if CheckFilePointer(repo, "../escape.go") {
		t.Fatal("parent-escaping pointer must be rejected")
	}
	if CheckFilePointer("", "internal/x.go") {
		t.Fatal("blank repo must not validate")
	}
}
