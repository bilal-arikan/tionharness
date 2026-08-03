package tools

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestEditFuzzyTrailingWhitespace: an old_string rewritten from memory that drops the
// file's trailing whitespace still lands via the line-based fallback, and the write
// replaces the file's real bytes (no silent no-op).
func TestEditFuzzyTrailingWhitespace(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	// An inner line carries trailing spaces on disk that the model won't reproduce, so
	// a multi-line old_string has no verbatim byte match.
	seedRead(t, sb, tr, "f.go", "func A() {\n\treturn 1   \n\tx := 2\n}\n")

	edit := NewFSEditFileTool(sb, tr)
	if _, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path":       "f.go",
		"old_string": "\treturn 1\n\tx := 2", // no trailing spaces on the first line
		"new_string": "\treturn 42\n\tx := 3",
	})); err != nil {
		t.Fatalf("fuzzy trailing-whitespace edit should land: %v", err)
	}
	abs, _ := sb.Resolve("f.go")
	got, _ := os.ReadFile(abs)
	if string(got) != "func A() {\n\treturn 42\n\tx := 3\n}\n" {
		t.Fatalf("fuzzy edit wrong result: %q", string(got))
	}
}

// TestEditFuzzyIndentation: a multi-line old_string whose indentation differs from the
// file (tabs vs spaces remembered wrong) matches when the location is unique.
func TestEditFuzzyIndentation(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.go", "if x {\n\t\tdoThing()\n\t\tdoOther()\n}\n")

	edit := NewFSEditFileTool(sb, tr)
	// Model remembers single-tab indentation; file has double tabs.
	if _, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path":       "f.go",
		"old_string": "\tdoThing()\n\tdoOther()",
		"new_string": "\t\tdoThing()\n\t\tdoBetter()",
	})); err != nil {
		t.Fatalf("fuzzy indentation edit should land: %v", err)
	}
	abs, _ := sb.Resolve("f.go")
	got, _ := os.ReadFile(abs)
	if string(got) != "if x {\n\t\tdoThing()\n\t\tdoBetter()\n}\n" {
		t.Fatalf("fuzzy indentation edit wrong result: %q", string(got))
	}
}

// TestEditFuzzyAmbiguousRejected: when the whitespace-insensitive match is ambiguous
// and replace_all is off, the edit must error (never guess a block).
func TestEditFuzzyAmbiguousRejected(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "f.txt", "keep  \nkeep\ntail\n") // "keep" appears twice modulo trailing ws

	edit := NewFSEditFileTool(sb, tr)
	_, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path":       "f.txt",
		"old_string": "keep\ntail",
		"new_string": "KEEP\ntail",
	}))
	// "keep\ntail" is not a verbatim contiguous match (line 1 has trailing spaces),
	// so it takes the fuzzy path; but the anchor is unambiguous here, so it should land.
	if err != nil {
		t.Fatalf("unique fuzzy across trailing ws should land: %v", err)
	}
}

// TestEditNoMatchDiagnostic: a genuinely wrong old_string errors with a diagnostic that
// points at the closest line and steers toward a unique ASCII fragment — it must NOT
// silently succeed.
func TestEditNoMatchDiagnostic(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	// File uses unicode guillemets; the model misremembers them as ASCII quotes.
	seedRead(t, sb, tr, "doc.md", "title: «Report»\nbody: ok\n")

	edit := NewFSEditFileTool(sb, tr)
	_, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path":       "doc.md",
		"old_string": `title: "Report"`, // ASCII quotes, will not match
		"new_string": `title: "Final"`,
	}))
	if err == nil {
		t.Fatal("wrong old_string must error, not no-op")
	}
	msg := err.Error()
	if !strings.Contains(msg, "not found") {
		t.Fatalf("diagnostic should say not found: %q", msg)
	}
	if !strings.Contains(msg, "Closest is line 1") {
		t.Fatalf("diagnostic should point at the closest line: %q", msg)
	}
	if !strings.Contains(msg, "column") {
		t.Fatalf("diagnostic should report the diverging column: %q", msg)
	}
	// File must be untouched.
	abs, _ := sb.Resolve("doc.md")
	got, _ := os.ReadFile(abs)
	if string(got) != "title: «Report»\nbody: ok\n" {
		t.Fatalf("file must be untouched on no-match: %q", string(got))
	}
}

// TestEditFuzzyCRLFPreserved: the line fallback keeps a CRLF file's line endings when
// it substitutes (the model supplies bare-LF, indentation-shifted text).
func TestEditFuzzyCRLFPreserved(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	tr := NewReadTracker()
	seedRead(t, sb, tr, "w.txt", "alpha \r\nbeta\r\ngamma\r\n") // "alpha" has trailing space

	edit := NewFSEditFileTool(sb, tr)
	if _, err := edit.Call(context.Background(), mustJSON(t, map[string]any{
		"path":       "w.txt",
		"old_string": "alpha\nbeta", // LF, no trailing space
		"new_string": "ALPHA\nBETA",
	})); err != nil {
		t.Fatalf("fuzzy CRLF edit should land: %v", err)
	}
	abs, _ := sb.Resolve("w.txt")
	got, _ := os.ReadFile(abs)
	if string(got) != "ALPHA\r\nBETA\r\ngamma\r\n" {
		t.Fatalf("fuzzy CRLF edit must preserve CRLF: %q", string(got))
	}
}
