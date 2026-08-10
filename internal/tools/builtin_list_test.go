package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// builtin_list_test.go — TSK68: every list_* tool honors the shared
// limit/offset/sort contract plus its own filters, and every reply is the
// standard {items,total,offset,limit,hasMore} envelope.

type listEnv struct {
	Items   json.RawMessage `json:"items"`
	Total   int             `json:"total"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	HasMore bool            `json:"hasMore"`
}

func parseListEnv(t *testing.T, out string) listEnv {
	t.Helper()
	var env listEnv
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("list tool output is not the standard envelope: %v\n%s", err, out)
	}
	return env
}

// ---- list_agents ----

func TestListAgentsPaginationAndFilters(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	for _, a := range []db.Agent{
		{Name: "Alpha", Provider: "anthropic", Model: "claude-3"},
		{Name: "Beta", Provider: "openrouter", Model: "gpt-4o"},
		{Name: "Gamma", Provider: "anthropic", Model: "claude-3.5"},
		{Name: "Delta", Provider: "openrouter", Model: "claude-3"},
	} {
		if _, err := d.CreateAgent(ctx, a); err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}
	tool := NewListAgentsTool(d, "actor-1")

	// Page 1 of 2 (limit 2) — default sort is updated_desc (all equal here, so
	// order is stable by creation sequence).
	out, err := tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	if err != nil {
		t.Fatalf("list_agents: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 4 || env.Limit != 2 || len(env.Items) == 0 || !env.HasMore {
		t.Fatalf("page 1 = total %d limit %d items %d hasMore %v; want 4/2/>0/true", env.Total, env.Limit, len(env.Items), env.HasMore)
	}

	// Page 2 picks up the rest and hasMore flips to false.
	out, _ = tool.Call(ctx, json.RawMessage(`{"limit":2,"offset":2}`))
	env = parseListEnv(t, out)
	if env.Total != 4 || env.Offset != 2 || env.HasMore {
		t.Fatalf("page 2 = %+v; want total 4 offset 2 hasMore false", env)
	}

	// provider filter (case-insensitive substring).
	out, _ = tool.Call(ctx, json.RawMessage(`{"provider":"ANTHROPIC"}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("provider=anthropic total = %d, want 2", env.Total)
	}

	// model filter.
	out, _ = tool.Call(ctx, json.RawMessage(`{"model":"gpt"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("model=gpt total = %d, want 1", env.Total)
	}

	// name_asc ordering.
	out, _ = tool.Call(ctx, json.RawMessage(`{"sort":"name_asc"}`))
	env = parseListEnv(t, out)
	var rows []struct{ Name string }
	if err := json.Unmarshal(env.Items, &rows); err != nil {
		t.Fatalf("items: %v", err)
	}
	if rows[0].Name != "Alpha" {
		t.Fatalf("name_asc first = %q, want Alpha", rows[0].Name)
	}

	// state=disabled on a fresh store (no deleted agents) → empty, not an error.
	out, err = tool.Call(ctx, json.RawMessage(`{"state":"disabled"}`))
	if err != nil {
		t.Fatalf("state=disabled: %v", err)
	}
	env = parseListEnv(t, out)
	if env.Total != 0 {
		t.Fatalf("state=disabled total = %d, want 0", env.Total)
	}

	// Invalid state is an explicit error, never silently ignored.
	if _, err := tool.Call(ctx, json.RawMessage(`{"state":"bogus"}`)); err == nil {
		t.Fatal("state=bogus should error")
	}
	if _, err := tool.Call(ctx, json.RawMessage(`{"sort":"wat"}`)); err == nil {
		t.Fatal("sort=wat should error")
	}
}

// ---- list_tasks ----

func TestListTasksFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	mk := func(title, state, prio, owner string, tags []string) {
		t.Helper()
		if _, err := d.CreateTask(ctx, db.Task{
			Title: title, BoardState: state, Priority: prio, OwnerAgentID: owner, Tags: tags,
		}); err != nil {
			t.Fatalf("create task: %v", err)
		}
	}
	mk("T1", "todo", "high", "AG1", []string{"plan", "urgent"})
	mk("T2", "in_progress", "low", "AG2", []string{"plan"})
	mk("T3", "todo", "low", "AG1", []string{"research"})
	tool := NewListTasksTool(d, "actor-1")

	out, err := tool.Call(ctx, json.RawMessage(`{"boardState":"todo"}`))
	if err != nil {
		t.Fatalf("list_tasks: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("boardState=todo total = %d, want 2", env.Total)
	}

	out, _ = tool.Call(ctx, json.RawMessage(`{"boardState":"todo","priority":"low"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("todo+low total = %d, want 1", env.Total)
	}

	out, _ = tool.Call(ctx, json.RawMessage(`{"ownerAgentId":"AG2"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("owner=AG2 total = %d, want 1", env.Total)
	}

	// tags filter: AND semantics — "plan" alone matches two cards.
	out, _ = tool.Call(ctx, json.RawMessage(`{"tags":"plan"}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("tags=plan total = %d, want 2", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"tags":"plan, urgent"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("tags=plan,urgent total = %d, want 1", env.Total)
	}

	// Pagination with a filter — the total counts filtered rows only.
	out, _ = tool.Call(ctx, json.RawMessage(`{"limit":1,"offset":0}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || len(env.Items) == 0 || !env.HasMore {
		t.Fatalf("limit=1 = %+v; want total 3, 1 item, hasMore", env)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"limit":1,"offset":2}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || env.HasMore {
		t.Fatalf("offset=2 = %+v; want total 3, hasMore false", env)
	}
}

// ---- list_flows ----

func TestListFlowsTagsAndPagination(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	for _, f := range []db.Flow{
		{Name: "Onboard", Tags: []string{"core"}},
		{Name: "Report", Tags: []string{"core", "nightly"}},
		{Name: "Cleanup", Tags: []string{"ops"}},
	} {
		if _, err := d.CreateFlow(ctx, f); err != nil {
			t.Fatalf("create flow: %v", err)
		}
	}
	tool := NewListFlowsTool(d, "actor-1")

	out, err := tool.Call(ctx, json.RawMessage(`{"tags":"core"}`))
	if err != nil {
		t.Fatalf("list_flows: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("tags=core total = %d, want 2", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"tags":"core, nightly"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("tags=core,nightly total = %d, want 1", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || !env.HasMore {
		t.Fatalf("limit=2 = %+v; want total 3, hasMore", env)
	}
}

// ---- list_automations ----

func TestListAutomationsFiltersAndPagination(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	mk := func(name, kind string, enabled bool) {
		t.Helper()
		if _, err := d.CreateAutomation(ctx, db.Automation{
			Name: name, TriggerKind: kind, Enabled: enabled,
			PromptTemplate: "run", MaxIterations: 10, TargetAgentID: "AG1",
		}); err != nil {
			t.Fatalf("create automation: %v", err)
		}
	}
	mk("TagRule", db.TriggerTag, true)
	mk("BoardRule", db.TriggerBoard, false)
	mk("TokenRule", db.TriggerToken, true)
	tool := NewListAutomationsTool(d, "actor-1")

	out, err := tool.Call(ctx, json.RawMessage(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("list_automations: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("enabled=true total = %d, want 2", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"triggerKind":"board"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("triggerKind=board total = %d, want 1", env.Total)
	}
	if _, err := tool.Call(ctx, json.RawMessage(`{"triggerKind":"bogus"}`)); err == nil {
		t.Fatal("triggerKind=bogus should error")
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"targetAgentId":"AG1"}`))
	env = parseListEnv(t, out)
	if env.Total != 3 {
		t.Fatalf("targetAgentId=AG1 total = %d, want 3", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || !env.HasMore {
		t.Fatalf("limit=2 = %+v; want total 3, hasMore", env)
	}
}

// ---- list_schedules ----

func TestListSchedulesFiltersAndUpdatedAt(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	// Names are set deliberately in REVERSE creation order, so name_asc can only
	// pass by ordering on the real Name field — not by accidentally falling back
	// to id or to the store's arrival order.
	sc1, err := d.CreateSchedule(ctx, db.Schedule{Name: "zeta rapor", AgentID: "AG1", CronExpr: "0 0 * * *", Prompt: "a", Enabled: true})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	sc2, err := d.CreateSchedule(ctx, db.Schedule{Name: "alfa rapor", AgentID: "AG2", CronExpr: "0 6 * * *", Prompt: "b", Enabled: false})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	tool := NewListSchedulesTool(d, "actor-1")

	out, err := tool.Call(ctx, json.RawMessage(`{"enabled":true}`))
	if err != nil {
		t.Fatalf("list_schedules: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("enabled=true total = %d, want 1", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"agentId":"AG2"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("agentId=AG2 total = %d, want 1", env.Total)
	}

	// Toggling a schedule bumps UpdatedAt (unix seconds). Sleep past the second
	// boundary so the bump is strictly greater than the other row's stamp, then
	// updated_desc must put the toggled schedule first.
	time.Sleep(1100 * time.Millisecond)
	if err := d.SetScheduleEnabled(ctx, sc1.ID, false); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"sort":"updated_desc"}`))
	env = parseListEnv(t, out)
	var ids []struct{ ID string }
	if err := json.Unmarshal(env.Items, &ids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if ids[0].ID != sc1.ID {
		t.Fatalf("updated_desc first = %q, want %q (just toggled)", ids[0].ID, sc1.ID)
	}

	// Schedules carry a real Name now, so name_* orders by it: "alfa" before "zeta".
	out, _ = tool.Call(ctx, json.RawMessage(`{"sort":"name_asc"}`))
	env = parseListEnv(t, out)
	if err := json.Unmarshal(env.Items, &ids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if ids[0].ID != sc2.ID {
		t.Fatalf("name_asc first = %q, want %q (\"alfa rapor\")", ids[0].ID, sc2.ID)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"sort":"name_desc"}`))
	env = parseListEnv(t, out)
	if err := json.Unmarshal(env.Items, &ids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if ids[0].ID != sc1.ID {
		t.Fatalf("name_desc first = %q, want %q (\"zeta rapor\")", ids[0].ID, sc1.ID)
	}
}

// ---- list_workspaces ----

type mockWorkspaceBridge struct {
	list []WorkspaceInfo
}

func (m *mockWorkspaceBridge) ListWorkspaces() []WorkspaceInfo { return m.list }
func (m *mockWorkspaceBridge) CreateWorkspace(name, _, _ string) (WorkspaceInfo, error) {
	return WorkspaceInfo{ID: fmt.Sprintf("WS%d", len(m.list)+1), Name: name, CreatedAt: 1}, nil
}
func (m *mockWorkspaceBridge) RenameWorkspace(id, name string) (WorkspaceInfo, error) {
	return WorkspaceInfo{ID: id, Name: name}, nil
}
func (m *mockWorkspaceBridge) DeleteWorkspace(id string) error { return nil }

func TestListWorkspacesSortAndPagination(t *testing.T) {
	bridge := &mockWorkspaceBridge{list: []WorkspaceInfo{
		{ID: "WS1", Name: "Zeta", CreatedAt: 100},
		{ID: "WS2", Name: "Alpha", CreatedAt: 200},
		{ID: "WS3", Name: "Mid", CreatedAt: 300},
	}}
	tool := NewListWorkspacesTool(bridge, "actor-1", "WS2")

	out, err := tool.Call(context.Background(), json.RawMessage(`{"sort":"name_asc"}`))
	if err != nil {
		t.Fatalf("list_workspaces: %v", err)
	}
	env := parseListEnv(t, out)
	var ids []struct{ ID string }
	if err := json.Unmarshal(env.Items, &ids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if env.Total != 3 || ids[0].ID != "WS2" {
		t.Fatalf("name_asc = %v (total %d); want [WS2 ...] total 3", ids, env.Total)
	}

	out, _ = tool.Call(context.Background(), json.RawMessage(`{"limit":2}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || !env.HasMore {
		t.Fatalf("limit=2 = %+v; want total 3 hasMore", env)
	}
	out, _ = tool.Call(context.Background(), json.RawMessage(`{"limit":2,"offset":2}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || env.HasMore {
		t.Fatalf("offset=2 = %+v; want total 3 hasMore false", env)
	}

	// updated_* is a documented alias for created_* (no updated-at on workspaces).
	out, _ = tool.Call(context.Background(), json.RawMessage(`{"sort":"updated_asc"}`))
	env = parseListEnv(t, out)
	if err := json.Unmarshal(env.Items, &ids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if ids[0].ID != "WS1" {
		t.Fatalf("updated_asc first = %q, want WS1 (oldest)", ids[0].ID)
	}
}

// ---- list_workers ----

func TestListWorkersStateSortPagination(t *testing.T) {
	rows := []WorkerRow{
		{SessionID: "S1", AgentName: "zeta", Running: true, CreatedAt: 100, UpdatedAt: 100},
		{SessionID: "S2", AgentName: "alpha", Running: false, CreatedAt: 200, UpdatedAt: 200},
		{SessionID: "S3", AgentName: "mid", Running: true, Stuck: true, CreatedAt: 300, UpdatedAt: 300},
	}
	ctx := WithCoordination(context.Background(), &CoordinationFuncs{
		ListRows: func(_ context.Context, subtree bool) ([]WorkerRow, error) {
			if subtree {
				return rows, nil
			}
			return rows[:2], nil
		},
	})
	var tool ListWorkersTool

	out, err := tool.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_workers: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("default scope total = %d, want 2", env.Total)
	}

	out, _ = tool.Call(ctx, json.RawMessage(`{"scope":"subtree","state":"running"}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("running total = %d, want 2", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"scope":"subtree","state":"stuck"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("stuck total = %d, want 1", env.Total)
	}
	out, _ = tool.Call(ctx, json.RawMessage(`{"scope":"subtree","state":"idle"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("idle total = %d, want 1", env.Total)
	}

	// name_asc on the full subtree.
	out, _ = tool.Call(ctx, json.RawMessage(`{"scope":"subtree","sort":"name_asc"}`))
	env = parseListEnv(t, out)
	var sids []struct {
		ID string `json:"sessionId"`
	}
	if err := json.Unmarshal(env.Items, &sids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if env.Total != 3 || sids[0].ID != "S2" {
		t.Fatalf("name_asc = %v (total %d); want [S2 ...] total 3", sids, env.Total)
	}

	// Filter + sort combined: state=running with name_asc must order the
	// FILTERED rows (mid before zeta), not the raw ones — regression for the
	// sort source bug where the comparator indexed the unfiltered slice.
	out, _ = tool.Call(ctx, json.RawMessage(`{"scope":"subtree","state":"running","sort":"name_asc"}`))
	env = parseListEnv(t, out)
	if err := json.Unmarshal(env.Items, &sids); err != nil {
		t.Fatalf("items: %v", err)
	}
	if env.Total != 2 || sids[0].ID != "S3" {
		t.Fatalf("running+name_asc = %v (total %d); want [S3 S1] total 2", sids, env.Total)
	}

	if _, err := tool.Call(ctx, json.RawMessage(`{"state":"bogus"}`)); err == nil {
		t.Fatal("state=bogus should error")
	}
}

func TestListWorkersRequiresCoordinator(t *testing.T) {
	var tool ListWorkersTool
	if _, err := tool.Call(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("list_workers outside a coordinator session should error")
	}
}

// ---- list_artifacts ----

func TestListArtifactsPaginationAndSort(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	for _, a := range []db.Artifact{
		{Title: "Zebra", Kind: "markdown", Origin: "agent"},
		{Title: "Alpha", Kind: "code", Origin: "tool"},
		{Title: "Mid", Kind: "markdown", Origin: "manual"},
	} {
		if _, err := d.CreateArtifact(ctx, a); err != nil {
			t.Fatalf("create artifact: %v", err)
		}
	}
	tool := NewListArtifactsTool(d, "actor-1")

	// Page 1 of 2 — default sort updated_desc.
	out, err := tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	if err != nil {
		t.Fatalf("list_artifacts: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 3 || env.Limit != 2 || len(env.Items) == 0 || !env.HasMore {
		t.Fatalf("page 1 = total %d limit %d items %d hasMore %v; want 3/2/>0/true", env.Total, env.Limit, len(env.Items), env.HasMore)
	}

	// Page 2 picks up the rest and hasMore flips to false.
	out, _ = tool.Call(ctx, json.RawMessage(`{"limit":2,"offset":2}`))
	env = parseListEnv(t, out)
	if env.Total != 3 || env.Offset != 2 || env.HasMore {
		t.Fatalf("page 2 = %+v; want total 3 offset 2 hasMore false", env)
	}

	// kind filter.
	out, _ = tool.Call(ctx, json.RawMessage(`{"kind":"markdown"}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("kind=markdown total = %d, want 2", env.Total)
	}

	// origin filter.
	out, _ = tool.Call(ctx, json.RawMessage(`{"origin":"tool"}`))
	env = parseListEnv(t, out)
	if env.Total != 1 {
		t.Fatalf("origin=tool total = %d, want 1", env.Total)
	}

	// name_asc ordering (by title).
	out, _ = tool.Call(ctx, json.RawMessage(`{"sort":"name_asc"}`))
	env = parseListEnv(t, out)
	var rows []struct{ Title string }
	if err := json.Unmarshal(env.Items, &rows); err != nil {
		t.Fatalf("items: %v", err)
	}
	if env.Total != 3 || rows[0].Title != "Alpha" {
		t.Fatalf("name_asc first = %q (total %d); want Alpha / 3", rows[0].Title, env.Total)
	}

	// An invalid sort key is an explicit error, never a silent fallback.
	if _, err := tool.Call(ctx, json.RawMessage(`{"sort":"bogus_desc"}`)); err == nil {
		t.Fatal("sort=bogus_desc should error")
	}
}

// ---- list_hooks ----

func TestListHooksPaginationAndSort(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	for _, h := range []db.Hook{
		{Event: "PreToolUse", Matcher: "Bash", Command: "echo pre"},
		{Event: "PostToolUse", Matcher: "*", Command: "echo post"},
		{Event: "PreToolUse", Matcher: "Read", Command: "echo read"},
	} {
		if _, err := d.CreateHook(ctx, h); err != nil {
			t.Fatalf("create hook: %v", err)
		}
	}
	tool := NewListHooksTool(d, "actor-1")

	out, err := tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	if err != nil {
		t.Fatalf("list_hooks: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 3 || env.Limit != 2 || !env.HasMore {
		t.Fatalf("page 1 = %+v; want total 3 limit 2 hasMore true", env)
	}

	// event filter.
	out, _ = tool.Call(ctx, json.RawMessage(`{"event":"PreToolUse"}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("event=PreToolUse total = %d, want 2", env.Total)
	}

	// created_asc works; updated_* maps to creation time and is accepted too.
	out, err = tool.Call(ctx, json.RawMessage(`{"sort":"created_asc","limit":10}`))
	if err != nil {
		t.Fatalf("created_asc: %v", err)
	}
	env = parseListEnv(t, out)
	if env.Total != 3 || env.Limit != 10 {
		t.Fatalf("created_asc = %+v; want total 3 limit 10", env)
	}

	// name_* is rejected with a clear error (hooks have no name field).
	if _, err := tool.Call(ctx, json.RawMessage(`{"sort":"name_asc"}`)); err == nil {
		t.Fatal("name_asc should error for hooks")
	}
}

// ---- list_mcp_servers ----

func TestListMCPServersPaginationAndSort(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	for _, m := range []db.MCPServer{
		{Name: "files", Transport: "stdio", Command: "npx mcp-files", Enabled: true},
		{Name: "web", Transport: "http", URL: "http://127.0.0.1:9999", Enabled: false},
		{Name: "search", Transport: "stdio", Command: "npx mcp-search", Enabled: true},
	} {
		if _, err := d.CreateMCPServer(ctx, m); err != nil {
			t.Fatalf("create mcp server: %v", err)
		}
	}
	tool := NewListMCPServersTool(d, "actor-1")

	out, err := tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	if err != nil {
		t.Fatalf("list_mcp_servers: %v", err)
	}
	env := parseListEnv(t, out)
	if env.Total != 3 || env.Limit != 2 || !env.HasMore {
		t.Fatalf("page 1 = %+v; want total 3 limit 2 hasMore true", env)
	}

	// transport filter (case-insensitive substring).
	out, _ = tool.Call(ctx, json.RawMessage(`{"transport":"STDIO"}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("transport=stdio total = %d, want 2", env.Total)
	}

	// enabled filter.
	out, _ = tool.Call(ctx, json.RawMessage(`{"enabled":true}`))
	env = parseListEnv(t, out)
	if env.Total != 2 {
		t.Fatalf("enabled=true total = %d, want 2", env.Total)
	}

	// name_asc ordering.
	out, _ = tool.Call(ctx, json.RawMessage(`{"sort":"name_asc"}`))
	env = parseListEnv(t, out)
	var rows []struct{ Name string }
	if err := json.Unmarshal(env.Items, &rows); err != nil {
		t.Fatalf("items: %v", err)
	}
	if env.Total != 3 || rows[0].Name != "files" {
		t.Fatalf("name_asc first = %q (total %d); want files / 3", rows[0].Name, env.Total)
	}
}
