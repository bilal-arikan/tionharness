package tools

import (
	"context"
	"os"
	"testing"
)

// TestEditMatchesCRLFFile: a multi-line old_string with bare LF (what the model
// copies from Read) must still match a CRLF file on disk, and the write must keep
// the file's CRLF line ending. Guards the Windows "Edit never matches" bug.
func TestEditMatchesCRLFFile(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	crlf := "func main() {\r\n\tx := 1\r\n\ty := 2\r\n}\r\n"
	seedRead(t, sb, tr, "main.go", crlf)

	edit := NewFSEditFileTool(sb, tr)
	if _, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path":       "main.go",
		"old_string": "x := 1\n\ty := 2", // bare LF, multi-line
		"new_string": "x := 10\n\ty := 20",
	})); err != nil {
		t.Fatalf("edit must match across the CRLF/LF gap: %v", err)
	}
	abs, _ := sb.Resolve("main.go")
	got, _ := os.ReadFile(abs)
	want := "func main() {\r\n\tx := 10\r\n\ty := 20\r\n}\r\n"
	if string(got) != want {
		t.Fatalf("edit must land AND preserve CRLF:\n got %q\nwant %q", string(got), want)
	}
}

// TestEditLFFileUnchangedBehavior: an LF file still edits (and stays LF) — the
// CRLF path must not regress the normal case.
func TestEditLFFileUnchangedBehavior(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.txt", "a\nb\nc\n")
	edit := NewFSEditFileTool(sb, tr)
	if _, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path": "f.txt", "old_string": "b\nc", "new_string": "B\nC",
	})); err != nil {
		t.Fatalf("LF edit: %v", err)
	}
	abs, _ := sb.Resolve("f.txt")
	got, _ := os.ReadFile(abs)
	if string(got) != "a\nB\nC\n" {
		t.Fatalf("LF file must stay LF: %q", string(got))
	}
}

// TestApplyPatchMatchesCRLFFile: apply_patch must locate hunks in a CRLF file
// (patch context is LF) and preserve CRLF on write.
func TestApplyPatchMatchesCRLFFile(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.txt", "line1\r\nline2\r\nline3\r\n")

	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+line2x\n line3\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err != nil {
		t.Fatalf("patch must match a CRLF file: %v", err)
	}
	abs, _ := sb.Resolve("f.txt")
	got, _ := os.ReadFile(abs)
	if string(got) != "line1\r\nline2x\r\nline3\r\n" {
		t.Fatalf("patch must land AND preserve CRLF: %q", string(got))
	}
}

// TestToCRLF: bare-LF and mixed input both normalize to uniform CRLF.
func TestToCRLF(t *testing.T) {
	if got := toCRLF("a\nb\n"); got != "a\r\nb\r\n" {
		t.Fatalf("LF→CRLF: %q", got)
	}
	if got := toCRLF("a\r\nb\n"); got != "a\r\nb\r\n" {
		t.Fatalf("mixed→CRLF: %q", got)
	}
}
