package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeWorkspaceBridge is an in-memory WorkspaceBridge for testing the workspace
// self-management tools' guards without a real manager.
type fakeWorkspaceBridge struct {
	list    []WorkspaceInfo
	deleted []string
}

func (f *fakeWorkspaceBridge) ListWorkspaces() []WorkspaceInfo { return f.list }

func (f *fakeWorkspaceBridge) CreateWorkspace(name, parentPath, createdBy string) (WorkspaceInfo, error) {
	info := WorkspaceInfo{ID: "ws-new", Name: name, Path: parentPath, CreatedByAgent: createdBy != ""}
	f.list = append(f.list, info)
	return info, nil
}

func (f *fakeWorkspaceBridge) RenameWorkspace(id, name string) (WorkspaceInfo, error) {
	for i := range f.list {
		if f.list[i].ID == id {
			f.list[i].Name = name
			return f.list[i], nil
		}
	}
	return WorkspaceInfo{}, nil
}

func (f *fakeWorkspaceBridge) DeleteWorkspace(id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

// TestCreateWorkspaceStampsCreatedBy verifies create_workspace tags the new
// workspace as agent-created via the actor id passed to the bridge.
func TestCreateWorkspaceStampsCreatedBy(t *testing.T) {
	ctx := context.Background()
	b := &fakeWorkspaceBridge{}
	create := NewCreateWorkspaceTool(b, "actor-1", "ws-current")

	if _, err := create.Call(ctx, json.RawMessage(`{"name":""}`)); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	out, err := create.Call(ctx, json.RawMessage(`{"name":"Research"}`))
	if err != nil {
		t.Fatalf("create_workspace: %v", err)
	}
	var res struct{ ID, Action string }
	_ = json.Unmarshal([]byte(out), &res)
	if res.Action != "created" || res.ID != "ws-new" {
		t.Fatalf("unexpected result: %s", out)
	}
	if len(b.list) != 1 || !b.list[0].CreatedByAgent {
		t.Fatalf("workspace not stamped agent-created: %+v", b.list)
	}
}

// TestDeleteWorkspaceGuards verifies delete_workspace refuses user-created
// workspaces, the current workspace, and unknown ids — and deletes an
// agent-created one.
func TestDeleteWorkspaceGuards(t *testing.T) {
	ctx := context.Background()
	b := &fakeWorkspaceBridge{list: []WorkspaceInfo{
		{ID: "ws-current", Name: "Current", CreatedByAgent: true},
		{ID: "ws-user", Name: "UserMade", CreatedByAgent: false},
		{ID: "ws-agent", Name: "AgentMade", CreatedByAgent: true},
	}}
	del := NewDeleteWorkspaceTool(b, "actor-1", "ws-current")

	// Current workspace cannot be deleted.
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"ws-current"}`)); err == nil || !strings.Contains(err.Error(), "currently running in") {
		t.Fatalf("expected current-ws delete rejected, got %v", err)
	}
	// User-created workspace cannot be deleted by an agent.
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"ws-user"}`)); err == nil || !strings.Contains(err.Error(), "created by the user") {
		t.Fatalf("expected user-ws delete rejected, got %v", err)
	}
	// Unknown id is rejected.
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"nope"}`)); err == nil || !strings.Contains(err.Error(), "no workspace") {
		t.Fatalf("expected unknown-ws delete rejected, got %v", err)
	}
	// Agent-created workspace is deleted.
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"ws-agent"}`)); err != nil {
		t.Fatalf("agent-ws delete should succeed: %v", err)
	}
	if len(b.deleted) != 1 || b.deleted[0] != "ws-agent" {
		t.Fatalf("expected ws-agent deleted, got %v", b.deleted)
	}
}
