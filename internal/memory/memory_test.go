package memory

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
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

// TestCoreMemoryUpsert verifies the per-section single-row invariant: repeated
// WriteCore replaces a section in place, AppendCore adds a line, persona and
// human are independent, and ReadCore returns "" before any write.
func TestCoreMemoryUpsert(t *testing.T) {
	ctx := context.Background()
	s, d, agentID := newTestStore(t)
	defer d.Close()

	got, err := s.ReadCore(ctx, agentID, CorePersona)
	if err != nil {
		t.Fatalf("read empty core: %v", err)
	}
	if got != "" {
		t.Fatalf("empty persona = %q, want \"\"", got)
	}

	if err := s.WriteCore(ctx, agentID, CorePersona, "role: assistant"); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	if err := s.WriteCore(ctx, agentID, CorePersona, "role: senior assistant"); err != nil {
		t.Fatalf("rewrite persona: %v", err)
	}
	got, _ = s.ReadCore(ctx, agentID, CorePersona)
	if got != "role: senior assistant" {
		t.Fatalf("persona = %q after rewrite", got)
	}
	// Upsert invariant: exactly one persona row regardless of write count.
	rows, _ := s.List(ctx, agentID, db.CoreKind(CorePersona))
	if len(rows) != 1 {
		t.Fatalf("persona rows = %d, want 1 (upsert must not accumulate)", len(rows))
	}

	// human is independent of persona.
	if err := s.WriteCore(ctx, agentID, CoreHuman, "name: Bilal"); err != nil {
		t.Fatalf("write human: %v", err)
	}
	if err := s.AppendCore(ctx, agentID, CoreHuman, "city: Istanbul"); err != nil {
		t.Fatalf("append human: %v", err)
	}
	human, _ := s.ReadCore(ctx, agentID, CoreHuman)
	if human != "name: Bilal\ncity: Istanbul" {
		t.Fatalf("human = %q after append", human)
	}
	// persona untouched by the human writes.
	got, _ = s.ReadCore(ctx, agentID, CorePersona)
	if got != "role: senior assistant" {
		t.Fatalf("persona changed by human writes: %q", got)
	}

	// ReadCoreBlocks returns every block with content, ordered.
	views, _ := s.ReadCoreBlocks(ctx, agentID)
	got2 := map[string]string{}
	for _, v := range views {
		got2[v.Label] = v.Content
	}
	if got2[CorePersona] != "role: senior assistant" || got2[CoreHuman] != "name: Bilal\ncity: Istanbul" {
		t.Fatalf("blocks = %+v", got2)
	}
}

// TestCoreBlockCharLimit verifies a write past a block's character limit is
// rejected with *CoreBlockFullError and leaves the block unchanged.
func TestCoreBlockCharLimit(t *testing.T) {
	ctx := context.Background()
	s, d, _ := newTestStore(t)
	defer d.Close()

	// Create a real agent with a tiny custom block.
	ag, err := d.CreateAgent(ctx, db.Agent{Name: "Limited"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := s.DefineCoreBlock(ctx, ag.ID, db.CoreBlock{Label: "tiny", CharLimit: 10}); err != nil {
		t.Fatalf("define block: %v", err)
	}
	if err := s.WriteCore(ctx, ag.ID, "tiny", "ok"); err != nil {
		t.Fatalf("write within limit: %v", err)
	}
	err = s.WriteCore(ctx, ag.ID, "tiny", "this is way over ten characters")
	var full *CoreBlockFullError
	if !errors.As(err, &full) {
		t.Fatalf("expected CoreBlockFullError, got %v", err)
	}
	if full.Limit != 10 || full.Count <= 10 {
		t.Fatalf("full = %+v, want limit 10 and count>10", full)
	}
	// Unchanged after the rejected write.
	got, _ := s.ReadCore(ctx, ag.ID, "tiny")
	if got != "ok" {
		t.Fatalf("block changed after rejected write: %q", got)
	}
	// Unknown label is rejected.
	if err := s.WriteCore(ctx, ag.ID, "nope", "x"); !errors.Is(err, ErrUnknownCoreBlock) {
		t.Fatalf("expected ErrUnknownCoreBlock, got %v", err)
	}
}

// TestDefineAndDeleteCoreBlock verifies custom blocks seed the defaults, persist,
// and that deletion removes both the definition and its content.
func TestDefineAndDeleteCoreBlock(t *testing.T) {
	ctx := context.Background()
	s, d, _ := newTestStore(t)
	defer d.Close()

	ag, err := d.CreateAgent(ctx, db.Agent{Name: "Blocks"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := s.DefineCoreBlock(ctx, ag.ID, db.CoreBlock{Label: "project", Description: "current project"}); err != nil {
		t.Fatalf("define: %v", err)
	}
	blocks, _ := s.CoreBlocks(ctx, ag.ID)
	// Defining a custom block must seed the defaults too (persona, human, project).
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3 (persona+human+project)", len(blocks))
	}
	if err := s.WriteCore(ctx, ag.ID, "project", "ship v2"); err != nil {
		t.Fatalf("write project: %v", err)
	}
	if err := s.DeleteCoreBlock(ctx, ag.ID, "project"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	blocks, _ = s.CoreBlocks(ctx, ag.ID)
	if len(blocks) != 2 {
		t.Fatalf("blocks after delete = %d, want 2", len(blocks))
	}
	// Content row is gone too.
	rows, _ := s.List(ctx, ag.ID, db.CoreKind("project"))
	if len(rows) != 0 {
		t.Fatalf("orphan project rows = %d, want 0", len(rows))
	}
}

// TestRecallExcludesCore ensures the always-injected core sections never surface
// through similarity recall (which would inject them twice).
func TestRecallExcludesCore(t *testing.T) {
	ctx := context.Background()
	s, d, agentID := newTestStore(t)
	defer d.Close()

	if err := s.WriteCore(ctx, agentID, CorePersona, "the sky is blue today"); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	if err := s.WriteCore(ctx, agentID, CoreHuman, "the sky is blue today"); err != nil {
		t.Fatalf("write human: %v", err)
	}
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, "the sky is blue today"); err != nil {
		t.Fatalf("remember document: %v", err)
	}

	hits, err := s.Recall(ctx, agentID, "what color is the sky", 5)
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	for _, h := range hits {
		if db.IsCoreKind(h.Source.Kind) {
			t.Fatalf("recall returned a core memory (%s); core must be excluded", h.Source.Kind)
		}
	}
	if len(hits) == 0 {
		t.Fatalf("recall returned nothing; expected the document hit")
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
