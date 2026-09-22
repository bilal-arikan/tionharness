package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// indexRun is one observed invocation of the stubbed indexer.
type indexRun struct {
	root      string
	embedding string
}

// newManagedIndexRuntime returns a runtime with an enabled zvec-grep server, the
// indexer stubbed, the version probe pinned, and the process-wide ledger reset —
// the ledger outlives a single test, so leaving entries behind would make the
// next test's first Observe see a stale Ready.
func newManagedIndexRuntime(t *testing.T, version string) (*Runtime, chan indexRun) {
	t.Helper()
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	shim := filepath.Join(t.TempDir(), "bin", "zg")
	row := zvecGrepRow()
	row.Command = shim
	if _, err := rt.db.CreateMCPServer(context.Background(), row); err != nil {
		t.Fatalf("create zvec-grep server: %v", err)
	}

	prevLedger := indexLedger
	indexLedger = indexstate.New()
	t.Cleanup(func() { indexLedger = prevLedger })

	prevVer := zvecGrepToolVersion
	zvecGrepToolVersion = func(context.Context, string) string { return version }
	t.Cleanup(func() { zvecGrepToolVersion = prevVer })

	runs := make(chan indexRun, 4)
	prevRun := runZvecGrepIndex
	t.Cleanup(func() { runZvecGrepIndex = prevRun })
	runZvecGrepIndex = func(_ context.Context, _, root, embedding string) ([]byte, error) {
		runs <- indexRun{root: root, embedding: embedding}
		// A real run leaves a manifest behind; the stub does the same so the next
		// observation sees what the run produced.
		writeManifest(t, root, embedding, time.Now())
		return nil, nil
	}
	return rt, runs
}

// writeManifest writes a zvec-grep manifest naming an embedding and update time.
func writeManifest(t *testing.T, root, embedding string, updated time.Time) {
	t.Helper()
	provider, model, _ := strings.Cut(embedding, "/")
	doc := map[string]any{
		"embedding":   map[string]any{"provider": provider, "model": model},
		"updatedTime": updated.UnixMilli(),
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, zvecGrepIndexDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, zvecGrepManifest), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a directory that looks like a git repository root.
func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return repo
}

// awaitRun waits for the stubbed indexer to be called.
func awaitRun(t *testing.T, runs chan indexRun) indexRun {
	t.Helper()
	select {
	case r := <-runs:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("no index run started")
		return indexRun{}
	}
}

// awaitPhase waits for a ledger entry to settle on a phase, since runs are
// asynchronous.
func awaitPhase(t *testing.T, root string, want indexstate.Phase) indexstate.Entry {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var last indexstate.Entry
	for time.Now().Before(deadline) {
		e, seen := indexLedger.Get(exttoolsZvecGrepName, root)
		if seen {
			last = e
			if e.Phase == want {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("phase=%q (%s), want %q", last.Phase, last.Error, want)
	return last
}

func TestManagedIndexCreateMovesMissingToIndexingToReady(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)

	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Fatal("no index scheduled for an unindexed repository")
	}
	run := awaitRun(t, runs)
	if run.root != repo {
		t.Fatalf("indexed %q, want %q", run.root, repo)
	}

	e := awaitPhase(t, repo, indexstate.PhaseReady)
	if e.Action != indexstate.ActionCreate {
		t.Errorf("action=%q, want create", e.Action)
	}
	if !e.Usable() {
		t.Error("a completed index is not usable")
	}
	if e.Embedding != zvecGrepDefaultEmbedding {
		t.Errorf("embedding=%q, want %q", e.Embedding, zvecGrepDefaultEmbedding)
	}
}

func TestManagedIndexRefreshesAStaleStore(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	// A store the watcher has not touched in well over the staleness window.
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now().Add(-3*zvecGrepStaleAfter))

	rt.EnsureZvecGrepIndex(context.Background(), repo)
	awaitRun(t, runs)

	e := awaitPhase(t, repo, indexstate.PhaseReady)
	if e.Action != indexstate.ActionRefresh {
		t.Fatalf("action=%q, want refresh for a stale store", e.Action)
	}
}

func TestManagedIndexRebuildsWhenTheEmbeddingModelChanged(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	// Built with a different model: its vectors are not comparable with the ones
	// the host would produce today, so the store must be discarded, not topped up.
	writeManifest(t, repo, "local/some-other-model", time.Now())
	marker := filepath.Join(repo, zvecGrepIndexDir, "vectors.bin")
	if err := os.WriteFile(marker, []byte("old vectors"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt.EnsureZvecGrepIndex(context.Background(), repo)
	run := awaitRun(t, runs)
	if run.embedding != zvecGrepDefaultEmbedding {
		t.Errorf("rebuilt with %q, want the host's model %q", run.embedding, zvecGrepDefaultEmbedding)
	}

	e := awaitPhase(t, repo, indexstate.PhaseReady)
	if e.Action != indexstate.ActionRebuild {
		t.Fatalf("action=%q, want rebuild on a model change", e.Action)
	}
	// A rebuild discards the old store rather than writing into it.
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("the rebuild kept the old store's files")
	}
}

func TestManagedIndexRebuildsWhenTheToolVersionChanged(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now())
	// Record a store built by an older zg than the one installed now.
	indexLedger.Observe(exttoolsZvecGrepName, repo, indexstate.PhaseReady, zvecGrepDefaultEmbedding, "0.9.0")

	rt.EnsureZvecGrepIndex(context.Background(), repo)
	awaitRun(t, runs)

	e := awaitPhase(t, repo, indexstate.PhaseReady)
	if e.Action != indexstate.ActionRebuild {
		t.Fatalf("action=%q, want rebuild on a tool version change", e.Action)
	}
	if e.ToolVersion != "1.0.0" {
		t.Errorf("toolVersion=%q, want the installed 1.0.0", e.ToolVersion)
	}
}

func TestManagedIndexDoesNothingForACurrentStore(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now())
	indexLedger.Observe(exttoolsZvecGrepName, repo, indexstate.PhaseReady, zvecGrepDefaultEmbedding, "1.0.0")

	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Fatal("a current index did not report as usable")
	}
	select {
	case run := <-runs:
		t.Fatalf("a current index was re-indexed: %+v", run)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestManagedIndexRecordsAFailureInsteadOfReportingReady(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	runZvecGrepIndex = func(context.Context, string, string, string) ([]byte, error) {
		return []byte("embedding model download failed"), context.DeadlineExceeded
	}

	rt.EnsureZvecGrepIndex(context.Background(), repo)

	e := awaitPhase(t, repo, indexstate.PhaseFailed)
	if e.Usable() {
		t.Error("a failed index reported as usable")
	}
	if e.Error == "" {
		t.Error("a failed index carries no reason")
	}
	// The indexer's own output is the diagnosis; it must survive into the entry.
	if !strings.Contains(e.Error, "embedding model download failed") {
		t.Errorf("reason=%q, want the indexer's output", e.Error)
	}
}

// --- preserved shields -----------------------------------------------------

func TestManagedIndexNeverUsesARemoteEmbeddingModel(t *testing.T) {
	// A remote model uploads file contents to a provider and needs an interactive
	// authorization grant; a background job must never fall through to one.
	t.Setenv("ZVEC_GREP_EMBEDDING", "qwen/text-embedding-v4")
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)

	rt.EnsureZvecGrepIndex(context.Background(), repo)
	run := awaitRun(t, runs)
	if !strings.HasPrefix(run.embedding, "local/") {
		t.Fatalf("background run used the remote model %q", run.embedding)
	}
}

func TestManagedIndexRefusesToIndexWhatItCannotExcludeFromGit(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	// Make .git/info a FILE so the exclude entry cannot be written. Without an
	// exclude entry the store shows up as untracked files, one `git add -A` from
	// a committed binary vector store — so no index may be built.
	if err := os.WriteFile(filepath.Join(repo, ".git", "info"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt.EnsureZvecGrepIndex(context.Background(), repo)

	select {
	case run := <-runs:
		t.Fatalf("indexed a repository the store could not be excluded from: %+v", run)
	case <-time.After(200 * time.Millisecond):
	}
	e := awaitPhase(t, repo, indexstate.PhaseFailed)
	if !strings.Contains(e.Error, "git") {
		t.Errorf("reason=%q, want it to name the exclude failure", e.Error)
	}
}

func TestManagedIndexWritesTheGitExcludeBeforeIndexing(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)

	rt.EnsureZvecGrepIndex(context.Background(), repo)
	awaitRun(t, runs)

	data, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(data), zvecGrepIndexDir+"/") {
		t.Fatalf("index not excluded from git: %q (%v)", data, err)
	}
}

func TestManagedIndexSkipsEphemeralAndBroadRoots(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	ctx := context.Background()

	// A worktree/scratchpad is a throwaway copy the parent index already covers.
	worktree := filepath.Join(t.TempDir(), ".tionharness-worktrees", "WS1", "tsk1")
	if rt.EnsureZvecGrepIndex(ctx, worktree) {
		t.Error("a throwaway worktree was indexed")
	}
	// A relative cwd cannot be resolved to a root.
	if rt.EnsureZvecGrepIndex(ctx, filepath.Join("relative", "dir")) {
		t.Error("a relative cwd was indexed")
	}
	// Home and volume root would embed far more than a project.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rt.EnsureZvecGrepIndex(ctx, home) {
			t.Error("the home directory was indexed")
		}
	}
	if rt.EnsureZvecGrepIndex(ctx, filepath.VolumeName(t.TempDir())+string(filepath.Separator)) {
		t.Error("a volume root was indexed")
	}

	select {
	case run := <-runs:
		t.Fatalf("a skipped root was indexed anyway: %+v", run)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestManagedIndexRunsOnceAcrossRuntimesSharingARepository(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	// Hold the run open so the second caller meets a claim in flight, the way a
	// second workspace runtime on the same repository would.
	release := make(chan struct{})
	runZvecGrepIndex = func(_ context.Context, _, root, embedding string) ([]byte, error) {
		runs <- indexRun{root: root, embedding: embedding}
		<-release
		writeManifest(t, root, embedding, time.Now())
		return nil, nil
	}

	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Fatal("first call did not schedule an index")
	}
	awaitRun(t, runs)
	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Error("a run in flight must still report true")
	}
	select {
	case run := <-runs:
		t.Fatalf("a second concurrent run started: %+v", run)
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	awaitPhase(t, repo, indexstate.PhaseReady)
}

// --- drop gate -------------------------------------------------------------

func TestDropRequiresConfirmationAndKeepsTheStore(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now())
	indexLedger.Observe(exttoolsZvecGrepName, repo, indexstate.PhaseReady, zvecGrepDefaultEmbedding, "1.0.0")

	if err := rt.DropSearchIndex(context.Background(), exttoolsZvecGrepName, repo, ""); err == nil {
		t.Fatal("an unconfirmed drop succeeded")
	}
	if _, err := os.Stat(zvecGrepManifestPath(repo)); err != nil {
		t.Fatal("an unconfirmed drop deleted the index")
	}

	if err := rt.DropSearchIndex(context.Background(), exttoolsZvecGrepName, repo, repo); err != nil {
		t.Fatalf("confirmed drop failed: %v", err)
	}
	if _, err := os.Stat(zvecGrepIndexPath(repo)); !os.IsNotExist(err) {
		t.Fatal("the index survived a confirmed drop")
	}
}

func TestDropRefusesAnUnknownTool(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	// Guessing a directory from an unknown tool's name is exactly what the gate
	// exists to prevent.
	if err := rt.DropSearchIndex(context.Background(), "mystery-tool", repo, repo); err == nil {
		t.Fatal("a drop for an unknown tool was accepted")
	}
}
