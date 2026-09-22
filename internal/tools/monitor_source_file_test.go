package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestFileSource creates a file with initial content and binds a source to it
// through an unconfined sandbox rooted at the temp dir.
func newTestFileSource(t *testing.T, initial string) (MonitorSource, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	src, err := NewFileSource(NewSandbox(dir), path)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	t.Cleanup(src.Close)
	return src, path
}

// appendTo adds content to an existing file.
func appendTo(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// TestFileSourceStartsAtEndOfFile pins the "report what happens from NOW on"
// contract: the backlog that existed when the monitor was armed is not replayed.
func TestFileSourceStartsAtEndOfFile(t *testing.T) {
	src, path := newTestFileSource(t, "old line 1\nold line 2\n")

	evs, done, _, err := src.Poll(context.Background())
	if err != nil || done {
		t.Fatalf("Poll: err=%v done=%v", err, done)
	}
	if len(evs) != 0 {
		t.Fatalf("the pre-existing backlog was replayed: %+v", evs)
	}

	appendTo(t, path, "new line\n")
	evs, _, _, err = src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 1 || evs[0].Payload != "new line" {
		t.Fatalf("expected the appended line only, got %+v", evs)
	}
}

// TestFileSourceJoinsLineSplitAcrossPolls: a write that lands mid-line must not
// be reported as two half lines, or a regex anchored on the whole line breaks.
func TestFileSourceJoinsLineSplitAcrossPolls(t *testing.T) {
	src, path := newTestFileSource(t, "")
	src.Poll(context.Background())

	appendTo(t, path, "ERROR: some")
	evs, _, _, err := src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 0 {
		t.Fatalf("an incomplete line was reported early: %+v", evs)
	}

	appendTo(t, path, "thing failed\n")
	evs, _, _, err = src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 1 || evs[0].Payload != "ERROR: something failed" {
		t.Fatalf("the split line was not rejoined: %+v", evs)
	}
}

// TestFileSourceReportsRemovalOnce pins the terminal contract: a deleted file
// ends the monitor, and the reason is produced exactly once.
func TestFileSourceReportsRemovalOnce(t *testing.T) {
	src, path := newTestFileSource(t, "")
	src.Poll(context.Background())

	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	_, done, reason, err := src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if !done {
		t.Fatal("a removed file must end the monitor")
	}
	if !strings.Contains(reason, "removed") {
		t.Fatalf("reason %q does not explain the removal", reason)
	}
}

// TestFileSourceTruncationRestartsFromZero: a rotated log must not leave the
// cursor past EOF, silently ignoring everything written afterwards.
func TestFileSourceTruncationRestartsFromZero(t *testing.T) {
	src, path := newTestFileSource(t, "long initial content that will be cut\n")
	src.Poll(context.Background())

	if err := os.WriteFile(path, []byte("after rotate\n"), 0o644); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	evs, _, _, err := src.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(evs) != 1 || evs[0].Payload != "after rotate" {
		t.Fatalf("content written after truncation was missed: %+v", evs)
	}
}

// TestFileSourceRejectsMissingPath: arming on a path that does not exist must
// fail NOW, not produce a monitor that can never fire.
func TestFileSourceRejectsMissingPath(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewFileSource(NewSandbox(dir), filepath.Join(dir, "nope.log")); err == nil {
		t.Fatal("a missing path must be rejected at arm time")
	}
	if _, err := NewFileSource(NewSandbox(dir), dir); err == nil {
		t.Fatal("a directory must be rejected at arm time")
	}
}

// TestFileSourceHonoursConfinedSandbox: the file source must not become a way
// around the sandbox boundary the fs tools enforce.
func TestFileSourceHonoursConfinedSandbox(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.log")
	if err := os.WriteFile(outside, []byte("x\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := NewFileSource(NewConfinedSandbox(root), outside); err == nil {
		t.Fatal("a confined sandbox must reject a path outside its root")
	}
}
