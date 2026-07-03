package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTree(t *testing.T) Sandbox {
	t.Helper()
	sb := NewSandbox(t.TempDir())
	_ = os.MkdirAll(filepath.Join(sb.Root, "src"), 0o755)
	_ = os.MkdirAll(filepath.Join(sb.Root, "node_modules", "pkg"), 0o755)
	_ = os.WriteFile(filepath.Join(sb.Root, "src", "main.go"), []byte("package main\n// TODO fix\nfunc Hello() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "src", "util.go"), []byte("package main\nfunc todo() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "readme.md"), []byte("# Title\nTODO write docs\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "node_modules", "pkg", "index.js"), []byte("// TODO ignore me\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, ".gitignore"), []byte("node_modules/\n*.log\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "debug.log"), []byte("TODO in a log\n"), 0o644)
	return sb
}

func TestGrepOutputModes(t *testing.T) {
	t.Setenv("SWARMGO_GREP_NO_RG", "1") // pin the deterministic Go engine
	sb := setupTree(t)
	ctx := context.Background()
	g := NewFSGrepTool(sb)

	// content (default), case-insensitive matches TODO and todo.
	out, err := g.Call(ctx, mustJSON(t, map[string]any{"pattern": "todo", "-i": true}))
	if err != nil {
		t.Fatalf("grep content: %v", err)
	}
	if !strings.Contains(out, "src/main.go:2:") || !strings.Contains(out, "src/util.go:2:") {
		t.Fatalf("content mode missing hits:\n%s", out)
	}
	// .gitignore excludes node_modules and *.log.
	if strings.Contains(out, "node_modules") || strings.Contains(out, "debug.log") {
		t.Fatalf("gitignored files leaked into results:\n%s", out)
	}

	// files_with_matches.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "output_mode": "files_with_matches"}))
	if !strings.Contains(out, "src/main.go") || strings.Contains(out, ":2:") {
		t.Fatalf("files_with_matches wrong:\n%s", out)
	}

	// count.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "output_mode": "count"}))
	if !strings.Contains(out, "src/main.go:1") {
		t.Fatalf("count wrong:\n%s", out)
	}

	// type filter: only .go files (readme.md excluded).
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "-i": true, "type": "go"}))
	if strings.Contains(out, "readme.md") {
		t.Fatalf("type=go should exclude readme:\n%s", out)
	}

	// no_ignore surfaces the gitignored hits.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "no_ignore": true}))
	if !strings.Contains(out, "node_modules") {
		t.Fatalf("no_ignore should include node_modules:\n%s", out)
	}
}

func TestGrepContext(t *testing.T) {
	t.Setenv("SWARMGO_GREP_NO_RG", "1")
	sb := setupTree(t)
	g := NewFSGrepTool(sb)
	// -B 1 includes the line before the match with a '-' separator.
	out, _ := g.Call(context.Background(), mustJSON(t, map[string]any{
		"pattern": "TODO fix", "path": "src/main.go", "-B": 1,
	}))
	if !strings.Contains(out, "main.go-1-package main") || !strings.Contains(out, "main.go:2:// TODO fix") {
		t.Fatalf("context output wrong:\n%s", out)
	}
}

func TestGrepOnlyMatching(t *testing.T) {
	t.Setenv("SWARMGO_GREP_NO_RG", "1")
	sb := setupTree(t)
	g := NewFSGrepTool(sb)
	out, _ := g.Call(context.Background(), mustJSON(t, map[string]any{
		"pattern": "T[O]DO", "path": "readme.md", "-o": true,
	}))
	// -o prints only the matched substring.
	if !strings.Contains(out, "readme.md:2:TODO") || strings.Contains(out, "write docs") {
		t.Fatalf("only-matching wrong:\n%s", out)
	}
}

func TestGlobMtimeSortAndIgnore(t *testing.T) {
	sb := setupTree(t)
	g := NewFSGlobTool(sb)

	// Make util.go the most recently modified.
	newer := time.Now().Add(1 * time.Hour)
	_ = os.Chtimes(filepath.Join(sb.Root, "src", "util.go"), newer, newer)

	out, err := g.Call(context.Background(), mustJSON(t, map[string]any{"pattern": "**/*.go"}))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 || lines[0] != "src/util.go" {
		t.Fatalf("expected most-recent (util.go) first, got:\n%s", out)
	}
	// node_modules is gitignored → not returned even though it has no .go here.
	if strings.Contains(out, "node_modules") {
		t.Fatalf("glob returned ignored path:\n%s", out)
	}
}

// TestGrepRGFastPath exercises the ripgrep delegation when rg is installed. It is
// skipped where rg is absent; the deterministic Go engine is covered separately.
func TestGrepRGFastPath(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := setupTree(t)
	ctx := context.Background()
	g := NewFSGrepTool(sb)

	// content: matches in tracked files, .gitignore honoured (via --no-require-git).
	out, err := g.Call(ctx, mustJSON(t, map[string]any{"pattern": "todo", "-i": true}))
	if err != nil {
		t.Fatalf("rg content: %v", err)
	}
	if !strings.Contains(out, "src/main.go:2:") || !strings.Contains(out, "src/util.go:2:") {
		t.Fatalf("rg content missing hits:\n%s", out)
	}
	if strings.Contains(out, "node_modules") || strings.Contains(out, "debug.log") {
		t.Fatalf("rg leaked gitignored files:\n%s", out)
	}
	// Windows backslashes must be normalised to forward slashes.
	if strings.Contains(out, "\\") {
		t.Fatalf("rg output not path-normalised:\n%s", out)
	}

	// count mode.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "output_mode": "count"}))
	if !strings.Contains(out, "src/main.go:1") {
		t.Fatalf("rg count wrong:\n%s", out)
	}

	// no_ignore surfaces the ignored hits.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "no_ignore": true}))
	if !strings.Contains(out, "node_modules") {
		t.Fatalf("rg no_ignore should include node_modules:\n%s", out)
	}
}

func TestGlobPathArg(t *testing.T) {
	sb := setupTree(t)
	g := NewFSGlobTool(sb)
	out, _ := g.Call(context.Background(), mustJSON(t, map[string]any{"pattern": "*.go", "path": "src"}))
	if !strings.Contains(out, "main.go") || strings.Contains(out, "src/") {
		t.Fatalf("path-scoped glob should be relative to src:\n%s", out)
	}
}
