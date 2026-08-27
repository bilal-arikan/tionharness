package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestUpdateMCPServerPreservesIdentity(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	created, err := d.CreateMCPServer(ctx, MCPServer{
		Name:      "mcp-alpha",
		Transport: MCPTransportHTTP,
		URL:       "http://127.0.0.1:1/mcp",
		Enabled:   true,
		Scope:     "shared",
		CreatedBy: "AGT9", // agent-created: must survive an edit
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := d.UpdateMCPServer(ctx, created.ID, MCPServer{
		Name:      "mcp-beta",
		Transport: MCPTransportHTTP,
		URL:       "http://127.0.0.1:2/mcp",
		Scope:     "scoped",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Identity + enabled preserved.
	if updated.ID != created.ID {
		t.Errorf("ID changed: %q → %q", created.ID, updated.ID)
	}
	if updated.CreatedAt != created.CreatedAt {
		t.Errorf("CreatedAt changed: %d → %d", created.CreatedAt, updated.CreatedAt)
	}
	if updated.CreatedBy != "AGT9" {
		t.Errorf("CreatedBy not preserved: %q", updated.CreatedBy)
	}
	if !updated.Enabled {
		t.Error("Enabled should be preserved (true)")
	}
	// Editable fields applied.
	if updated.Name != "mcp-beta" || updated.URL != "http://127.0.0.1:2/mcp" || updated.Scope != "scoped" {
		t.Errorf("fields not applied: %+v", updated)
	}

	// Persisted (reload).
	got, err := d.GetMCPServer(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Scope != "scoped" || got.Name != "mcp-beta" {
		t.Errorf("update not persisted: %+v", got)
	}
}

func TestUpdateMCPServerDefaultsAndNotFound(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	created, err := d.CreateMCPServer(ctx, MCPServer{
		Name: "s", Transport: MCPTransportStdio, Command: "x", Enabled: true, Scope: "scoped",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Empty scope on update falls back to the store default.
	updated, err := d.UpdateMCPServer(ctx, created.ID, MCPServer{
		Name: "s", Transport: MCPTransportStdio, Command: "x", Scope: "",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Scope != "shared" {
		t.Errorf("empty scope should default to shared, got %q", updated.Scope)
	}

	// Unknown id is a clean ErrNotFound, not a silent no-op.
	if _, err := d.UpdateMCPServer(ctx, "MCP999", MCPServer{Name: "z"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound for unknown id, got %v", err)
	}
}
