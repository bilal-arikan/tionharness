package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestCreateHookStampsCreatedBy verifies create_hook tags the new hook with the
// acting agent's id and enables it.
func TestCreateHookStampsCreatedBy(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	create := NewCreateHookTool(d, actor)
	out, err := create.Call(ctx, json.RawMessage(`{"event":"PreToolUse","matcher":"shell","command":"echo hi"}`))
	if err != nil {
		t.Fatalf("create_hook: %v", err)
	}
	var res struct{ ID string }
	_ = json.Unmarshal([]byte(out), &res)
	got, err := d.GetHook(ctx, res.ID)
	if err != nil {
		t.Fatalf("get hook: %v", err)
	}
	if got.CreatedBy != actor || !got.Enabled || got.Event != db.HookPreToolUse {
		t.Fatalf("hook wrong: %+v", got)
	}
	// Bad event is rejected.
	if _, err := create.Call(ctx, json.RawMessage(`{"event":"Nope","command":"x"}`)); err == nil {
		t.Fatal("expected invalid event to be rejected")
	}
}

// TestDeleteHookGuard verifies only agent-created hooks can be deleted.
func TestDeleteHookGuard(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userHook, _ := d.CreateHook(ctx, db.Hook{Event: db.HookPreToolUse, Command: "x"}) // CreatedBy ""
	del := NewDeleteHookTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userHook.ID+`"}`)); err == nil || !strings.Contains(err.Error(), "created by the user") {
		t.Fatalf("expected user-hook delete rejected, got %v", err)
	}

	agentHook, _ := d.CreateHook(ctx, db.Hook{Event: db.HookPreToolUse, Command: "y", CreatedBy: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentHook.ID+`"}`)); err != nil {
		t.Fatalf("agent-hook delete should succeed: %v", err)
	}
	if _, err := d.GetHook(ctx, agentHook.ID); err == nil {
		t.Fatal("agent hook should be gone")
	}
}

// TestCreateMCPServerValidation verifies transport-specific required fields and
// provenance stamping.
func TestCreateMCPServerValidation(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"
	create := NewCreateMCPServerTool(d, actor)

	// stdio without command is rejected.
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"x","transport":"stdio"}`)); err == nil {
		t.Fatal("expected stdio without command to be rejected")
	}
	// http without url is rejected.
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"x","transport":"http"}`)); err == nil {
		t.Fatal("expected http without url to be rejected")
	}
	// valid stdio stamps CreatedBy.
	out, err := create.Call(ctx, json.RawMessage(`{"name":"weather","transport":"stdio","command":"weather-mcp"}`))
	if err != nil {
		t.Fatalf("create_mcp_server: %v", err)
	}
	var res struct{ ID string }
	_ = json.Unmarshal([]byte(out), &res)
	got, _ := d.GetMCPServer(ctx, res.ID)
	if got.CreatedBy != actor || !got.Enabled {
		t.Fatalf("mcp server wrong: %+v", got)
	}
}

// TestDeleteMCPServerGuard verifies only agent-created MCP servers can be deleted.
func TestDeleteMCPServerGuard(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userSrv, _ := d.CreateMCPServer(ctx, db.MCPServer{Name: "U", Transport: "stdio", Command: "c"}) // CreatedBy ""
	del := NewDeleteMCPServerTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userSrv.ID+`"}`)); err == nil || !strings.Contains(err.Error(), "configured by the user") {
		t.Fatalf("expected user-server delete rejected, got %v", err)
	}

	agentSrv, _ := d.CreateMCPServer(ctx, db.MCPServer{Name: "A", Transport: "stdio", Command: "c", CreatedBy: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentSrv.ID+`"}`)); err != nil {
		t.Fatalf("agent-server delete should succeed: %v", err)
	}
	if _, err := d.GetMCPServer(ctx, agentSrv.ID); err == nil {
		t.Fatal("agent server should be gone")
	}
}
