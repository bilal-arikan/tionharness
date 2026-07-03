package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedRead writes content to rel under the sandbox and records a freshness
// baseline (as if the agent had Read it), so a subsequent patch passes the guard.
func seedRead(t *testing.T, sb Sandbox, tr *ReadTracker, rel, content string) {
	t.Helper()
	abs, err := sb.Resolve(rel)
	if err != nil {
		t.Fatalf("resolve %s: %v", rel, err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	tr.Record(abs, ReadRecord{Size: int64(len(content)), Sum: contentSum([]byte(content))})
}

func TestApplyPatchUpdate(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.txt", "line1\nline2\nline3\n")

	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+line2x\n line3\n"
	tool := NewFSApplyPatchTool(sb, tr)
	out, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch}))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(out, "patched f.txt") {
		t.Fatalf("unexpected result: %q", out)
	}
	abs, _ := sb.Resolve("f.txt")
	got, _ := os.ReadFile(abs)
	if string(got) != "line1\nline2x\nline3\n" {
		t.Fatalf("content = %q", string(got))
	}
}

func TestApplyPatchCreateAndDelete(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	ctx := context.Background()
	tool := NewFSApplyPatchTool(sb, tr)

	// Create a brand-new file (no prior read required).
	create := "--- /dev/null\n+++ b/new/created.txt\n@@ -0,0 +1,2 @@\n+alpha\n+beta\n"
	if _, err := tool.Call(ctx, mustJSON(t, map[string]any{"patch": create})); err != nil {
		t.Fatalf("create: %v", err)
	}
	abs, _ := sb.Resolve("new/created.txt")
	if got, _ := os.ReadFile(abs); string(got) != "alpha\nbeta\n" {
		t.Fatalf("created content = %q", string(got))
	}

	// Delete an existing (read) file.
	seedRead(t, sb, tr, "gone.txt", "x\ny\n")
	del := "--- a/gone.txt\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-x\n-y\n"
	if _, err := tool.Call(ctx, mustJSON(t, map[string]any{"patch": del})); err != nil {
		t.Fatalf("delete: %v", err)
	}
	goneAbs, _ := sb.Resolve("gone.txt")
	if _, err := os.Stat(goneAbs); !os.IsNotExist(err) {
		t.Fatalf("file should be deleted, stat err = %v", err)
	}
}

func TestApplyPatchMismatchRejects(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.txt", "aaa\nbbb\nccc\n")

	// Context "zzz" is not in the file — the patch must be rejected and the file
	// left untouched (no partial application).
	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n zzz\n-bbb\n+BBB\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	abs, _ := sb.Resolve("f.txt")
	if got, _ := os.ReadFile(abs); string(got) != "aaa\nbbb\nccc\n" {
		t.Fatalf("file must be unchanged on mismatch, got %q", string(got))
	}
}

func TestApplyPatchFreshnessGuard(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	// File exists on disk but was never Read → the guard must block the patch.
	abs, _ := sb.Resolve("f.txt")
	os.WriteFile(abs, []byte("a\nb\n"), 0o644)

	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,2 +1,2 @@\n a\n-b\n+B\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err == nil {
		t.Fatal("expected freshness-guard error for unread file, got nil")
	}
}

func TestApplyPatchMultiHunk(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.txt", "1\n2\n3\n4\n5\n6\n")

	// Two hunks in one file, well apart, matched by context (line numbers ignored).
	patch := "--- a/f.txt\n+++ b/f.txt\n" +
		"@@ -1,2 +1,2 @@\n 1\n-2\n+two\n" +
		"@@ -5,2 +5,2 @@\n 5\n-6\n+six\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err != nil {
		t.Fatalf("multi-hunk: %v", err)
	}
	abs, _ := sb.Resolve("f.txt")
	if got, _ := os.ReadFile(abs); string(got) != "1\ntwo\n3\n4\n5\nsix\n" {
		t.Fatalf("content = %q", string(got))
	}
}
