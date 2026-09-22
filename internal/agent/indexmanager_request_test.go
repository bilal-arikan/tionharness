package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// TestRequestRebuildRunsEvenWhenTheIndexIsReady is the whole reason an explicit
// request exists: the automatic path asks Decide, which does nothing for a
// healthy index, so an agent that has seen the index misbehave needs a way to
// force the run anyway.
func TestRequestRebuildRunsEvenWhenTheIndexIsReady(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now())
	indexLedger.Observe(exttoolsZvecGrepName, repo, indexstate.PhaseReady, zvecGrepDefaultEmbedding, "1.0.0")

	// The automatic path agrees there is nothing to do.
	if !rt.EnsureZvecGrepIndex(context.Background(), repo) {
		t.Fatal("a ready index should report usable")
	}
	select {
	case run := <-runs:
		t.Fatalf("the automatic path started a run for a ready index: %+v", run)
	case <-time.After(100 * time.Millisecond):
	}

	entry, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: exttoolsZvecGrepName, Root: repo, Action: indexstate.ActionRebuild,
	})
	if err != nil {
		t.Fatalf("explicit rebuild refused: %v", err)
	}
	if entry.Phase != indexstate.PhaseIndexing {
		t.Errorf("claimed entry phase = %q, want indexing", entry.Phase)
	}
	if got := awaitRun(t, runs); got.root != repo {
		t.Errorf("rebuild ran for %q, want %q", got.root, repo)
	}
	awaitPhase(t, repo, indexstate.PhaseReady)
}

// TestRequestRebuildDiscardsTheOldStore pins the difference between the two
// actions: a rebuild must remove the store first, because `zg index` over a
// store built with another embedding model adds incomparable vectors instead of
// replacing them.
func TestRequestRebuildDiscardsTheOldStore(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, "local/other-model", time.Now())

	stray := filepath.Join(repo, zvecGrepIndexDir, "stray.bin")
	if err := os.WriteFile(stray, []byte("old vectors"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: exttoolsZvecGrepName, Root: repo, Action: indexstate.ActionRebuild,
	}); err != nil {
		t.Fatalf("rebuild refused: %v", err)
	}
	awaitRun(t, runs)
	awaitPhase(t, repo, indexstate.PhaseReady)

	if _, err := os.Stat(stray); !os.IsNotExist(err) {
		t.Error("rebuild kept a file from the discarded store")
	}
}

// TestRequestRefreshKeepsTheStore is the complement: a refresh re-embeds in
// place, so an unrelated file in the store directory survives.
func TestRequestRefreshKeepsTheStore(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now())

	keep := filepath.Join(repo, zvecGrepIndexDir, "keep.bin")
	if err := os.WriteFile(keep, []byte("existing vectors"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: exttoolsZvecGrepName, Root: repo, Action: indexstate.ActionRefresh,
	})
	if err != nil {
		t.Fatalf("refresh refused: %v", err)
	}
	if entry.Action != indexstate.ActionRefresh {
		t.Errorf("entry action = %q, want refresh", entry.Action)
	}
	awaitRun(t, runs)
	awaitPhase(t, repo, indexstate.PhaseReady)

	if _, err := os.Stat(keep); err != nil {
		t.Errorf("refresh discarded the existing store: %v", err)
	}
}

// TestRequestRefreshOfAMissingIndexBecomesACreate keeps the ledger's Action
// honest: there is nothing to re-embed into a store that does not exist.
func TestRequestRefreshOfAMissingIndexBecomesACreate(t *testing.T) {
	rt, runs := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)

	entry, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: exttoolsZvecGrepName, Root: repo, Action: indexstate.ActionRefresh,
	})
	if err != nil {
		t.Fatalf("refresh refused: %v", err)
	}
	if entry.Action != indexstate.ActionCreate {
		t.Errorf("entry action = %q, want create for a missing index", entry.Action)
	}
	awaitRun(t, runs)
	awaitPhase(t, repo, indexstate.PhaseReady)
}

// TestRequestRefusesBroadAndEphemeralRoots proves the explicit path did not
// become a way around the guards the automatic path applies. Each of these
// would embed something the user never asked to index.
func TestRequestRefusesBroadAndEphemeralRoots(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory on this host")
	}
	worktree := filepath.Join(t.TempDir(), ".tionharness-worktrees", "WS1", "tsk1")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct{ name, root string }{
		{"home directory", home},
		{"volume root", filepath.VolumeName(home) + string(filepath.Separator)},
		{"ephemeral worktree", worktree},
		{"relative path", "relative/repo"},
		{"empty", ""},
	}
	for _, c := range cases {
		_, err := rt.RequestIndexRun(context.Background(), IndexRequest{
			Tool: exttoolsZvecGrepName, Root: c.root, Action: indexstate.ActionRefresh,
		})
		if !errors.Is(err, ErrIndexRootNotAllowed) {
			t.Errorf("%s: err = %v, want ErrIndexRootNotAllowed", c.name, err)
		}
	}
}

// TestRequestRefusesAnUnknownToolAndAction guards the dispatch: an unknown tool
// has no known index layout, and an unknown action must not silently become a
// rebuild.
func TestRequestRefusesAnUnknownToolAndAction(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)

	if _, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: "mystery-tool", Root: repo, Action: indexstate.ActionRefresh,
	}); !errors.Is(err, ErrUnknownIndexTool) {
		t.Errorf("unknown tool err = %v, want ErrUnknownIndexTool", err)
	}

	if _, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: exttoolsZvecGrepName, Root: repo, Action: indexstate.ActionDrop,
	}); err == nil {
		t.Error("drop was accepted as a request action")
	}
}

// TestRequestReportsARunAlreadyInFlight: the single-run claim still holds, and
// the caller is told the work is happening rather than that it failed.
func TestRequestReportsARunAlreadyInFlight(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	indexLedger.Begin(exttoolsZvecGrepName, repo, indexstate.ActionRefresh)
	t.Cleanup(func() { indexLedger.Forget(exttoolsZvecGrepName, repo) })

	_, err := rt.RequestIndexRun(context.Background(), IndexRequest{
		Tool: exttoolsZvecGrepName, Root: repo, Action: indexstate.ActionRefresh,
	})
	if !errors.Is(err, ErrIndexRunInFlight) {
		t.Fatalf("err = %v, want ErrIndexRunInFlight", err)
	}
}

// --- status ---------------------------------------------------------------

// TestSearchIndexStatusReportsTheLedgerEntry covers the read path an agent uses
// after a search came back [INDEX_MISSING].
func TestSearchIndexStatusReportsTheLedgerEntry(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	writeManifest(t, repo, zvecGrepDefaultEmbedding, time.Now())
	indexLedger.Observe(exttoolsZvecGrepName, repo, indexstate.PhaseReady, zvecGrepDefaultEmbedding, "1.0.0")
	t.Cleanup(func() { indexLedger.Forget(exttoolsZvecGrepName, repo) })

	got := rt.SearchIndexStatus(context.Background(), repo)
	var zg *IndexStatus
	for i := range got {
		if got[i].Tool == exttoolsZvecGrepName {
			zg = &got[i]
		}
	}
	if zg == nil {
		t.Fatalf("no zvec-grep row in %+v", got)
	}
	if zg.Phase != string(indexstate.PhaseReady) || !zg.Usable || !zg.Managed {
		t.Errorf("row = %+v, want a ready/usable/managed entry", *zg)
	}
	if zg.Embedding != zvecGrepDefaultEmbedding {
		t.Errorf("embedding = %q, want %q", zg.Embedding, zvecGrepDefaultEmbedding)
	}
}

// TestSearchIndexStatusReadsDiskWithoutALedgerEntry: a root this process has
// never touched must report what is actually there, and must NOT record it — an
// Observe from a status read could overwrite a phase a concurrent run is setting.
func TestSearchIndexStatusReadsDiskWithoutALedgerEntry(t *testing.T) {
	rt, _ := newManagedIndexRuntime(t, "1.0.0")
	repo := newRepo(t)
	indexLedger.Forget(exttoolsZvecGrepName, repo)

	got := rt.SearchIndexStatus(context.Background(), repo)
	if len(got) == 0 {
		t.Fatal("no status rows for an enabled tool")
	}
	if got[0].Phase != string(indexstate.PhaseMissing) {
		t.Errorf("phase = %q, want missing", got[0].Phase)
	}
	if !strings.Contains(got[0].Note, "search_index") {
		t.Errorf("a missing index should point at the fix, got note %q", got[0].Note)
	}
	if _, seen := indexLedger.Get(exttoolsZvecGrepName, repo); seen {
		t.Error("a status read wrote to the ledger")
	}
}
