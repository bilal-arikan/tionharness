package db

import (
	"os"
	"path/filepath"
	"testing"
)

// TestImportMediaSource_InsideWorkspace verifies a file already under the
// workspace is referenced by a clean relative path with no copy.
func TestImportMediaSource_InsideWorkspace(t *testing.T) {
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Write a file inside <workspace>/shots/pic.png.
	wsFile := filepath.Join(root, "workspace", "shots", "pic.png")
	if err := os.MkdirAll(filepath.Dir(wsFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wsFile, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := d.ImportMediaSource("SES1", wsFile)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if rel != "shots/pic.png" {
		t.Fatalf("want relative shots/pic.png, got %q", rel)
	}
	// No copy was made into artifacts/.
	if _, err := os.Stat(filepath.Join(root, "workspace", "artifacts")); !os.IsNotExist(err) {
		t.Errorf("expected no artifacts copy dir, stat err = %v", err)
	}
}

// TestImportMediaSource_OutsideWorkspace verifies a file outside the workspace
// (e.g. a screenshot in Downloads) is copied in under artifacts/<session>/ and
// the returned relative path points at the copy.
func TestImportMediaSource_OutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// A file in a sibling "Downloads" dir, outside <workspace>/.
	outside := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(outside, []byte("OUTSIDEPNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	rel, err := d.ImportMediaSource("SES7", outside)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	wantPrefix := "artifacts/SES7/media-"
	if len(rel) < len(wantPrefix) || rel[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("want copy under %s..., got %q", wantPrefix, rel)
	}
	// The copied file exists under the workspace with the original bytes.
	copied := filepath.Join(root, "workspace", filepath.FromSlash(rel))
	b, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	if string(b) != "OUTSIDEPNG" {
		t.Errorf("copied bytes = %q", string(b))
	}
}

// TestImportMediaSource_Missing verifies a non-existent source path is rejected.
func TestImportMediaSource_Missing(t *testing.T) {
	root := t.TempDir()
	d, err := Open(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := d.ImportMediaSource("SES1", filepath.Join(root, "nope.png")); err == nil {
		t.Fatal("expected error for missing source file")
	}
	if _, err := d.ImportMediaSource("SES1", ""); err == nil {
		t.Fatal("expected error for empty source path")
	}
}
