package db

import (
	"context"
	"testing"
)

// HiddenTools must persist independently of DisabledTools and survive a reload.
func TestWorkspaceToolConfig_HiddenRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := d.SetWorkspaceToolConfig(ctx, WorkspaceToolConfig{
		DisabledTools: []string{"WebFetch"},
		HiddenTools:   []string{"list_sessions", "secret_get"},
		ShownTools:    []string{"create_agent"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Reopen to confirm it was written to disk, not just held in memory.
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	cfg, err := d2.GetWorkspaceToolConfig(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(cfg.DisabledTools) != 1 || cfg.DisabledTools[0] != "WebFetch" {
		t.Fatalf("disabled = %v", cfg.DisabledTools)
	}
	if len(cfg.HiddenTools) != 2 || cfg.HiddenTools[0] != "list_sessions" {
		t.Fatalf("hidden = %v", cfg.HiddenTools)
	}
	if len(cfg.ShownTools) != 1 || cfg.ShownTools[0] != "create_agent" {
		t.Fatalf("shown = %v", cfg.ShownTools)
	}
}
