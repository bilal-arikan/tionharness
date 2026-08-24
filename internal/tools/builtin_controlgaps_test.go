package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
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
	if _, err := create.Call(ctx, json.RawMessage(`{"event":"SessionStart","command":"echo lifecycle"}`)); err != nil {
		t.Fatalf("lifecycle event should be accepted: %v", err)
	}
}

func TestUpdateHookMatchesRESTFields(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	h, _ := d.CreateHook(ctx, db.Hook{Event: db.HookPreToolUse, Command: "old", Enabled: true})
	update := NewUpdateHookTool(d, "actor")
	_, err := update.Call(ctx, json.RawMessage(`{"id":"`+h.ID+`","event":"SessionEnd","matcher":"cleanup","command":"new","timeoutSec":45,"enabled":false}`))
	if err != nil {
		t.Fatalf("update_hook: %v", err)
	}
	got, _ := d.GetHook(ctx, h.ID)
	if got.Event != db.HookSessionEnd || got.Matcher != "cleanup" || got.Command != "new" || got.TimeoutSec != 45 || got.Enabled {
		t.Fatalf("updated hook wrong: %+v", got)
	}
}

// TestDeleteHook verifies any hook — user- or agent-created — can be deleted.
func TestDeleteHook(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userHook, _ := d.CreateHook(ctx, db.Hook{Event: db.HookPreToolUse, Command: "x"}) // CreatedBy ""
	del := NewDeleteHookTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userHook.ID+`"}`)); err != nil {
		t.Fatalf("delete of user-created hook should succeed: %v", err)
	}
	if _, err := d.GetHook(ctx, userHook.ID); err == nil {
		t.Fatal("user hook should be gone")
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
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"legacy","transport":"sse","url":"https://example.test"}`)); err == nil {
		t.Fatal("expected deprecated sse transport to be rejected")
	}

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
	if got.Args != "[]" || got.EnvConfig != "{}" {
		t.Fatalf("empty args/env not normalized: args=%q env=%q", got.Args, got.EnvConfig)
	}
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"bad-array","transport":"stdio","command":"x","args":"{}"}`)); err == nil {
		t.Fatal("expected non-array args to be rejected")
	}
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"bad-object","transport":"stdio","command":"x","env":"[]"}`)); err == nil {
		t.Fatal("expected non-object env to be rejected")
	}
}

// TestDeleteMCPServer verifies any MCP server — user- or agent-created — can be deleted.
func TestDeleteMCPServer(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userSrv, _ := d.CreateMCPServer(ctx, db.MCPServer{Name: "U", Transport: "stdio", Command: "c"}) // CreatedBy ""
	del := NewDeleteMCPServerTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userSrv.ID+`"}`)); err != nil {
		t.Fatalf("delete of user-created MCP server should succeed: %v", err)
	}
	if _, err := d.GetMCPServer(ctx, userSrv.ID); err == nil {
		t.Fatal("user server should be gone")
	}

	agentSrv, _ := d.CreateMCPServer(ctx, db.MCPServer{Name: "A", Transport: "stdio", Command: "c", CreatedBy: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentSrv.ID+`"}`)); err != nil {
		t.Fatalf("agent-server delete should succeed: %v", err)
	}
	if _, err := d.GetMCPServer(ctx, agentSrv.ID); err == nil {
		t.Fatal("agent server should be gone")
	}
}
