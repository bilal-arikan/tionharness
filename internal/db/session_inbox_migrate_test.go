package db

import (
	"context"
	"testing"
)

// TestMigrateLegacyInboxSessions converts a pre-TSK507 inbox transcript into the
// writable chat thread new deliveries append to, and — the point of the whole
// migration — makes GetOrCreateSourceSession find THAT session rather than open
// a second thread beside it, which would split the agent's history in two.
func TestMigrateLegacyInboxSessions(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	agent, err := d.CreateAgent(ctx, Agent{Name: "Kai"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	legacy, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: legacyInboxKind, Title: "📥 Inbox"})
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}

	converted, failed := d.migrateLegacyInboxSessions()
	if converted != 1 || failed != 0 {
		t.Fatalf("converted=%d failed=%d, want 1/0", converted, failed)
	}

	got, err := d.GetSession(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Kind != peerThreadKind {
		t.Errorf("kind = %q, want %q", got.Kind, peerThreadKind)
	}
	if want := PeerThreadSourceID(agent.ID); got.SourceID != want {
		t.Errorf("sourceID = %q, want %q", got.SourceID, want)
	}
	if !IsWritableSessionKind(got.Kind) {
		t.Error("a migrated peer thread must be writable — that is why it was migrated")
	}

	// The delivery-side lookup must land on the migrated session.
	found, err := d.GetOrCreateSourceSession(ctx, peerThreadKind, PeerThreadSourceID(agent.ID), agent.ID, "💬 Kai — Mesajlar")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if found.ID != legacy.ID {
		t.Fatalf("lookup opened a second thread %q instead of reusing the migrated %q", found.ID, legacy.ID)
	}
}

// TestMigrateLegacyInboxSessionsIsIdempotent: the migration runs on every boot,
// so a second pass must convert nothing and must not disturb the session it
// already converted.
func TestMigrateLegacyInboxSessionsIsIdempotent(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	agent, _ := d.CreateAgent(ctx, Agent{Name: "Kai"})
	legacy, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: legacyInboxKind})
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}
	if converted, _ := d.migrateLegacyInboxSessions(); converted != 1 {
		t.Fatalf("first pass converted %d, want 1", converted)
	}
	first, _ := d.GetSession(ctx, legacy.ID)

	converted, failed := d.migrateLegacyInboxSessions()
	if converted != 0 || failed != 0 {
		t.Fatalf("second pass converted=%d failed=%d, want 0/0", converted, failed)
	}
	second, _ := d.GetSession(ctx, legacy.ID)
	if second.Kind != first.Kind || second.SourceID != first.SourceID {
		t.Errorf("re-run changed the session: %+v -> %+v", first, second)
	}
}

// TestMigrateLegacyInboxSessionsPreservesExistingSourceID: SourceID identifies a
// session to every other (kind, sourceID) lookup, so a non-empty one is left
// alone — overwriting it would repoint those lookups at the peer thread.
func TestMigrateLegacyInboxSessionsPreservesExistingSourceID(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	agent, _ := d.CreateAgent(ctx, Agent{Name: "Kai"})
	legacy, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: legacyInboxKind, SourceID: "someone-elses-key"})
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}
	if converted, _ := d.migrateLegacyInboxSessions(); converted != 1 {
		t.Fatal("expected the session to be converted")
	}
	got, _ := d.GetSession(ctx, legacy.ID)
	if got.SourceID != "someone-elses-key" {
		t.Errorf("sourceID = %q, want it untouched", got.SourceID)
	}
	if got.Kind != peerThreadKind {
		t.Errorf("kind = %q, want %q", got.Kind, peerThreadKind)
	}
}

// TestMigrateLegacyInboxSessionsLeavesOtherKinds guards the blast radius: only
// the legacy inbox kind is rewritten. A task/flow run log converted by accident
// would silently become writable and lose its read-only guard.
func TestMigrateLegacyInboxSessionsLeavesOtherKinds(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()

	agent, _ := d.CreateAgent(ctx, Agent{Name: "Kai"})
	kinds := []string{"chat", "task", "flow", "worker", SessionKindInsight}
	ids := make(map[string]string, len(kinds))
	for _, k := range kinds {
		s, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: k})
		if err != nil {
			t.Fatalf("create %s session: %v", k, err)
		}
		ids[k] = s.ID
	}

	if converted, failed := d.migrateLegacyInboxSessions(); converted != 0 || failed != 0 {
		t.Fatalf("converted=%d failed=%d, want 0/0 — no legacy session exists", converted, failed)
	}
	for _, k := range kinds {
		got, err := d.GetSession(ctx, ids[k])
		if err != nil {
			t.Fatalf("get %s session: %v", k, err)
		}
		if got.Kind != k {
			t.Errorf("kind %q was rewritten to %q", k, got.Kind)
		}
	}
}

// TestOpenRunsLegacyInboxMigration wires the boot path: reopening a store that
// holds a legacy inbox session must convert it, since the migration is only ever
// triggered from Open in production.
func TestOpenRunsLegacyInboxMigration(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	agent, _ := d.CreateAgent(ctx, Agent{Name: "Kai"})
	// Write the legacy shape directly, bypassing the migration this Open already ran.
	legacy, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: legacyInboxKind})
	if err != nil {
		t.Fatalf("create legacy session: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	got, err := reopened.GetSession(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Kind != peerThreadKind {
		t.Errorf("Open did not migrate the legacy session: kind = %q", got.Kind)
	}
	if want := PeerThreadSourceID(agent.ID); got.SourceID != want {
		t.Errorf("sourceID = %q, want %q", got.SourceID, want)
	}
}
