package backup

import (
	"archive/zip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFile creates a file with content, making parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunOnceArchivesAndPrunes(t *testing.T) {
	tmp := t.TempDir()
	wsDir := filepath.Join(tmp, "ws", "WS1")
	writeFile(t, filepath.Join(wsDir, "store", "a.json"), `{"x":1}`)
	writeFile(t, filepath.Join(wsDir, "config", "instructions.md"), "hello")

	backupRoot := filepath.Join(tmp, "backups")
	m := New(tmp, func() []Target {
		return []Target{{ID: "WS1", Name: "Default", Dir: wsDir}}
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Configure(Config{Enabled: false, IntervalHours: 24, Retain: 2, Dir: backupRoot})

	// First pass writes one archive.
	res, err := m.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(res.Archives) != 1 {
		t.Fatalf("want 1 archive, got %d (failures=%v)", len(res.Archives), res.Failures)
	}
	if res.Archives[0].Bytes <= 0 {
		t.Fatalf("archive is empty")
	}

	// Verify the zip actually contains the workspace files with relative paths.
	zr, err := zip.OpenReader(res.Archives[0].Path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	got := map[string]bool{}
	for _, f := range zr.File {
		got[f.Name] = true
	}
	zr.Close()
	for _, want := range []string{"store/a.json", "config/instructions.md"} {
		if !got[want] {
			t.Errorf("zip missing %q (have %v)", want, got)
		}
	}

	// Retention: 3 more passes with distinct stamps would normally need time to
	// advance; instead drop extra archives manually then prune via another run.
	wsBackupDir := filepath.Join(backupRoot, "WS1")
	writeFile(t, filepath.Join(wsBackupDir, "WS1-20000101-000000.zip"), "old1")
	writeFile(t, filepath.Join(wsBackupDir, "WS1-20000102-000000.zip"), "old2")
	prune(wsBackupDir, "WS1", 2, nil)

	entries, _ := os.ReadDir(wsBackupDir)
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("after prune want 2 archives, got %d", count)
	}
}

func TestZipUnzipRoundTrip(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "store", "a.json"), `{"x":1}`)
	writeFile(t, filepath.Join(src, "config", "instructions.md"), "hello world")
	writeFile(t, filepath.Join(src, "ws-settings.json"), `{"name":"X"}`)

	archive := filepath.Join(t.TempDir(), "out.zip")
	if _, err := zipDir(src, archive, filepath.Join(src, "backups")); err != nil {
		t.Fatalf("zipDir: %v", err)
	}

	dst := t.TempDir()
	if err := Unzip(archive, dst); err != nil {
		t.Fatalf("Unzip: %v", err)
	}
	for rel, want := range map[string]string{
		"store/a.json":           `{"x":1}`,
		"config/instructions.md": "hello world",
		"ws-settings.json":       `{"name":"X"}`,
	} {
		got, err := os.ReadFile(filepath.Join(dst, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("missing %q: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%q = %q, want %q", rel, got, want)
		}
	}
}

// TestUnzipRejectsZipSlip feeds Unzip a malicious archive whose entry escapes
// the destination via "..". The zip-slip guard (archive.go) must reject it and
// nothing may be written outside destDir. The existing tests only covered
// path-name traversal at the API layer (ResolveArchive/DeleteArchive), never the
// guard inside Unzip itself. See _Docs/34-YEDEKLEME.md (zip-slip korumalı).
func TestUnzipRejectsZipSlip(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "evil.zip")
	zf, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	w, err := zw.Create("../escaped.txt") // escapes destDir
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("pwned")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zf.Close()

	dst := filepath.Join(t.TempDir(), "dest")
	err = Unzip(archive, dst)
	if err == nil {
		t.Fatal("expected the zip-slip entry to be rejected")
	}
	if !strings.Contains(err.Error(), "unsafe archive entry") {
		t.Fatalf("expected an unsafe-entry error, got %v", err)
	}
	// The escaping file must not exist next to (one level above) destDir.
	escaped := filepath.Join(filepath.Dir(dst), "escaped.txt")
	if _, statErr := os.Stat(escaped); !os.IsNotExist(statErr) {
		t.Fatalf("zip-slip wrote outside destDir (stat err = %v)", statErr)
	}
}

func TestResolveArchiveRejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	m := New(tmp, func() []Target { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Configure(Config{Enabled: false, Dir: filepath.Join(tmp, "backups")})

	// A real archive to resolve positively.
	writeFile(t, filepath.Join(tmp, "backups", "WS1", "WS1-20260625-000000.zip"), "z")

	if _, err := m.ResolveArchive("WS1", "WS1-20260625-000000.zip"); err != nil {
		t.Fatalf("valid archive rejected: %v", err)
	}
	for _, bad := range []string{"../secret.zip", "..\\secret.zip", "sub/evil.zip", "nope.zip", "WS1-x.txt"} {
		if _, err := m.ResolveArchive("WS1", bad); err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
	}
}

func TestDeleteArchive(t *testing.T) {
	tmp := t.TempDir()
	m := New(tmp, func() []Target { return nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	m.Configure(Config{Enabled: false, Dir: filepath.Join(tmp, "backups")})

	p := filepath.Join(tmp, "backups", "WS1", "WS1-20260625-000000.zip")
	writeFile(t, p, "z")

	// Traversal is rejected and leaves the real file intact.
	if err := m.DeleteArchive("WS1", "../evil.zip"); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file should still exist after rejected delete: %v", err)
	}
	// Valid delete removes the file.
	if err := m.DeleteArchive("WS1", "WS1-20260625-000000.zip"); err != nil {
		t.Fatalf("DeleteArchive: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file should be gone, stat err = %v", err)
	}
}

func TestZipDirSkipsBackupsRoot(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "data.txt"), "keep")
	// A backups dir nested inside the source must be excluded.
	backups := filepath.Join(tmp, "backups")
	writeFile(t, filepath.Join(backups, "old.zip"), "skip")

	dst := filepath.Join(t.TempDir(), "out.zip")
	if _, err := zipDir(tmp, dst, backups); err != nil {
		t.Fatalf("zipDir: %v", err)
	}
	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == "backups/old.zip" {
			t.Fatalf("backups root was not skipped")
		}
	}
}
