package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	if got, err := sb.Resolve(filepath.Join(string(filepath.Separator), "tmp", "x")); err != nil || !filepath.IsAbs(got) {
		t.Fatalf("absolute resolve without base: got %q err %v", got, err)
	}
}

func TestFSWriteReadEdit(t *testing.T) {
	sb := NewSandbox(t.TempDir())
	ctx := context.Background()

	write := NewFSWriteFileTool(sb)
	if _, err := write.Call(ctx, mustJSON(t, map[string]any{
		"path":    "notes/hello.txt",
		"content": "alpha beta gamma",
	})); err != nil {
		t.Fatalf("write: %v", err)
	}

	read := NewFSReadFileTool(sb)
	out, err := read.Call(ctx, mustJSON(t, map[string]any{"path": "notes/hello.txt"}))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out != "alpha beta gamma" {
		t.Fatalf("read got %q", out)
	}

	edit := NewFSEditFileTool(sb)
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path":       "notes/hello.txt",
		"old_string": "beta",
		"new_string": "BETA",
	})); err != nil {
		t.Fatalf("edit: %v", err)
	}
	out, _ = read.Call(ctx, mustJSON(t, map[string]any{"path": "notes/hello.txt"}))
	if out != "alpha BETA gamma" {
		t.Fatalf("after edit got %q", out)
	}

	// Editing a non-unique string without replace_all must fail.
	_ = os.WriteFile(filepath.Join(sb.Root, "dup.txt"), []byte("x x x"), 0o644)
	if _, err := edit.Call(ctx, mustJSON(t, map[string]any{
		"path": "dup.txt", "old_string": "x", "new_string": "y",
	})); err == nil {
		t.Fatal("expected non-unique edit to fail without replace_all")
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
