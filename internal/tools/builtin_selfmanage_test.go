package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/memory"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return d
}

// TestCreateAgentStampsCreatedBy verifies create_agent tags the new agent with
// the acting agent's id (provenance) and that delete enforces it.
func TestCreateAgentStampsCreatedBy(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	create := NewCreateAgentTool(d, actor, nil, nil)
	out, err := create.Call(ctx, json.RawMessage(`{"name":"Helper","soul":"helpful"}`))
	if err != nil {
		t.Fatalf("create_agent: %v", err)
	}
	var res struct{ ID string }
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	got, err := d.GetAgent(ctx, res.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.CreatedBy != actor {
		t.Fatalf("CreatedBy = %q, want %q", got.CreatedBy, actor)
	}
}

// TestCreateAgentSeedsSkills verifies create_agent seeds the default skill set
// when none is given, and validates caller-supplied slugs (dropping unknowns).
func TestCreateAgentSeedsSkills(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	known := map[string]bool{"tionswarm-guide": true, "custom": true}
	exists := func(s string) bool { return known[s] }
	create := NewCreateAgentTool(d, "actor", []string{"tionswarm-guide"}, exists)

	// No skills → defaults seeded + persisted.
	out, err := create.Call(ctx, json.RawMessage(`{"name":"A"}`))
	if err != nil {
		t.Fatalf("create_agent: %v", err)
	}
	var r1 struct {
		ID     string   `json:"id"`
		Skills []string `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &r1); err != nil {
		t.Fatal(err)
	}
	if len(r1.Skills) != 1 || r1.Skills[0] != "tionswarm-guide" {
		t.Fatalf("default skills = %v", r1.Skills)
	}
	got, _ := d.GetAgent(ctx, r1.ID)
	if len(got.Skills) != 1 || got.Skills[0] != "tionswarm-guide" {
		t.Fatalf("persisted skills = %v", got.Skills)
	}

	// Explicit skills with an unknown one → unknown skipped, known kept.
	out, err = create.Call(ctx, json.RawMessage(`{"name":"B","skills":["custom","nope"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var r2 struct {
		Skills  []string `json:"skills"`
		Skipped []string `json:"skippedUnknownSkills"`
	}
	if err := json.Unmarshal([]byte(out), &r2); err != nil {
		t.Fatal(err)
	}
	if len(r2.Skills) != 1 || r2.Skills[0] != "custom" {
		t.Fatalf("explicit skills = %v", r2.Skills)
	}
	if len(r2.Skipped) != 1 || r2.Skipped[0] != "nope" {
		t.Fatalf("skipped unknown = %v", r2.Skipped)
	}
}

// TestRunScheduleTool verifies run_schedule fires the scheduler's RunNow for an
// existing schedule and errors on an unknown id.
func TestRunScheduleTool(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	sc, err := d.CreateSchedule(ctx, db.Schedule{AgentID: "a", CronExpr: "* * * * *", Prompt: "hi"})
	if err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	var fired string
	tool := NewRunScheduleTool(d, func(_ context.Context, id string) error { fired = id; return nil })

	out, err := tool.Call(ctx, json.RawMessage(`{"id":"`+sc.ID+`"}`))
	if err != nil {
		t.Fatalf("run_schedule: %v", err)
	}
	if fired != sc.ID {
		t.Fatalf("fired %q, want %q", fired, sc.ID)
	}
	if !strings.Contains(out, "fired") {
		t.Fatalf("output = %q", out)
	}
	if _, err := tool.Call(ctx, json.RawMessage(`{"id":"ghost"}`)); err == nil {
		t.Fatal("expected error for unknown schedule id")
	}
}

// TestDeleteAgentRejectsUserCreated verifies an agent cannot delete a
// user-created agent (CreatedBy == "").
func TestDeleteAgentRejectsUserCreated(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	userAgent, err := d.CreateAgent(ctx, db.Agent{Name: "UserMade"}) // CreatedBy == ""
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	del := NewDeleteAgentTool(d, "actor-1", nil)
	_, err = del.Call(ctx, json.RawMessage(`{"id":"`+userAgent.ID+`"}`))
	if err == nil {
		t.Fatal("expected delete of user-created agent to be rejected")
	}
	if !strings.Contains(err.Error(), "created by the user") {
		t.Fatalf("unexpected error: %v", err)
	}
	// It must still exist.
	if _, err := d.GetAgent(ctx, userAgent.ID); err != nil {
		t.Fatalf("agent should still exist: %v", err)
	}
}

// TestDeleteAgentRejectsSelf verifies an agent cannot delete itself.
func TestDeleteAgentRejectsSelf(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	self, err := d.CreateAgent(ctx, db.Agent{Name: "Me", CreatedBy: "someone"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	del := NewDeleteAgentTool(d, self.ID, nil)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+self.ID+`"}`)); err == nil {
		t.Fatal("expected self-delete to be rejected")
	}
}

// TestScheduleCreateAndGuard verifies create_schedule stamps provenance, reload
// is invoked, and delete enforces provenance.
func TestScheduleCreateAndGuard(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"
	ag, _ := d.CreateAgent(ctx, db.Agent{Name: "A", CreatedBy: actor})

	reloaded := 0
	reload := func(context.Context) error { reloaded++; return nil }

	create := NewCreateScheduleTool(d, actor, reload)
	out, err := create.Call(ctx, json.RawMessage(`{"agentId":"`+ag.ID+`","cronExpr":"0 9 * * *","prompt":"hi"}`))
	if err != nil {
		t.Fatalf("create_schedule: %v", err)
	}
	if reloaded != 1 {
		t.Fatalf("reload called %d times, want 1", reloaded)
	}
	var res struct{ ID string }
	_ = json.Unmarshal([]byte(out), &res)
	sc, err := d.GetSchedule(ctx, res.ID)
	if err != nil {
		t.Fatalf("get schedule: %v", err)
	}
	if sc.CreatedBy != actor {
		t.Fatalf("CreatedBy = %q, want %q", sc.CreatedBy, actor)
	}

	// A user-created schedule must be undeletable by an agent.
	userSc, _ := d.CreateSchedule(ctx, db.Schedule{AgentID: ag.ID, CronExpr: "0 0 * * *", Prompt: "x"})
	del := NewDeleteScheduleTool(d, actor, reload)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userSc.ID+`"}`)); err == nil {
		t.Fatal("expected delete of user-created schedule to be rejected")
	}
}

// TestFlowCreateValidatesGraph verifies create_flow rejects invalid graph JSON,
// rejects structurally-invalid (but well-formed JSON) graphs via deep
// validation, and stamps provenance on a valid graph.
func TestFlowCreateValidatesGraph(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"
	create := NewCreateFlowTool(d, actor)

	if _, err := create.Call(ctx, json.RawMessage(`{"name":"Bad","graph":"{not json"}`)); err == nil {
		t.Fatal("expected invalid graph JSON to be rejected")
	}
	// Well-formed JSON but no start node → deep validation (ParseGraph+Validate)
	// must reject it at create time, not only at run time.
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"Empty","graph":"{\"nodes\":[]}"}`)); err == nil {
		t.Fatal("expected structurally-invalid graph (no start node) to be rejected")
	}
	const validGraph = `{\"start\":\"n1\",\"nodes\":[{\"id\":\"n1\",\"type\":\"agent\",\"agentId\":\"a1\"}]}`
	out, err := create.Call(ctx, json.RawMessage(`{"name":"Good","graph":"`+validGraph+`"}`))
	if err != nil {
		t.Fatalf("create_flow: %v", err)
	}
	var res struct{ ID string }
	_ = json.Unmarshal([]byte(out), &res)
	f, err := d.GetFlow(ctx, res.ID)
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if f.CreatedBy != actor {
		t.Fatalf("CreatedBy = %q, want %q", f.CreatedBy, actor)
	}
}

// TestMemoryAdd verifies memory_add stores under the acting agent's id.
func TestMemoryAdd(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	mem := memory.New(d)
	const actor = "actor-1"

	add := NewMemoryAddTool(mem, actor)
	if _, err := add.Call(ctx, json.RawMessage(`{"content":"remember this","kind":"document"}`)); err != nil {
		t.Fatalf("memory_add: %v", err)
	}
	list, err := mem.List(ctx, actor)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Content != "remember this" {
		t.Fatalf("memory not stored correctly: %+v", list)
	}
}

// TestDeleteArtifactGuard verifies only agent-created artifacts can be deleted.
func TestDeleteArtifactGuard(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userArt, _ := d.CreateArtifact(ctx, db.Artifact{Title: "User", Kind: "text", Content: "x"}) // AgentID == ""
	del := NewDeleteArtifactTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userArt.ID+`"}`)); err == nil {
		t.Fatal("expected delete of user-created artifact to be rejected")
	}

	agentArt, _ := d.CreateArtifact(ctx, db.Artifact{Title: "Agent", Kind: "text", Content: "y", AgentID: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentArt.ID+`"}`)); err != nil {
		t.Fatalf("delete of agent-created artifact should succeed: %v", err)
	}
	if _, err := d.GetArtifact(ctx, agentArt.ID); err == nil {
		t.Fatal("artifact should be gone")
	}
}
