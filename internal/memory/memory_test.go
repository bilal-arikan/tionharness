package memory

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
)

// newTestStore opens a throwaway file-backed DB and wraps it in a memory Store.
func newTestStore(t *testing.T) (*Store, *db.DB, string) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return New(d), d, "agent-1"
}

// TestPruneKindKeepsNewest verifies the journal ring-buffer drops the oldest
// entries beyond the cap while leaving other kinds untouched.
func TestPruneKindKeepsNewest(t *testing.T) {
	ctx := context.Background()
	s, d, agentID := newTestStore(t)
	defer d.Close()

	for i := 0; i < 8; i++ {
		if _, err := s.Remember(ctx, agentID, db.MemoryJournal, "j"); err != nil {
			t.Fatalf("remember journal %d: %v", i, err)
		}
	}
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, "durable fact"); err != nil {
		t.Fatalf("remember document: %v", err)
	}

	deleted, err := s.PruneKind(ctx, agentID, db.MemoryJournal, 3)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if deleted != 5 {
		t.Fatalf("deleted = %d, want 5", deleted)
	}

	journals, err := s.List(ctx, agentID, db.MemoryJournal)
	if err != nil {
		t.Fatalf("list journals: %v", err)
	}
	if len(journals) != 3 {
		t.Fatalf("journals kept = %d, want 3", len(journals))
	}
	docs, err := s.List(ctx, agentID, db.MemoryDocument)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("documents = %d, want 1 (prune must not touch other kinds)", len(docs))
	}

	// Pruning when already under the cap is a no-op.
	deleted, err = s.PruneKind(ctx, agentID, db.MemoryJournal, 10)
	if err != nil || deleted != 0 {
		t.Fatalf("prune under cap: deleted=%d err=%v", deleted, err)
	}
}

// TestDeleteIDsIgnoresMissing ensures consuming journals after a reflection is
// idempotent and tolerant of already-removed ids.
func TestDeleteIDsIgnoresMissing(t *testing.T) {
	ctx := context.Background()
	s, d, agentID := newTestStore(t)
	defer d.Close()

	a, err := s.Remember(ctx, agentID, db.MemoryJournal, "one")
	if err != nil {
		t.Fatalf("remember: %v", err)
	}
	if err := s.DeleteIDs(ctx, a.ID, "does-not-exist"); err != nil {
		t.Fatalf("delete ids: %v", err)
	}
	journals, _ := s.List(ctx, agentID, db.MemoryJournal)
	if len(journals) != 0 {
		t.Fatalf("journals = %d, want 0", len(journals))
	}
}
