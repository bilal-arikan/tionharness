package db

import (
	"context"
	"testing"
)

// TestBackfillThinkingLevels covers the legacy migration of the ambiguous empty
// ThinkingLevel: CLI rows become "high" (what cliEffortLevel already did with
// ""), every other provider becomes "off" (what a 0 thinking budget already
// did), and a row that already names a level is never rewritten.
func TestBackfillThinkingLevels(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	// CreateAgent resolves a blank level itself, so the legacy state is written
	// directly onto the stored rows to reproduce what is on disk today.
	mk := func(provider, level string) string {
		a, err := d.CreateAgent(ctx, Agent{Name: "a", Provider: provider})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if _, err := d.mutateAgentLocked(a.ID, func(x *Agent) { x.ThinkingLevel = level }); err != nil {
			t.Fatalf("seed level: %v", err)
		}
		return a.ID
	}

	cli := mk("claude-cli", "")
	codex := mk("codex-cli", "")
	native := mk("anthropic", "")
	noProvider := mk("", "")
	explicit := mk("anthropic", "medium")
	explicitOff := mk("claude-cli", "off")

	migrated, err := d.BackfillThinkingLevels(ctx)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if migrated != 4 {
		t.Fatalf("expected 4 migrated rows, got %d", migrated)
	}

	want := map[string]string{
		cli:         "high",
		codex:       "high",
		native:      "off",
		noProvider:  "high", // empty provider reads as claude-cli
		explicit:    "medium",
		explicitOff: "off",
	}
	for id, level := range want {
		got, err := d.GetAgent(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if got.ThinkingLevel != level {
			t.Fatalf("agent %s (provider %q): want %q, got %q", id, got.Provider, level, got.ThinkingLevel)
		}
	}

	// Idempotent: a second pass has nothing left to do.
	again, err := d.BackfillThinkingLevels(ctx)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if again != 0 {
		t.Fatalf("expected second pass to be a no-op, migrated %d", again)
	}
}

// TestCreateAgentResolvesBlankThinkingLevel covers the creation paths that do
// not go through the API (pack install, templates, self-management tool): they
// may omit the level, and the store must not persist the ambiguous empty value.
func TestCreateAgentResolvesBlankThinkingLevel(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	cli, err := d.CreateAgent(ctx, Agent{Name: "cli", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create cli: %v", err)
	}
	if cli.ThinkingLevel != "high" {
		t.Fatalf("cli agent: want high, got %q", cli.ThinkingLevel)
	}

	native, err := d.CreateAgent(ctx, Agent{Name: "native", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create native: %v", err)
	}
	if native.ThinkingLevel != "off" {
		t.Fatalf("native agent: want off, got %q", native.ThinkingLevel)
	}

	explicit, err := d.CreateAgent(ctx, Agent{Name: "explicit", Provider: "claude-cli", ThinkingLevel: "low"})
	if err != nil {
		t.Fatalf("create explicit: %v", err)
	}
	if explicit.ThinkingLevel != "low" {
		t.Fatalf("explicit level was overwritten: %q", explicit.ThinkingLevel)
	}
}
