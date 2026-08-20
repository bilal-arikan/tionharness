package insight

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadRunsLegacyRecords: records written before the session pairing existed
// carry neither id nor sessionId. They must still parse and be returned (newest
// first) alongside new ones — the format change is additive, not breaking.
func TestReadRunsLegacyRecords(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, runsRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := `{"at":100,"durationMs":10,"trigger":"auto","lensIds":["tool-errors"],"sessions":3,"analyzed":2,"skipped":1,"prefiltered":0,"findings":4,"errors":0}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy log: %v", err)
	}
	// Append a NEW record on top of the legacy file — the two shapes coexist.
	if err := AppendRun(root, RunRecord{ID: "IRUN-1", SessionID: "SES-1", At: 200, Findings: 1}); err != nil {
		t.Fatalf("append: %v", err)
	}

	runs, err := ReadRuns(root, 10)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(runs))
	}
	if runs[0].ID != "IRUN-1" || runs[0].SessionID != "SES-1" {
		t.Fatalf("newest record lost its identity: %+v", runs[0])
	}
	old := runs[1]
	if old.ID != "" || old.SessionID != "" {
		t.Fatalf("legacy record must stay unpaired, got %+v", old)
	}
	if old.At != 100 || old.Findings != 4 || old.Analyzed != 2 || old.Trigger != "auto" {
		t.Fatalf("legacy record fields lost: %+v", old)
	}
	if len(old.LensIDs) != 1 || old.LensIDs[0] != "tool-errors" {
		t.Fatalf("legacy lens ids lost: %+v", old.LensIDs)
	}
}
