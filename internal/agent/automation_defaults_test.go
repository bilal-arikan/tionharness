package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func newAutomationSeedDB(t *testing.T) (*db.DB, string) {
	t.Helper()
	storeDir := filepath.Join(t.TempDir(), "store")
	database, err := db.Open(storeDir)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, storeDir
}

// TestEnsureDefaultBoardAutomationsSeeds seeds every shipped default exactly once,
// disabled, and stays idempotent across repeated opens.
func TestEnsureDefaultBoardAutomationsSeeds(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)

	if err := EnsureDefaultBoardAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	if len(autos) != len(defaultBoardAutomations) {
		t.Fatalf("want %d seeded automations, got %d", len(defaultBoardAutomations), len(autos))
	}
	for _, a := range autos {
		if a.Seed == "" {
			t.Fatalf("seeded automation %q missing Seed key", a.ID)
		}
		if a.TriggerKind != db.TriggerBoard {
			t.Fatalf("seeded automation %q is not a board trigger: %q", a.ID, a.TriggerKind)
		}
		if a.Enabled {
			t.Fatalf("seeded automation %q must be disabled (opt-in)", a.ID)
		}
	}

	// Second call must not duplicate.
	if err := EnsureDefaultBoardAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	autos, _ = database.ListAutomations(ctx)
	if len(autos) != len(defaultBoardAutomations) {
		t.Fatalf("idempotency broken: got %d automations", len(autos))
	}
}

// TestEnsureDefaultBoardAutomationsSeedsActionsAndColumns pins the two shipped
// rules to their contract: an in_progress→spawn rule and a done→archive rule.
func TestEnsureDefaultBoardAutomationsSeedsActionsAndColumns(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)
	if err := EnsureDefaultBoardAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	bySeed := map[string]db.Automation{}
	for _, a := range autos {
		bySeed[a.Seed] = a
	}
	run, ok := bySeed["board-run-in-progress"]
	if !ok {
		t.Fatal("missing board-run-in-progress rule")
	}
	if run.BoardToState != db.BoardInProgress || run.BoardAction != db.BoardActionSpawn {
		t.Fatalf("run rule wrong wiring: to=%q action=%q", run.BoardToState, run.BoardAction)
	}
	arch, ok := bySeed["board-archive-done"]
	if !ok {
		t.Fatal("missing board-archive-done rule")
	}
	if arch.BoardToState != db.BoardDone || arch.BoardAction != db.BoardActionArchive {
		t.Fatalf("archive rule wrong wiring: to=%q action=%q", arch.BoardToState, arch.BoardAction)
	}
}

// TestEnsureDefaultBoardAutomationsRespectsDeletion: a user-deleted default is not
// resurrected on the next open (deletion ledger).
func TestEnsureDefaultBoardAutomationsRespectsDeletion(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)

	if err := EnsureDefaultBoardAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	if len(autos) == 0 {
		t.Fatal("no seeded automations")
	}
	if err := database.DeleteAutomation(ctx, autos[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if err := EnsureDefaultBoardAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	after, _ := database.ListAutomations(ctx)
	if len(after) != len(autos)-1 {
		t.Fatalf("deleted default was resurrected: want %d, got %d", len(autos)-1, len(after))
	}
}
