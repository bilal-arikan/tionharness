package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// newZvecIndexRuntime returns a runtime with an enabled zvec-grep server whose
// command is the shim at an absolute path, and the index runner stubbed to report
// each run's root on the returned channel (or a description of an unexpected
// invocation).
func newZvecIndexRuntime(t *testing.T) (*Runtime, <-chan string) {
	t.Helper()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	shim := filepath.Join(t.TempDir(), "bin", "zg")
	row := zvecGrepRow()
	row.Command = shim
	if _, err := rt.db.CreateMCPServer(context.Background(), row); err != nil {
		t.Fatalf("create zvec-grep server: %v", err)
	}
	roots := make(chan string, 4)
	original := runZvecGrepIndex
	t.Cleanup(func() { runZvecGrepIndex = original })
	runZvecGrepIndex = func(_ context.Context, command, root, embedding string) ([]byte, error) {
		if command != shim || !strings.HasPrefix(embedding, "local/") {
			roots <- "unexpected invocation: " + command + " --embedding " + embedding
			return nil, nil
		}
		roots <- root
		return nil, nil
	}
	return rt, roots
}

func writeZvecManifest(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, zvecGrepIndexDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, zvecGrepManifest), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureZvecGrepIndexedBuildsTheRepositoryIndexOnce(t *testing.T) {
	// A remote model in the environment must not leak into a background run.
	t.Setenv("ZVEC_GREP_EMBEDDING", "qwen/text-embedding-v4")
	rt, roots := newZvecIndexRuntime(t)
	repo := t.TempDir()
	sub := filepath.Join(repo, "internal", "agent")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if !rt.EnsureZvecGrepIndexed(context.Background(), sub) {
		t.Fatal("no index scheduled for an unindexed repository")
	}
	select {
	case root := <-roots:
		if root != repo {
			t.Fatalf("indexed %q, want the repository root %q", root, repo)
		}
	case <-time.After(time.Second):
		t.Fatal("index did not start")
	}
	// The exclude entry lands before the index can create a single file.
	data, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(data), ".zvec-grep/") {
		t.Errorf("index not excluded from git: %q (%v)", data, err)
	}

	// Another turn in the same repository reports the index as on its way without
	// starting a second run.
	if !rt.EnsureZvecGrepIndexed(context.Background(), repo) {
		t.Error("an index in flight must still report true")
	}
	select {
	case root := <-roots:
		t.Errorf("second index run for %q", root)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestEnsureZvecGrepIndexedSkips(t *testing.T) {
	rt, roots := newZvecIndexRuntime(t)
	ctx := context.Background()

	// An index at or above the cwd is the daemon's to keep fresh.
	indexed := t.TempDir()
	writeZvecManifest(t, indexed)
	if !rt.EnsureZvecGrepIndexed(ctx, filepath.Join(indexed, "docs")) {
		t.Error("an existing ancestor index must report true")
	}
	worktree := filepath.Join(t.TempDir(), ".tionharness-worktrees", "WS1", "tsk1")
	if rt.EnsureZvecGrepIndexed(ctx, worktree) {
		t.Error("a throwaway worktree was indexed")
	}
	if rt.EnsureZvecGrepIndexed(ctx, filepath.Join("relative", "dir")) {
		t.Error("a relative cwd was indexed")
	}
	rt.SetZvecGrep(false)
	if rt.EnsureZvecGrepIndexed(ctx, t.TempDir()) {
		t.Error("indexed with the workspace switch off")
	}
	select {
	case root := <-roots:
		t.Errorf("unexpected index run for %q", root)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestZvecGrepHomeIsNeitherAnIndexNorATarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// zvec-grep keeps its daemon state and model cache in ~/.zvec-grep. Even with a
	// manifest-shaped file there it must not make every folder under home look
	// indexed.
	writeZvecManifest(t, home)
	project := filepath.Join(home, "projects", "notes")
	if got := zvecGrepIndexedRoot(project); got != "" {
		t.Errorf("zvecGrepIndexedRoot(%q) = %q, want none", project, got)
	}

	// A dotfiles repository in home must not turn an unversioned folder into an
	// index of the whole home directory.
	if err := os.MkdirAll(filepath.Join(home, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := zvecGrepIndexTarget(project); got != project {
		t.Errorf("zvecGrepIndexTarget(%q) = %q, want the folder itself", project, got)
	}
	if !zvecGrepBroadDir(home) {
		t.Error("the home directory itself must be refused as a working directory")
	}
}

func TestEnsureZvecGrepGitExclude(t *testing.T) {
	repo := t.TempDir()
	info := filepath.Join(repo, ".git", "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		t.Fatal(err)
	}
	exclude := filepath.Join(info, "exclude")
	// An existing file without a trailing newline: the entry must not be glued
	// onto its last pattern.
	if err := os.WriteFile(exclude, []byte("# local\n*.log"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := ensureZvecGrepGitExclude(repo); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
	data, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), ".zvec-grep/"); got != 1 {
		t.Errorf("exclude holds %d entries, want exactly one:\n%s", got, data)
	}
	if !strings.HasPrefix(string(data), "# local\n*.log\n") {
		t.Errorf("existing patterns not preserved:\n%s", data)
	}

	// A worktree or submodule has a .git FILE; its exclude lives elsewhere, and this
	// helper must not try to create a directory under that file.
	wt := t.TempDir()
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: ../elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureZvecGrepGitExclude(wt); err != nil {
		t.Errorf(".git file: %v", err)
	}
}

func TestZvecGrepCLIPrefersTheRowsShimThenTheResolver(t *testing.T) {
	original := zvecGrepDetect
	t.Cleanup(func() { zvecGrepDetect = original })
	resolved := filepath.Join(t.TempDir(), "npm", "zg.cmd")
	zvecGrepDetect = func() (bool, string) { return true, resolved }

	abs := filepath.Join(t.TempDir(), "bin", "zg")
	if got := zvecGrepCLI(db.MCPServer{Command: abs}); got != abs {
		t.Errorf("absolute shim row: got %q, want the row's own command", got)
	}
	// A bare shim name may not be on this process's PATH; the resolver knows better.
	if got := zvecGrepCLI(db.MCPServer{Command: "zg"}); got != resolved {
		t.Errorf("bare shim row: got %q, want the resolved %q", got, resolved)
	}
	// A node/npx launcher carries no CLI path of its own.
	if got := zvecGrepCLI(db.MCPServer{Command: "npx", Args: `["-y","@zvec/zvec-grep","server","--stdio"]`}); got != resolved {
		t.Errorf("npx row: got %q, want the resolved %q", got, resolved)
	}

	zvecGrepDetect = func() (bool, string) { return false, "" }
	if got := zvecGrepCLI(db.MCPServer{Command: "zg"}); got != "zg" {
		t.Errorf("unresolved bare shim: got %q, want exec to try \"zg\"", got)
	}
	if got := zvecGrepCLI(db.MCPServer{Command: "npx"}); got != "" {
		t.Errorf("unresolved npx row: got %q, want none", got)
	}
}
