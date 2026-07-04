package db

import (
	"context"
	"testing"
)

// ToolVisibility + DisabledTools must persist independently and survive a reload.
func TestWorkspaceToolConfig_VisibilityRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := d.SetWorkspaceToolConfig(ctx, WorkspaceToolConfig{
		DisabledTools: []string{"WebFetch"},
		ToolVisibility: map[string]string{
			"list_sessions": "name-only",
			"create_agent":  "full",
			"secret":        "hidden",
		},
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
	if cfg.ToolVisibility["list_sessions"] != "name-only" ||
		cfg.ToolVisibility["create_agent"] != "full" ||
		cfg.ToolVisibility["secret"] != "hidden" {
		t.Fatalf("visibility = %v", cfg.ToolVisibility)
	}
	// Legacy lists must NOT be written back.
	if len(cfg.HiddenTools) != 0 || len(cfg.ShownTools) != 0 {
		t.Fatalf("legacy lists should be empty: hidden=%v shown=%v", cfg.HiddenTools, cfg.ShownTools)
	}
}

// A legacy config file (HiddenTools/ShownTools lists) must migrate into the
// ToolVisibility map on load: HiddenTools→name-only, ShownTools→full.
func TestWorkspaceToolConfig_LegacyMigration(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Write a legacy-shaped file directly (Set would clear the lists).
	legacy := WorkspaceToolConfig{
		DisabledTools: []string{"Bash"},
		HiddenTools:   []string{"list_sessions"},
		ShownTools:    []string{"create_agent"},
	}
	if err := atomicWriteJSON(d.dir(toolConfigFile), legacy); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	d2, err := Open(dir) // Open → loadToolConfig migrates
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	cfg, err := d2.GetWorkspaceToolConfig(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if cfg.ToolVisibility["list_sessions"] != "name-only" {
		t.Fatalf("HiddenTools should migrate to name-only: %v", cfg.ToolVisibility)
	}
	if cfg.ToolVisibility["create_agent"] != "full" {
		t.Fatalf("ShownTools should migrate to full: %v", cfg.ToolVisibility)
	}
}
