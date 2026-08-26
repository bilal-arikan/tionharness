package tools

import (
	"context"
	"fmt"
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
	t.Setenv("TIONHARNESS_GREP_NO_RG", "1") // pin the deterministic Go engine
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

	// no_ignore surfaces ordinary gitignored hits, but default heavy dirs stay out.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "no_ignore": true}))
	if !strings.Contains(out, "debug.log") || strings.Contains(out, "node_modules") {
		t.Fatalf("no_ignore should include debug.log but retain heavy-dir exclusion:\n%s", out)
	}
}

// TestGrepMultiPath: a comma-joined path list scans every listed file instead of
// failing as one nonexistent path (the reported bug). Files outside the list stay out.
func TestGrepMultiPath(t *testing.T) {
	t.Setenv("TIONHARNESS_GREP_NO_RG", "1") // pin the deterministic Go engine
	sb := setupTree(t)
	g := NewFSGrepTool(sb)

	out, err := g.Call(context.Background(), mustJSON(t, map[string]any{
		"pattern": "todo", "-i": true, "path": "src/main.go,src/util.go",
	}))
	if err != nil {
		t.Fatalf("multi-path grep: %v", err)
	}
	if !strings.Contains(out, "src/main.go:2:") || !strings.Contains(out, "src/util.go:2:") {
		t.Fatalf("multi-path should hit both listed files:\n%s", out)
	}
	if strings.Contains(out, "readme.md") {
		t.Fatalf("multi-path leaked a file not in the list:\n%s", out)
	}
}

// TestGrepMultiPathMissing: an invalid entry names exactly the bad path and offers a
// way out — it must not echo the whole joined blob or read as "no matches".
func TestGrepMultiPathMissing(t *testing.T) {
	t.Setenv("TIONHARNESS_GREP_NO_RG", "1")
	sb := setupTree(t)
	g := NewFSGrepTool(sb)

	_, err := g.Call(context.Background(), mustJSON(t, map[string]any{
		"pattern": "todo", "path": "src/main.go,src/nope.go",
	}))
	if err == nil {
		t.Fatal("a missing path in the list must error, not silently drop it")
	}
	if !strings.Contains(err.Error(), "nope.go") {
		t.Fatalf("error must name the missing path: %v", err)
	}
	if strings.Contains(err.Error(), "src/main.go,src/nope.go") {
		t.Fatalf("error must not echo the whole joined string: %v", err)
	}
}

// TestGrepMultiPathRGParity locks the ripgrep fast path to the Go engine for a
// comma-joined path list. Skipped when rg is absent.
func TestGrepMultiPathRGParity(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := setupTree(t)
	arg := map[string]any{"pattern": "todo", "-i": true, "path": "src/main.go,src/util.go"}

	rgOut, _ := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, arg))
	t.Setenv("TIONHARNESS_GREP_NO_RG", "1")
	goOut, _ := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, arg))
	if rgOut != goOut {
		t.Fatalf("multi-path parity mismatch\nrg:\n%q\ngo:\n%q", rgOut, goOut)
	}
	if !strings.Contains(rgOut, "src/main.go:2:") || !strings.Contains(rgOut, "src/util.go:2:") {
		t.Fatalf("multi-path rg missing hits:\n%s", rgOut)
	}
}

func TestGrepContext(t *testing.T) {
	t.Setenv("TIONHARNESS_GREP_NO_RG", "1")
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
	t.Setenv("TIONHARNESS_GREP_NO_RG", "1")
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

	// no_ignore surfaces ordinary ignored hits, but default heavy dirs stay out.
	out, _ = g.Call(ctx, mustJSON(t, map[string]any{"pattern": "TODO", "no_ignore": true}))
	if !strings.Contains(out, "debug.log") || strings.Contains(out, "node_modules") {
		t.Fatalf("rg no_ignore should include debug.log but retain heavy-dir exclusion:\n%s", out)
	}
}

func TestGrepGlobExcludeHeavyDirsByDefault(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := NewSandbox(t.TempDir())
	for _, dir := range heavySearchDirs {
		if err := os.MkdirAll(filepath.Join(sb.Root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(sb.Root, dir, "heavy.txt"), []byte("heavy needle\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(sb.Root, "visible.txt"), []byte("visible needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	grepOut, err := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "needle"}))
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !strings.Contains(grepOut, "visible.txt") {
		t.Fatalf("grep missed visible file:\n%s", grepOut)
	}
	globOut, err := NewFSGlobTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "**/*.txt"}))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, dir := range heavySearchDirs {
		if strings.Contains(grepOut, dir+"/") || strings.Contains(globOut, dir+"/") {
			t.Fatalf("default search included heavy directory %q\ngrep:\n%s\nglob:\n%s", dir, grepOut, globOut)
		}
	}
}

func TestGrepGlobKeepsGenericSourceDirsByDefault(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := NewSandbox(t.TempDir())
	for _, dir := range []string{"Library", "Temp", "obj", "bin", "dist", "build", "target"} {
		if err := os.MkdirAll(filepath.Join(sb.Root, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(sb.Root, dir, "source.txt"), []byte("source needle\n"), 0o644); err != nil {
			t.Fatalf("seed %s: %v", dir, err)
		}
	}

	grepOut, err := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "needle"}))
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	globOut, err := NewFSGlobTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "**/*.txt"}))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	for _, dir := range []string{"Library", "Temp", "obj", "bin", "dist", "build", "target"} {
		if !strings.Contains(grepOut, dir+"/") || !strings.Contains(globOut, dir+"/") {
			t.Fatalf("default search excluded generic source directory %q\ngrep:\n%s\nglob:\n%s", dir, grepOut, globOut)
		}
	}
}

func TestGrepGlobExplicitHeavyDirStillSearches(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := NewSandbox(t.TempDir())
	if err := os.MkdirAll(filepath.Join(sb.Root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sb.Root, "node_modules", "asset.txt"), []byte("explicit needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	grepOut, err := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, map[string]any{
		"pattern": "needle", "path": "node_modules",
	}))
	if err != nil || !strings.Contains(grepOut, "asset.txt") {
		t.Fatalf("explicit grep must search node_modules: out=%q err=%v", grepOut, err)
	}
	globOut, err := NewFSGlobTool(sb).Call(context.Background(), mustJSON(t, map[string]any{
		"pattern": "node_modules/**",
	}))
	if err != nil || !strings.Contains(globOut, "node_modules/asset.txt") {
		t.Fatalf("explicit glob must search node_modules: out=%q err=%v", globOut, err)
	}
}

func TestGrepTimedOutSearchTTL(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := setupTree(t)
	args := grepArgs{Pattern: "TTL needle"}
	key := normalizedSearchKey("Grep", args)
	currentTime := time.Date(2026, time.August, 27, 0, 0, 0, 0, time.UTC)
	originalNow := nowFn
	nowFn = func() time.Time { return currentTime }
	recordTimedOutSearch(key)

	originalRun := runRGCommand
	spawned := 0
	runRGCommand = func(context.Context, string, []string, string) (string, error) {
		spawned++
		return "", nil
	}
	t.Cleanup(func() {
		nowFn = originalNow
		runRGCommand = originalRun
		timedOutSearchesMu.Lock()
		delete(timedOutSearches, key)
		timedOutSearchesMu.Unlock()
	})

	currentTime = currentTime.Add(timedOutSearchTTL - time.Second)
	_, err := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "TTL needle"}))
	if err == nil || !strings.Contains(err.Error(), repeatedSearchTimeoutMessage) {
		t.Fatalf("unexpired timeout must fail fast: %v", err)
	}
	if spawned != 0 {
		t.Fatalf("unexpired timeout spawned ripgrep %d times, want 0", spawned)
	}

	currentTime = currentTime.Add(time.Second)
	_, err = NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "TTL needle"}))
	if err != nil {
		t.Fatalf("expired timeout must retry: %v", err)
	}
	if spawned != 1 {
		t.Fatalf("expired timeout spawned ripgrep %d times, want 1", spawned)
	}
}

func TestGrepIdenticalTimedOutSearchFailsFast(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := setupTree(t)
	args := grepArgs{Pattern: "TODO"}
	key := normalizedSearchKey("Grep", args)
	recordTimedOutSearch(key)
	t.Cleanup(func() {
		timedOutSearchesMu.Lock()
		delete(timedOutSearches, key)
		timedOutSearchesMu.Unlock()
	})

	originalRun := runRGCommand
	spawned := 0
	runRGCommand = func(context.Context, string, []string, string) (string, error) {
		spawned++
		return "", nil
	}
	t.Cleanup(func() { runRGCommand = originalRun })

	_, err := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, map[string]any{"pattern": "TODO"}))
	if err == nil || !strings.Contains(err.Error(), repeatedSearchTimeoutMessage) {
		t.Fatalf("repeat must fail with directive message: %v", err)
	}
	if spawned != 0 {
		t.Fatalf("repeat spawned ripgrep %d times, want 0", spawned)
	}
}

// TestGrepRGGoParity locks byte-for-byte parity between the ripgrep fast path and
// the Go engine across the main modes (incl. a CRLF file and context), so the
// --sort/CRLF normalisation never regresses. Skipped when rg is absent.
func TestGrepRGGoParity(t *testing.T) {
	if rgExe() == "" {
		t.Skip("ripgrep (rg) not on PATH")
	}
	sb := NewSandbox(t.TempDir())
	_ = os.MkdirAll(filepath.Join(sb.Root, "a"), 0o755)
	// Mixed line endings: bcrlf.go uses CRLF, others LF.
	_ = os.WriteFile(filepath.Join(sb.Root, "a", "lf.go"), []byte("package a\nfunc One() {}\nfunc Two() {}\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "bcrlf.go"), []byte("package main\r\nfunc One() {}\r\nfunc Three() {}\r\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sb.Root, "c.md"), []byte("One two\nfunc-ish One\n"), 0o644)

	cases := []map[string]any{
		{"pattern": "func One"},
		{"pattern": "func", "output_mode": "count"},
		{"pattern": "func", "output_mode": "files_with_matches"},
		{"pattern": "One", "-B": 1, "-A": 1},
		{"pattern": "func", "type": "go"},
		{"pattern": "one", "-i": true, "-n": false},
	}
	for i, c := range cases {
		rgOut, _ := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, c))
		t.Setenv("TIONHARNESS_GREP_NO_RG", "1")
		goOut, _ := NewFSGrepTool(sb).Call(context.Background(), mustJSON(t, c))
		os.Unsetenv("TIONHARNESS_GREP_NO_RG")
		if rgOut != goOut {
			t.Errorf("case %d (%v) parity mismatch\nrg:\n%q\ngo:\n%q", i, c, rgOut, goOut)
		}
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

// TestWalkGrepFilesCapsHugeTree pins the Go-fallback guard: this path collects
// every candidate before reading any of them, so an unbounded tree would be
// walked and slurped into memory inside a single tool call. Past the cap it must
// refuse LOUDLY (naming the way out), not truncate silently.
func TestWalkGrepFilesCapsHugeTree(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= maxWalkFiles+1; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	files, _, err := walkGrepFiles(root, grepArgs{}, func(string) bool { return true })
	if err == nil {
		t.Fatal("walk past the cap must fail, not return a truncated list")
	}
	if files != nil {
		t.Fatalf("no partial result may leak out, got %d files", len(files))
	}
	for _, want := range []string{"too large", "ripgrep"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must mention %q so the caller knows the way out: %v", want, err)
		}
	}
}

// TestWalkGrepFilesUnderCapIsUntouched: the guard must not change the ordinary
// case — a normal tree still returns every match.
func TestWalkGrepFilesUnderCapIsUntouched(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	files, _, err := walkGrepFiles(root, grepArgs{}, func(string) bool { return true })
	if err != nil {
		t.Fatalf("normal tree must walk cleanly: %v", err)
	}
	if len(files) != 20 {
		t.Fatalf("got %d files, want 20", len(files))
	}
}
