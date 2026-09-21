package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestEphemeralFlowHiddenFromCatalog: the hidden row behind a run_adhoc_flow run
// must never appear in the flow catalog, yet stay resolvable by id — the flowrun
// view and run lineage read it through GetFlow — and survive a reopen.
func TestEphemeralFlowHiddenFromCatalog(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	saved, err := d.CreateFlow(ctx, Flow{Name: "saved"})
	if err != nil {
		t.Fatalf("create saved: %v", err)
	}
	eph, err := d.CreateFlow(ctx, Flow{Name: "adhoc", Ephemeral: true})
	if err != nil {
		t.Fatalf("create ephemeral: %v", err)
	}

	assertCatalog := func(d *DB) {
		t.Helper()
		list, err := d.ListFlows(ctx)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) != 1 || list[0].ID != saved.ID {
			t.Fatalf("catalog must hold only the saved flow, got %+v", list)
		}
		got, err := d.GetFlow(ctx, eph.ID)
		if err != nil {
			t.Fatalf("ephemeral flow must stay resolvable by id: %v", err)
		}
		if !got.Ephemeral {
			t.Fatal("ephemeral flag lost")
		}
	}
	assertCatalog(d)

	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = d2.Close() })
	assertCatalog(d2)
}
