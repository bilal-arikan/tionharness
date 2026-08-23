package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSandboxResolve(t *testing.T) {
	root := t.TempDir()
	sb := NewSandbox(root)

	if !sb.Ready() {
		t.Fatal("sandbox should be ready")
	}

	// In-bounds paths resolve under root.
	got, err := sb.Resolve("sub/file.txt")
	if err != nil {
		t.Fatalf("resolve in-bounds: %v", err)
	}
	if !strings.HasPrefix(got, filepath.Clean(root)) {
		t.Fatalf("resolved %q not under root %q", got, root)
	}

	// Empty path resolves to the root itself.
	if got, err := sb.Resolve(""); err != nil || got != filepath.Clean(root) {
		t.Fatalf("resolve root: got %q err %v", got, err)
	}

	// Confinement is disabled: ".." escapes resolve without error (relative to
	// the base dir).
	if _, err := sb.Resolve("../escape"); err != nil {
		t.Fatalf("escape should be allowed now: %v", err)
	}

	// Absolute paths are honoured as-is.
	abs := filepath.Join(root, "x")
	if got, err := sb.Resolve(abs); err != nil || got != filepath.Clean(abs) {
		t.Fatalf("absolute path: got %q err %v", got, err)
	}
}

func TestSandboxNotReady(t *testing.T) {
	sb := NewSandbox("")
	if sb.Ready() {
		t.Fatal("empty sandbox should not report a configured base dir")
	}
	// With no base dir, a relative path resolves against the process cwd, and an
	// absolute path is honoured as-is.
	if got, err := sb.Resolve("anything"); err != nil || !filepath.IsAbs(got) {
		t.Fatalf("relative resolve without base: got %q err %v", got, err)
	}
	got, err := sb.Resolve(filepath.Join(string(filepath.Separator), "tmp", "x"))
	if runtime.GOOS == "windows" {
		if err == nil || got != "" {
			t.Fatalf("slash-rooted Windows path: got %q err %v, want explicit error", got, err)
		}
	} else if err != nil || !filepath.IsAbs(got) {
		t.Fatalf("absolute resolve without base: got %q err %v", got, err)
	}
}

func TestFSWriteReadEdit(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	ctx := context.Background()
	tr := NewReadTracker()

	write := NewFSWriteFileTool(sb, tr)
	if _, err := write.Call(ctx, mustJSON(t, map[string]any{
		"path":    "notes/hello.txt",
		"content": "alpha beta gamma",
	})); err != nil {
		t.Fatalf("write: %v", err)
	}

	read := NewFSReadFileTool(sb, tr)
	out, err := read.Call(ctx, mustJSON(t, map[string]any{"path": "notes/hello.txt"}))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "alpha beta gamma") {
		t.Fatalf("read got %q", out)
	}

	edit := NewFSEditFileTool(sb, tr)
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path":       "notes/hello.txt",
		"old_string": "beta",
		"new_string": "BETA",
	})); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, _ = read.Call(ctx, mustJSON(t, map[string]any{"path": "notes/hello.txt"}))
	if !strings.Contains(out, "alpha BETA gamma") {
		t.Fatalf("after edit got %q", out)
	}

	// Editing a non-unique string without replace_all must fail. The file was just
	// read above, so it clears the freshness guard and fails on the non-unique count.
	_ = os.WriteFile(filepath.Join(sb.Root, "dup.txt"), []byte("x x x"), 0o644)
	if _, err := read.Call(ctx, mustJSON(t, map[string]any{"path": "dup.txt"})); err != nil {
		t.Fatalf("read dup: %v", err)
	}
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path": "dup.txt", "old_string": "x", "new_string": "y",
	})); err == nil {
		t.Fatal("expected non-unique edit to fail without replace_all")
	}
}

// TestFSReadWindow covers line-numbered output and the offset/limit window.
func TestFSReadWindow(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	ctx := context.Background()
	read := NewFSReadFileTool(sb, nil)

	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, fmt.Sprintf("line-%d", i))
	}
	_ = os.WriteFile(filepath.Join(sb.Root, "f.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	// Full read is line-numbered.
	out, err := read.Call(ctx, mustJSON(t, map[string]any{"path": "f.txt"}))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(out, "     1\tline-1") || !strings.Contains(out, "    10\tline-10") {
		t.Fatalf("expected numbered lines, got:\n%s", out)
	}

	// offset+limit selects a window and reports continuation.
	out, _ = read.Call(ctx, mustJSON(t, map[string]any{"path": "f.txt", "offset": 3, "limit": 2}))
	if !strings.Contains(out, "     3\tline-3") || !strings.Contains(out, "     4\tline-4") {
		t.Fatalf("window should contain lines 3-4, got:\n%s", out)
	}
	if strings.Contains(out, "line-5") || strings.Contains(out, "line-2") {
		t.Fatalf("window leaked out-of-range lines:\n%s", out)
	}

	// offset past the end is reported, not an error.
	out, _ = read.Call(ctx, mustJSON(t, map[string]any{"path": "f.txt", "offset": 99}))
	if !strings.Contains(out, "past the end") {
		t.Fatalf("expected past-end note, got %q", out)
	}
}

// TestFSEditToleratesLineNumbers covers the Edit fallback that strips cat -n line
// prefixes copied from Read output so a numbered paste still matches.
func TestFSEditToleratesLineNumbers(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	ctx := context.Background()
	tr := NewReadTracker()
	read := NewFSReadFileTool(sb, tr)
	edit := NewFSEditFileTool(sb, tr)

	_ = os.WriteFile(filepath.Join(sb.Root, "f.go"), []byte("func A() {}\nfunc B() {}\n"), 0o644)
	if _, err := read.Call(ctx, mustJSON(t, map[string]any{"path": "f.go"})); err != nil {
		t.Fatalf("read: %v", err)
	}
	// old_string carries the "     2\t" prefix a model may copy from Read output;
	// new_string is numbered too. The edit must strip both and apply cleanly.
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path":       "f.go",
		"old_string": "     2\tfunc B() {}",
		"new_string": "     2\tfunc B() int { return 0 }",
	})); err != nil {
		t.Fatalf("numbered edit should succeed: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(sb.Root, "f.go"))
	if string(got) != "func A() {}\nfunc B() int { return 0 }\n" {
		t.Fatalf("stripped edit wrong result:\n%q", string(got))
	}
}

// TestFSFreshnessGuard covers the Claude-parity read-before-write / modified-since
// checks the ReadTracker enforces on Edit and Write.
func TestFSFreshnessGuard(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	ctx := context.Background()
	tr := NewReadTracker()
	read := NewFSReadFileTool(sb, tr)
	write := NewFSWriteFileTool(sb, tr)
	edit := NewFSEditFileTool(sb, tr)

	path := filepath.Join(sb.Root, "guarded.txt")
	_ = os.WriteFile(path, []byte("one two three"), 0o644)

	// Edit before any read → "not read yet".
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path": "guarded.txt", "old_string": "two", "new_string": "TWO",
	})); err == nil || !strings.Contains(err.Error(), "not been read") {
		t.Fatalf("edit-before-read: want not-read error, got %v", err)
	}

	// Overwrite of an existing file before any read → "not read yet".
	if _, err := write.Call(ctx, mustJSON(t, map[string]any{
		"path": "guarded.txt", "content": "blind overwrite",
	})); err == nil || !strings.Contains(err.Error(), "not been read") {
		t.Fatalf("write-before-read: want not-read error, got %v", err)
	}

	// After a read, an edit succeeds.
	if _, err := read.Call(ctx, mustJSON(t, map[string]any{"path": "guarded.txt"})); err != nil {
		t.Fatalf("read: %v", err)
	}
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path": "guarded.txt", "old_string": "two", "new_string": "TWO",
	})); err != nil {
		t.Fatalf("edit-after-read: %v", err)
	}

	// Out-of-band change → next edit trips the modified-since-read check.
	_ = os.WriteFile(path, []byte("changed by someone else"), 0o644)
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path": "guarded.txt", "old_string": "changed", "new_string": "CHANGED",
	})); err == nil || !strings.Contains(err.Error(), "modified since") {
		t.Fatalf("edit-after-external-change: want stale error, got %v", err)
	}

	// Writing a BRAND-NEW file needs no prior read.
	if _, err := write.Call(ctx, mustJSON(t, map[string]any{
		"path": "fresh.txt", "content": "new content",
	})); err != nil {
		t.Fatalf("write-new-file: %v", err)
	}

	// A nil tracker disables the guard entirely (historical behaviour).
	edOff := NewFSEditFileTool(sb, nil)
	_ = os.WriteFile(filepath.Join(sb.Root, "off.txt"), []byte("a b c"), 0o644)
	if _, err := edOff.Call(ctx, mustJSON(t, map[string]any{
		"path": "off.txt", "old_string": "b", "new_string": "B",
	})); err != nil {
		t.Fatalf("guard-off edit without read should succeed: %v", err)
	}
}

func TestFSListGlobGrep(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	ctx := context.Background()
	_ = os.MkdirAll(filepath.Join(sb.Root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(sb.Root, "src", "main.go"), []byte("package main\nfunc Hello() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "readme.md"), []byte("# title\nHello world\n"), 0o644)

	list := NewFSListDirTool(sb)
	out, err := list.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "src/") || !strings.Contains(out, "readme.md") {
		t.Fatalf("list got %q", out)
	}

	glob := NewFSGlobTool(sb)
	out, err = glob.Call(ctx, mustJSON(t, map[string]any{"pattern": "**/*.go"}))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "src/main.go") {
		t.Fatalf("glob got %q", out)
	}

	grep := NewFSGrepTool(sb)
	out, err = grep.Call(ctx, mustJSON(t, map[string]any{"pattern": "Hello"}))
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(out, "src/main.go:") || !strings.Contains(out, "readme.md:") {
		t.Fatalf("grep got %q", out)
	}

	// glob-restricted grep only matches the .go file.
	out, _ = grep.Call(ctx, mustJSON(t, map[string]any{"pattern": "Hello", "glob": "**/*.go"}))
	if strings.Contains(out, "readme.md") {
		t.Fatalf("glob-restricted grep should not match readme: %q", out)
	}
}

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		match   bool
	}{
		{"**/*.go", "src/main.go", true},
		{"**/*.go", "main.go", true},
		{"*.go", "main.go", true},
		{"*.go", "src/main.go", false},
		{"src/*.ts", "src/app.ts", true},
		{"src/*.ts", "src/sub/app.ts", false},
		// "**/" spans WHOLE segments only: a name that merely ends with the
		// pattern must not match (regression — "**/log_*" used to hit
		// "d81c8e90-log_....txt", so an agent globbing for a file by name got a
		// false positive on a differently-named one).
		{"**/x.go", "a/b/x.go", true},
		{"**/x.go", "x.go", true},
		{"**/x.go", "barx.go", false},
		{"**/x.go", "a/b/barx.go", false},
		{"**/log_2026*", "art/S1/log_20260729.txt", true},
		{"**/log_2026*", "art/S1/d81c8e90-log_20260729.txt", false},
		// A bare ** (no trailing slash) still spans separators.
		{"artifacts/**", "artifacts/S1/a.txt", true},
		{"a/**/b.go", "a/x/y/b.go", true},
		{"a/**/b.go", "a/b.go", true},
		{"a/**/b.go", "a/x/zzb.go", false},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.pattern)
		if err != nil {
			t.Fatalf("compile %q: %v", c.pattern, err)
		}
		if got := re.MatchString(c.path); got != c.match {
			t.Fatalf("glob %q vs %q: got %v want %v", c.pattern, c.path, got, c.match)
		}
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
