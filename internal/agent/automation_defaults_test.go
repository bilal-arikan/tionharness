package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func newAutomationSeedDB(t *testing.T) (*db.DB, string) {
	t.Helper()
	storeDir := filepath.Join(t.TempDir(), "store")
	database, err := db.Open(storeDir)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	// The spawn seed needs a real target agent to pass ValidateAutomationShape at
	// seed time; a workspace normally has one before board automations matter.
	if _, err := database.CreateAgent(context.Background(), db.Agent{Name: "Seed", Provider: "anthropic"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// The system agents are seeded before the automations at workspace open, so a
	// seed pinned to a SystemKey (insight-applier) can resolve its target here too.
	if err := database.EnsureSystemAgents(context.Background(), SystemAgentDefaults()...); err != nil {
		t.Fatalf("seed system agents: %v", err)
	}
	return database, storeDir
}

// TestEnsureDefaultBoardAutomationsDefersSpawnWithoutAgent locks the seed-time
// shape contract: when a workspace has no agent yet, the spawn seed (empty target)
// fails ValidateAutomationShape and is skipped WITHOUT being recorded in the
// deletion ledger, so a later startup — once an agent exists — backfills it. The
// same holds for a seed pinned to a system agent that is not present yet. The
// archive seed needs no target and seeds immediately.
func TestEnsureDefaultBoardAutomationsDefersSpawnWithoutAgent(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	database, err := db.Open(storeDir)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// No agent yet.
	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed (no agent): %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	if len(autos) != 1 || autos[0].Seed != "board-archive-done" {
		t.Fatalf("without an agent only the archive rule should seed, got %d: %+v", len(autos), autos)
	}

	// Now an agent exists → the deferred spawn rule backfills on the next startup.
	if _, err := database.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := database.EnsureSystemAgents(ctx, SystemAgentDefaults()...); err != nil {
		t.Fatalf("seed system agents: %v", err)
	}
	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed (with agent): %v", err)
	}
	autos, _ = database.ListAutomations(ctx)
	if len(autos) != len(defaultAutomations) {
		t.Fatalf("spawn rule not backfilled: want %d, got %d", len(defaultAutomations), len(autos))
	}
	for _, a := range autos {
		if err := db.ValidateAutomationShape(a); err != nil {
			t.Errorf("seeded rule %q is not shape-valid: %v", a.Seed, err)
		}
	}
}

// TestEnsureDefaultBoardAutomationsSeeds seeds every shipped default exactly once,
// disabled, and stays idempotent across repeated opens.
func TestEnsureDefaultBoardAutomationsSeeds(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)

	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	if len(autos) != len(defaultAutomations) {
		t.Fatalf("want %d seeded automations, got %d", len(defaultAutomations), len(autos))
	}
	for _, a := range autos {
		if a.Seed == "" {
			t.Fatalf("seeded automation %q missing Seed key", a.ID)
		}
		if a.Enabled {
			t.Fatalf("seeded automation %q must be disabled (opt-in)", a.ID)
		}
	}

	// Second call must not duplicate.
	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	autos, _ = database.ListAutomations(ctx)
	if len(autos) != len(defaultAutomations) {
		t.Fatalf("idempotency broken: got %d automations", len(autos))
	}
}

// TestEnsureDefaultBoardAutomationsSeedsActionsAndColumns pins the two shipped
// rules to their contract: an in_progress→spawn rule and a done→archive rule.
func TestEnsureDefaultBoardAutomationsSeedsActionsAndColumns(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)
	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
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

// TestEnsureDefaultAutomationsSeedsInsightApplier pins the shipped insight-apply
// rule: it is a tag trigger aimed at the insight-applier SYSTEM agent (never the
// workspace's first agent), spawns a fresh session, and breaks the self-loop by
// spawning with no tags.
func TestEnsureDefaultAutomationsSeedsInsightApplier(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)
	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	var rule db.Automation
	for _, a := range autos {
		if a.Seed == "insight-apply-workspace-opt" {
			rule = a
		}
	}
	if rule.ID == "" {
		t.Fatal("missing insight-apply-workspace-opt rule")
	}
	if rule.TriggerKind != db.TriggerTag || rule.TriggerTag != insightScanSessionTag {
		t.Fatalf("wrong trigger: kind=%q tag=%q", rule.TriggerKind, rule.TriggerTag)
	}
	applier, ok := database.FindAgentBySystemKey("insight-applier")
	if !ok {
		t.Fatal("insight-applier system agent not seeded")
	}
	if rule.TargetAgentID != applier.ID {
		t.Fatalf("target = %q, want the insight-applier agent %q", rule.TargetAgentID, applier.ID)
	}
	if rule.SessionMode != db.SessionModeSpawn {
		t.Fatalf("sessionMode = %q, want spawn", rule.SessionMode)
	}
	for _, tag := range rule.SpawnTags {
		if tag == rule.TriggerTag {
			t.Fatalf("spawnTags %#v carry the trigger tag: the applier session would re-fire the rule", rule.SpawnTags)
		}
	}
	if len(rule.SpawnTags) == 0 {
		t.Fatal("spawnTags must be non-empty: nil defaults to the trigger tag at fire time")
	}
	if rule.Enabled {
		t.Fatal("insight-apply rule must ship disabled")
	}
	if rule.MaxIterations <= 0 || rule.CooldownSec <= 0 {
		t.Fatalf("missing guardrails: maxIterations=%d cooldownSec=%d", rule.MaxIterations, rule.CooldownSec)
	}
	if err := db.ValidateAutomationShape(rule); err != nil {
		t.Fatalf("seeded rule is not shape-valid: %v", err)
	}
}

// TestEnsureDefaultBoardAutomationsRespectsDeletion: a user-deleted default is not
// resurrected on the next open (deletion ledger).
func TestEnsureDefaultBoardAutomationsRespectsDeletion(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newAutomationSeedDB(t)

	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	autos, _ := database.ListAutomations(ctx)
	if len(autos) == 0 {
		t.Fatal("no seeded automations")
	}
	if err := database.DeleteAutomation(ctx, autos[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if err := EnsureDefaultAutomations(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	after, _ := database.ListAutomations(ctx)
	if len(after) != len(autos)-1 {
		t.Fatalf("deleted default was resurrected: want %d, got %d", len(autos)-1, len(after))
	}
}
