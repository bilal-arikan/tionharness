package agent

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestBuildSnapshotGenome: user agents enter the genome, locked built-ins do
// not; the hash moves with a config change and stays put otherwise; the
// provider stamps new sessions and the snapshot is persisted.
func TestBuildSnapshotGenome(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()
	if err := database.EnsureSystemAgents(ctx, SystemAgentDefaults()...); err != nil {
		t.Fatalf("system agents: %v", err)
	}
	r := &Runtime{db: database, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	dev, _ := database.CreateAgent(ctx, db.Agent{Name: "Dev", Soul: "be kind", Model: "sonnet", Skills: []string{"x"}})
	auto, _ := database.CreateAutomation(ctx, db.Automation{Name: "nightly", Enabled: true, TriggerKind: "tag", TriggerTag: "t", CooldownSec: 5})

	snap, err := r.BuildSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Agents[dev.ID]; !ok {
		t.Fatal("user agent missing from genome")
	}
	for id, a := range snap.Agents {
		if a.SystemKey != "" {
			t.Fatalf("locked built-in %s (%s) must not be in the genome", id, a.SystemKey)
		}
	}
	if g := snap.Agents[dev.ID]; g.SoulChars != 7 || g.Model != "sonnet" || len(g.Skills) != 1 {
		t.Fatalf("genome = %+v", g)
	}
	if _, ok := snap.Automations[auto.ID]; !ok {
		t.Fatal("enabled automation missing")
	}
	h1 := r.CurrentSnapshotHash()
	if h1 == "" || h1 != snap.Hash {
		t.Fatalf("hash = %q, want %q", h1, snap.Hash)
	}
	if h1 != r.CurrentSnapshotHash() {
		t.Fatal("hash must be stable without a change")
	}
	rows, current, _ := database.ListSnapshots(ctx)
	if current != h1 || len(rows) != 1 {
		t.Fatalf("snapshot not persisted: current=%s rows=%d", current, len(rows))
	}

	// A config change → new hash after the cache expires (force by resetting).
	opus := "opus"
	if _, err := database.UpdateAgent(ctx, dev.ID, db.AgentProfilePatch{Model: &opus}); err != nil {
		t.Fatal(err)
	}
	r.snapshotCache().at = r.snapshotCache().at.Add(-snapshotCacheTTL * 2)
	h2 := r.CurrentSnapshotHash()
	if h2 == h1 {
		t.Fatal("model change must produce a new snapshot hash")
	}
	rows, _, _ = database.ListSnapshots(ctx)
	if len(rows) != 2 || rows[1].Prev != h1 {
		t.Fatalf("history edge missing: %+v", rows)
	}

	database.SetSnapshotProvider(r.CurrentSnapshotHash)
	s, _ := database.CreateSession(ctx, db.Session{AgentID: dev.ID})
	if s.SnapshotHash != h2 {
		t.Fatalf("session stamp = %q, want %q", s.SnapshotHash, h2)
	}
}
