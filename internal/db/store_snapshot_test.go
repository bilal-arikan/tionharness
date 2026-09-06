package db

import (
	"context"
	"testing"
	"time"
)

// TestSnapshotStoreAndSessionStamp: a registered provider stamps every new
// session; SaveSnapshot writes content once, keeps first/last seen and the
// previous-hash edge; ListSnapshots orders oldest first.
func TestSnapshotStoreAndSessionStamp(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	// Before a provider exists, sessions are unstamped.
	a, _ := d.CreateAgent(ctx, Agent{Name: "A"})
	s0, _ := d.CreateSession(ctx, Session{AgentID: a.ID})
	if s0.SnapshotHash != "" {
		t.Fatalf("unstamped expected, got %q", s0.SnapshotHash)
	}
	hash := "h1"
	d.SetSnapshotProvider(func() string { return hash })
	s1, _ := d.CreateSession(ctx, Session{AgentID: a.ID})
	if s1.SnapshotHash != "h1" {
		t.Fatalf("stamp = %q", s1.SnapshotHash)
	}
	// An explicit stamp (a replayed header) is never overwritten.
	s2, _ := d.CreateSession(ctx, Session{AgentID: a.ID, SnapshotHash: "old"})
	if s2.SnapshotHash != "old" {
		t.Fatalf("explicit stamp overwritten: %q", s2.SnapshotHash)
	}

	type body struct{ V int }
	if err := d.SaveSnapshot(ctx, "h1", body{1}, 100); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveSnapshot(ctx, "h1", body{999}, 150); err != nil { // same hash: content not rewritten
		t.Fatal(err)
	}
	if err := d.SaveSnapshot(ctx, "h2", body{2}, 200); err != nil {
		t.Fatal(err)
	}
	var got body
	if err := d.GetSnapshot(ctx, "h1", &got); err != nil || got.V != 1 {
		t.Fatalf("h1 content = %+v %v", got, err)
	}
	rows, current, err := d.ListSnapshots(ctx)
	if err != nil || current != "h2" || len(rows) != 2 {
		t.Fatalf("list: %v current=%s rows=%d", err, current, len(rows))
	}
	if rows[0].Hash != "h1" || rows[0].FirstSeen != 100 || rows[0].LastSeen != 150 || rows[0].Prev != "" {
		t.Fatalf("h1 row = %+v", rows[0])
	}
	if rows[1].Hash != "h2" || rows[1].Prev != "h1" {
		t.Fatalf("h2 row = %+v", rows[1])
	}
	if err := d.GetSnapshot(ctx, "nope", &got); err != ErrNotFound {
		t.Fatalf("missing snapshot: %v", err)
	}
	counts := d.CountSessionsBySnapshot(ctx)
	if counts[""] != 1 || counts["h1"] != 1 || counts["old"] != 1 {
		t.Fatalf("counts = %v", counts)
	}
	if len(d.ListSessionUsage(ctx)) != 0 || len(d.ListSessionAsks(ctx)) != 0 {
		t.Fatal("fresh store must list no usage / asks")
	}
}

// TestSnapshotProviderMayReadStore: the provider is invoked outside the write
// lock on every creation path, so a provider that reads the store (as the
// runtime's does) never deadlocks.
func TestSnapshotProviderMayReadStore(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()
	a, _ := d.CreateAgent(ctx, Agent{Name: "A"})
	d.SetSnapshotProvider(func() string {
		agents, _ := d.ListAgents(ctx) // takes d.mu.RLock
		return "n" + string(rune('0'+len(agents)))
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		s, _ := d.CreateSession(ctx, Session{AgentID: a.ID})
		if s.SnapshotHash != "n1" {
			t.Errorf("CreateSession stamp = %q", s.SnapshotHash)
		}
		child, err := d.CreateChildSession(ctx, Session{
			AgentID: a.ID, ParentSessionID: s.ID, Kind: "worker", TargetAgentID: a.ID,
			ExecutionType: ExecutionWorker, Category: CategoryWorker, ContextMode: ContextIsolated, Visibility: VisibilityInternal,
		})
		if err != nil || child.SnapshotHash != "n1" {
			t.Errorf("child stamp = %q (%v)", child.SnapshotHash, err)
		}
		src, _ := d.GetOrCreateSourceSession(ctx, "task", "TSK1", a.ID, "t")
		if src.SnapshotHash != "n1" {
			t.Errorf("source session stamp = %q", src.SnapshotHash)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("session creation deadlocked with a store-reading snapshot provider")
	}
}
