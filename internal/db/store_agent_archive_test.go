package db

import (
	"context"
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/archive"
)

// openArchiveTestDB opens a throwaway store.
func openArchiveTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestSetAgentArchivedRoundTrip(t *testing.T) {
	d := openArchiveTestDB(t)
	ctx := context.Background()
	a, err := d.CreateAgent(ctx, Agent{Name: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RunnableErr(); err != nil {
		t.Fatalf("a live agent must be runnable: %v", err)
	}

	got, err := d.SetAgentArchived(ctx, a.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Archived || got.ArchivedAt == 0 {
		t.Fatalf("archived=%v archivedAt=%d", got.Archived, got.ArchivedAt)
	}
	if err := got.RunnableErr(); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("RunnableErr = %v, want ErrArchived", err)
	}
	// Archived agents stay readable and listed: the roster filters, the store
	// does not hide them (unlike Deleted).
	reread, err := d.GetAgent(ctx, a.ID)
	if err != nil || !reread.Archived {
		t.Fatalf("GetAgent after archive: %+v, %v", reread, err)
	}
	list, _ := d.ListAgents(ctx)
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("ListAgents must keep archived agents: %+v", list)
	}

	got, err = d.SetAgentArchived(ctx, a.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Archived || got.ArchivedAt != 0 || got.RunnableErr() != nil {
		t.Fatalf("unarchive left %+v", got)
	}
}

func TestSetAgentArchivedRefusals(t *testing.T) {
	d := openArchiveTestDB(t)
	ctx := context.Background()
	if _, err := d.SetAgentArchived(ctx, "AGT404", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing agent: %v", err)
	}
	sys, err := d.CreateAgent(ctx, Agent{Name: "sys", System: true, SystemKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentArchived(ctx, sys.ID, true); !errors.Is(err, ErrSystemAgentArchive) {
		t.Fatalf("system agent: %v", err)
	}
	gone, _ := d.CreateAgent(ctx, Agent{Name: "gone"})
	if err := d.DeleteAgent(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentArchived(ctx, gone.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted agent: %v", err)
	}
}
