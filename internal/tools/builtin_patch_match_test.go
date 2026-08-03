package tools

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestApplyPatchFuzzyTrailingWhitespace: a hunk whose context line lacks the file's
// trailing whitespace still applies via the whitespace-tolerant fallback.
func TestApplyPatchFuzzyTrailingWhitespace(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	// "line1" carries trailing spaces on disk; the patch context omits them.
	seedRead(t, sb, tr, "f.txt", "line1   \nline2\nline3\n")

	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n line1\n-line2\n+line2x\n line3\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err != nil {
		t.Fatalf("fuzzy trailing-ws patch should apply: %v", err)
	}
	abs, _ := sb.Resolve("f.txt")
	got, _ := os.ReadFile(abs)
	if string(got) != "line1\nline2x\nline3\n" {
		t.Fatalf("fuzzy patch wrong result: %q", string(got))
	}
}

// TestApplyPatchFuzzyIndentation: a hunk whose context/removed indentation differs from
// disk applies when the location is unique.
func TestApplyPatchFuzzyIndentation(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.go", "if x {\n\t\treturn 1\n}\n") // double-tab indent on disk

	// Patch remembers single-tab indentation.
	patch := "--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,3 @@\n if x {\n-\treturn 1\n+\t\treturn 2\n }\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err != nil {
		t.Fatalf("fuzzy indentation patch should apply: %v", err)
	}
	abs, _ := sb.Resolve("f.go")
	got, _ := os.ReadFile(abs)
	if string(got) != "if x {\n\t\treturn 2\n}\n" {
		t.Fatalf("fuzzy indentation patch wrong result: %q", string(got))
	}
}

// TestApplyPatchFuzzyAmbiguousRejected: an ambiguous whitespace-insensitive context
// must be rejected rather than applied to a guessed block.
func TestApplyPatchFuzzyAmbiguousRejected(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	// "dup" appears twice, BOTH with trailing whitespace so neither matches verbatim —
	// only the fuzzy path fires, and it finds two candidates (ambiguous).
	seedRead(t, sb, tr, "f.txt", "dup   \nmid\ndup\t\n")

	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,1 +1,1 @@\n-dup\n+DUP\n"
	tool := NewFSApplyPatchTool(sb, tr)
	_, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch}))
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous fuzzy hunk must be rejected, got %v", err)
	}
	// File untouched.
	abs, _ := sb.Resolve("f.txt")
	got, _ := os.ReadFile(abs)
	if string(got) != "dup   \nmid\ndup\t\n" {
		t.Fatalf("file must be untouched on ambiguous reject: %q", string(got))
	}
}

// TestApplyPatchNoMatchDiagnostic: a genuinely wrong hunk errors with a diagnostic that
// points at the closest line — and never half-applies.
func TestApplyPatchNoMatchDiagnostic(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "doc.md", "title: «Report»\nbody: ok\n")

	patch := "--- a/doc.md\n+++ b/doc.md\n@@ -1,1 +1,1 @@\n-title: \"Report\"\n+title: \"Final\"\n"
	tool := NewFSApplyPatchTool(sb, tr)
	_, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch}))
	if err == nil {
		t.Fatal("wrong hunk must error, not no-op")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Closest is line 1") || !strings.Contains(msg, "column") {
		t.Fatalf("diagnostic should point at closest line + column: %q", msg)
	}
	abs, _ := sb.Resolve("doc.md")
	got, _ := os.ReadFile(abs)
	if string(got) != "title: «Report»\nbody: ok\n" {
		t.Fatalf("file must be untouched on no-match: %q", string(got))
	}
}

// TestApplyPatchFuzzyCRLFPreserved: the fuzzy fallback keeps a CRLF file's endings.
func TestApplyPatchFuzzyCRLFPreserved(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "w.txt", "alpha   \r\nbeta\r\ngamma\r\n") // "alpha" has trailing spaces

	patch := "--- a/w.txt\n+++ b/w.txt\n@@ -1,3 +1,3 @@\n alpha\n-beta\n+BETA\n gamma\n"
	tool := NewFSApplyPatchTool(sb, tr)
	if _, err := tool.Call(context.Background(), mustJSON(t, map[string]any{"patch": patch})); err != nil {
		t.Fatalf("fuzzy CRLF patch should apply: %v", err)
	}
	abs, _ := sb.Resolve("w.txt")
	got, _ := os.ReadFile(abs)
	// The fuzzy path rewrites the matched region (incl. context lines) to the patch's
	// text, so "alpha"'s trailing spaces are normalized away — but CRLF is preserved.
	if string(got) != "alpha\r\nBETA\r\ngamma\r\n" {
		t.Fatalf("fuzzy CRLF patch must preserve CRLF: %q", string(got))
	}
}
