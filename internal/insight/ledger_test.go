package insight

import (
	"path/filepath"
	"testing"
)

func TestFingerprintChangesWithContent(t *testing.T) {
	a := Fingerprint(10, 0)
	if a == Fingerprint(11, 0) {
		t.Fatal("fingerprint must change when message count changes")
	}
	if a == Fingerprint(10, 5) {
		t.Fatal("fingerprint must change when compaction watermark changes")
	}
	if a != Fingerprint(10, 0) {
		t.Fatal("fingerprint must be stable for identical input")
	}
}

func TestLedgerNeedsScanRules(t *testing.T) {
	root := t.TempDir()
	l, err := OpenLedger(root)
	if err != nil {
		t.Fatal(err)
	}

	// No entry yet -> must scan.
	if !l.NeedsScan("tool-errors", "SES1", 100, Fingerprint(5, 0), "lv1") {
		t.Fatal("first-time session should need a scan")
	}

	if err := l.Record(LedgerEntry{
		LensID: "tool-errors", SessionID: "SES1",
		SeenUpdatedAt: 100, SeenFingerprint: Fingerprint(5, 0), SeenLensVersion: "lv1", Status: "clean",
	}); err != nil {
		t.Fatal(err)
	}

	// Same UpdatedAt -> skip.
	if l.NeedsScan("tool-errors", "SES1", 100, Fingerprint(5, 0), "lv1") {
		t.Fatal("unchanged UpdatedAt should skip")
	}
	// UpdatedAt bumped but fingerprint identical (metadata-only) -> skip.
	if l.NeedsScan("tool-errors", "SES1", 200, Fingerprint(5, 0), "lv1") {
		t.Fatal("metadata-only bump (same fingerprint) should skip")
	}
	// UpdatedAt bumped and fingerprint changed (new message) -> scan.
	if !l.NeedsScan("tool-errors", "SES1", 200, Fingerprint(6, 0), "lv1") {
		t.Fatal("content change should trigger a re-scan")
	}
	// Different lens with no record -> backfill scan.
	if !l.NeedsScan("skill-usage-opt", "SES1", 100, Fingerprint(5, 0), "lv1") {
		t.Fatal("a new lens should backfill-scan an already-scanned session")
	}
}

func TestLedgerPersistsAcrossReopen(t *testing.T) {
	root := t.TempDir()
	l, err := OpenLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Record(LedgerEntry{
		LensID: "tool-errors", SessionID: "SES1",
		SeenUpdatedAt: 100, SeenFingerprint: Fingerprint(5, 0),
	}); err != nil {
		t.Fatal(err)
	}
	// Overwrite with a newer scan (append-only, last wins).
	if err := l.Record(LedgerEntry{
		LensID: "tool-errors", SessionID: "SES1",
		SeenUpdatedAt: 300, SeenFingerprint: Fingerprint(9, 0), SeenLensVersion: "lv1",
	}); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.NeedsScan("tool-errors", "SES1", 300, Fingerprint(9, 0), "lv1") {
		t.Fatal("reopened ledger should reflect the latest recorded scan")
	}
	if _, err := filepath.Abs(filepath.Join(root, ledgerRelPath)); err != nil {
		t.Fatal(err)
	}
}
