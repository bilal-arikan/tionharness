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
	if _, err := s.SetStatus(f.ID, StatusDismissed, 110, nil); err != nil {
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
	if _, err := s.SetStatus(again.ID, StatusAccepted, 210, nil); err != nil {
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

// rescanned is a ScanEvidence stub: sessionID -> the time it was last scanned.
type rescanned map[string]int64

func (r rescanned) ScannedAfter(sessionIDs []string, after int64) bool {
	for _, id := range sessionIDs {
		if at, ok := r[id]; ok && at > after {
			return true
		}
	}
	return false
}

// TestMaintain: auto-verify applied-not-recurring; prune old resolved; keep regressed applied.
func TestMaintain(t *testing.T) {
	s := openStore(t)
	now := int64(1_000_000_000)
	old := now - DefaultAutoVerifyAge - 1
	veryOld := now - DefaultPruneAge - 1

	ev := &AppliedEntity{EntityType: "skill", EntityID: "s1"}
	applied, _ := s.Upsert(Finding{LensID: "l", Signature: "a", Title: "applied", Status: StatusApplied,
		AppliedEntity: ev, AppliedAt: old - 100, EvidenceSessionIDs: []string{"SES1"}, LastSeen: old})
	appliedReg, _ := s.Upsert(Finding{LensID: "l", Signature: "b", Title: "reg", Status: StatusApplied,
		AppliedEntity: ev, AppliedAt: old - 100, EvidenceSessionIDs: []string{"SES1"}, Regressed: true, LastSeen: old})
	s.Upsert(Finding{LensID: "l", Signature: "c", Title: "dismissed-old", Status: StatusDismissed, LastSeen: veryOld})
	s.Upsert(Finding{LensID: "l", Signature: "d", Title: "new-recent", Status: StatusNew, LastSeen: now})

	res, err := s.Maintain(now, 0, 0, rescanned{"SES1": old - 50})
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

// Age alone is not proof: an applied finding stays applied unless it names the
// entity it changed AND its sessions were re-scanned after the fix landed.
func TestMaintainAutoVerifyNeedsEvidenceAndRescan(t *testing.T) {
	now := int64(1_000_000_000)
	old := now - DefaultAutoVerifyAge - 1
	ev := &AppliedEntity{EntityType: "skill", EntityID: "s1"}

	cases := []struct {
		name    string
		finding Finding
		scans   ScanEvidence
	}{
		{"no evidence entity", Finding{AppliedAt: old - 100, EvidenceSessionIDs: []string{"SES1"}}, rescanned{"SES1": old - 50}},
		{"no appliedAt stamp", Finding{AppliedEntity: ev, EvidenceSessionIDs: []string{"SES1"}}, rescanned{"SES1": old - 50}},
		{"session not re-scanned", Finding{AppliedEntity: ev, AppliedAt: old - 100, EvidenceSessionIDs: []string{"SES1"}}, rescanned{"SES1": old - 500}},
		{"unknown session", Finding{AppliedEntity: ev, AppliedAt: old - 100, EvidenceSessionIDs: []string{"SES9"}}, rescanned{"SES1": now}},
		{"nil evidence source", Finding{AppliedEntity: ev, AppliedAt: old - 100, EvidenceSessionIDs: []string{"SES1"}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openStore(t)
			f := tc.finding
			f.LensID, f.Signature, f.Title, f.Status, f.LastSeen = "l", "a", "applied", StatusApplied, old
			if _, err := s.Upsert(f); err != nil {
				t.Fatal(err)
			}
			res, err := s.Maintain(now, 0, 0, tc.scans)
			if err != nil {
				t.Fatal(err)
			}
			if res.AutoVerified != 0 {
				t.Fatalf("must not auto-verify without evidence: %+v", res)
			}
			if got := s.List("", "")[0]; got.Status != StatusApplied {
				t.Fatalf("expected the finding to stay applied, got %s", got.Status)
			}
		})
	}
}

func TestLedgerScannedAfter(t *testing.T) {
	l, err := OpenLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Record(LedgerEntry{LensID: "lens-a", SessionID: "SES1", ScannedAt: 500}); err != nil {
		t.Fatal(err)
	}
	if !l.ScannedAfter([]string{"SES1"}, 400) {
		t.Fatal("a session scanned at 500 counts as scanned after 400")
	}
	if l.ScannedAfter([]string{"SES1"}, 500) {
		t.Fatal("ScannedAfter is strict: 500 is not after 500")
	}
	if l.ScannedAfter([]string{"SES2"}, 0) || l.ScannedAfter(nil, 0) {
		t.Fatal("unknown/empty sessions are not evidence")
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

// TestReset: default reset clears findings/runs/actions but KEEPS the ledger;
// deep reset clears the ledger too.
func TestReset(t *testing.T) {
	root := t.TempDir()
	// Seed all four artifacts.
	s, _ := OpenFindingStore(root)
	s.Upsert(Finding{LensID: "l", Signature: "a", Title: "A", Channel: ChannelWorkspaceOpt, LastSeen: 1})
	AppendWorkspaceActions(root, []Finding{{Signature: "a", Title: "A", Channel: ChannelWorkspaceOpt}})
	AppendRun(root, RunRecord{At: 1})
	led, _ := OpenLedger(root)
	led.Record(LedgerEntry{LensID: "l", SessionID: "S1", SeenFingerprint: "fp"})

	exists := func(rel string) bool { _, err := os.Stat(filepath.Join(root, rel)); return err == nil }
	if !exists(findingsRelPath) || !exists(runsRelPath) || !exists(workspaceActionsRelPath) || !exists(ledgerRelPath) {
		t.Fatal("seed: all four artifacts should exist")
	}

	// Default reset: findings/runs/actions gone, ledger kept.
	if err := Reset(root, false); err != nil {
		t.Fatal(err)
	}
	if exists(findingsRelPath) || exists(runsRelPath) || exists(workspaceActionsRelPath) {
		t.Fatal("default reset should remove findings/runs/actions")
	}
	if !exists(ledgerRelPath) {
		t.Fatal("default reset must KEEP the ledger (so old sessions aren't re-scanned)")
	}

	// Deep reset: ledger gone too.
	if err := Reset(root, true); err != nil {
		t.Fatal(err)
	}
	if exists(ledgerRelPath) {
		t.Fatal("deep reset should remove the ledger")
	}
	// Reset on an already-clean root is a no-op (no error).
	if err := Reset(root, true); err != nil {
		t.Fatalf("reset on clean root should be a no-op: %v", err)
	}
}

// TestFindingDelete: Delete drops a row entirely (distinct from Dismiss); an
// unknown id is a no-op (found=false, no error).
func TestFindingDelete(t *testing.T) {
	s := openStore(t)
	a, _ := s.Upsert(Finding{LensID: "l", Signature: "a", Title: "A", LastSeen: 1})
	s.Upsert(Finding{LensID: "l", Signature: "b", Title: "B", LastSeen: 2})
	if found, err := s.Delete(a.ID); err != nil || !found {
		t.Fatalf("delete existing: found=%v err=%v", found, err)
	}
	if got := s.List("", ""); len(got) != 1 || got[0].Signature != "b" {
		t.Fatalf("only B should remain: %+v", got)
	}
	if found, err := s.Delete("nope"); err != nil || found {
		t.Fatalf("delete unknown id should be a no-op: found=%v err=%v", found, err)
	}
}

// TestLedgerCompactBoundsFile: repeated re-scans of the same key append lines;
// Compact rewrites to one line per key so the file stops growing.
func TestLedgerCompactBoundsFile(t *testing.T) {
	root := t.TempDir()
	l, err := OpenLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	// Same (lens,session) recorded 20 times → 20 appended lines.
	for i := 0; i < 20; i++ {
		if err := l.Record(LedgerEntry{LensID: "x", SessionID: "S1", SeenFingerprint: "fp", ScannedAt: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, ledgerRelPath)
	before := lineCount(t, path)
	if before < 20 {
		t.Fatalf("expected >=20 appended lines before compact, got %d", before)
	}
	if err := l.Compact(); err != nil {
		t.Fatal(err)
	}
	if after := lineCount(t, path); after != 1 {
		t.Fatalf("compact should collapse to 1 line per key, got %d", after)
	}
	// Correctness survives compaction: the key is still known (skip on same fp).
	if l.NeedsScan("x", "S1", 0, "fp", "v1") {
		t.Fatal("compacted ledger must still remember the scanned key")
	}
}

// TestRunLogTrims: AppendRun keeps the file bounded to the newest maxRunRecords.
func TestRunLogTrims(t *testing.T) {
	root := t.TempDir()
	const extra = 3 // just enough over the cap to prove trimming, without O(n²) churn
	for i := 0; i < maxRunRecords+extra; i++ {
		if err := AppendRun(root, RunRecord{At: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if n := lineCount(t, filepath.Join(root, runsRelPath)); n != maxRunRecords {
		t.Fatalf("runs file should be capped at %d, got %d", maxRunRecords, n)
	}
	runs, err := ReadRuns(root, 5)
	if err != nil || len(runs) != 5 {
		t.Fatalf("expected 5 newest runs, got %d err=%v", len(runs), err)
	}
	// Newest kept: the very last At we wrote.
	if runs[0].At != int64(maxRunRecords+extra-1) {
		t.Fatalf("newest record should survive, got At=%d", runs[0].At)
	}
}

func lineCount(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
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
