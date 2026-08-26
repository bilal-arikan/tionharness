package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
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

	create := NewCreateAgentTool(d, actor, nil, nil, nil)
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
	known := map[string]bool{"tionharness-guide": true, "custom": true}
	exists := func(s string) bool { return known[s] }
	create := NewCreateAgentTool(d, "actor", []string{"tionharness-guide"}, exists, nil)

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
	if len(r1.Skills) != 1 || r1.Skills[0] != "tionharness-guide" {
		t.Fatalf("default skills = %v", r1.Skills)
	}
	got, _ := d.GetAgent(ctx, r1.ID)
	if len(got.Skills) != 1 || got.Skills[0] != "tionharness-guide" {
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

// TestCreateAgentInheritsCreatorProviderModel verifies that when the caller omits
// provider (and model), the new agent inherits both from the creating agent as a
// matched pair; and that an explicit provider suppresses inheritance so an
// inherited model can never be paired with a mismatched provider.
func TestCreateAgentInheritsCreatorProviderModel(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	creator, err := d.CreateAgent(ctx, db.Agent{Name: "Coordinator", Provider: "deepseek-anthropic", Model: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("seed creator: %v", err)
	}
	create := NewCreateAgentTool(d, creator.ID, nil, nil, nil)

	// Neither provider nor model given → inherit both from the creator.
	out, err := create.Call(ctx, json.RawMessage(`{"name":"Child"}`))
	if err != nil {
		t.Fatalf("create_agent: %v", err)
	}
	var r struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	child, _ := d.GetAgent(ctx, r.ID)
	if child.Provider != "deepseek-anthropic" || child.Model != "deepseek-v4-flash" {
		t.Fatalf("inherited provider/model = %q/%q, want deepseek-anthropic/deepseek-v4-flash", child.Provider, child.Model)
	}

	// Explicit provider given → no inheritance; model stays whatever the caller
	// set (here empty, the provider's own default), never the creator's model.
	out, err = create.Call(ctx, json.RawMessage(`{"name":"Anthropic child","provider":"anthropic"}`))
	if err != nil {
		t.Fatalf("create_agent: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	child2, _ := d.GetAgent(ctx, r.ID)
	if child2.Provider != "anthropic" || child2.Model != "" {
		t.Fatalf("explicit provider = %q/%q, want anthropic/<empty>", child2.Provider, child2.Model)
	}

	// Unknown/empty actor (no creator to inherit from) → claude-cli fallback.
	orphan := NewCreateAgentTool(d, "", nil, nil, nil)
	out, err = orphan.Call(ctx, json.RawMessage(`{"name":"Orphan"}`))
	if err != nil {
		t.Fatalf("create_agent: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	child3, _ := d.GetAgent(ctx, r.ID)
	if child3.Provider != "claude-cli" {
		t.Fatalf("orphan provider = %q, want claude-cli", child3.Provider)
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

// TestDeleteAgentAllowsUserCreated verifies the provenance gate is gone: an
// agent can delete a user-created agent (CreatedBy == "").
func TestDeleteAgentAllowsUserCreated(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	userAgent, err := d.CreateAgent(ctx, db.Agent{Name: "UserMade"}) // CreatedBy == ""
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	del := NewDeleteAgentTool(d, "actor-1", nil, nil)
	if _, err = del.Call(ctx, json.RawMessage(`{"id":"`+userAgent.ID+`"}`)); err != nil {
		t.Fatalf("delete of user-created agent should succeed: %v", err)
	}
	// DeleteAgent is a soft delete (the row survives for history), so verify it is
	// now marked deleted rather than absent.
	got, err := d.GetAgent(ctx, userAgent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if !got.Deleted {
		t.Fatal("agent should be marked deleted")
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
	del := NewDeleteAgentTool(d, self.ID, nil, nil)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+self.ID+`"}`)); err == nil {
		t.Fatal("expected self-delete to be rejected")
	}
}

// TestScheduleCreateAndDelete verifies create_schedule stamps provenance, reload
// is invoked, and delete works regardless of provenance (no gate).
func TestScheduleCreateAndDelete(t *testing.T) {
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
	if sc.Enabled {
		t.Fatal("omitted enabled should default false to match REST")
	}
	out, err = create.Call(ctx, json.RawMessage(`{"agentId":"`+ag.ID+`","cronExpr":"0 10 * * *","prompt":"later","enabled":true,"expiresAt":2000000000}`))
	if err != nil {
		t.Fatalf("create expiring schedule: %v", err)
	}
	_ = json.Unmarshal([]byte(out), &res)
	expiring, _ := d.GetSchedule(ctx, res.ID)
	if !expiring.Enabled || expiring.ExpiresAt != 2000000000 {
		t.Fatalf("expiring schedule wrong: %+v", expiring)
	}
	update := NewUpdateScheduleTool(d, actor, reload)
	if _, err := update.Call(ctx, json.RawMessage(`{"id":"`+expiring.ID+`","expiresAt":0}`)); err != nil {
		t.Fatalf("clear schedule expiry: %v", err)
	}
	expiring, _ = d.GetSchedule(ctx, expiring.ID)
	if expiring.ExpiresAt != 0 {
		t.Fatalf("schedule expiry not cleared: %+v", expiring)
	}

	// A user-created schedule is now deletable by an agent (no provenance gate).
	userSc, _ := d.CreateSchedule(ctx, db.Schedule{AgentID: ag.ID, CronExpr: "0 0 * * *", Prompt: "x"})
	del := NewDeleteScheduleTool(d, actor, reload)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userSc.ID+`"}`)); err != nil {
		t.Fatalf("delete of user-created schedule should succeed: %v", err)
	}
	if _, err := d.GetSchedule(ctx, userSc.ID); err == nil {
		t.Fatal("schedule should be gone")
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
	// The agent node's agentId must resolve to a real agent — "a1" does not exist,
	// so this must be rejected at create time too (TSK254), not only once run.
	if _, err := create.Call(ctx, json.RawMessage(`{"name":"BadAgent","graph":"{\"start\":\"start\",\"nodes\":[{\"id\":\"start\",\"type\":\"start\",\"next\":\"n1\"},{\"id\":\"n1\",\"type\":\"agent\",\"agentId\":\"a1\"}]}"}`)); err == nil {
		t.Fatal("expected an agentId that does not exist to be rejected")
	}
	ag, err := d.CreateAgent(ctx, db.Agent{Name: "worker"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	validGraph := `{\"start\":\"start\",\"nodes\":[{\"id\":\"start\",\"type\":\"start\",\"next\":\"n1\"},{\"id\":\"n1\",\"type\":\"agent\",\"agentId\":\"` + ag.ID + `\"}]}`
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

// TestDeleteArtifact verifies any artifact — user- or agent-created — can be deleted.
func TestDeleteArtifact(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	const actor = "actor-1"

	userArt, _ := d.CreateArtifact(ctx, db.Artifact{Title: "User", Kind: "text", Content: "x"}) // AgentID == ""
	del := NewDeleteArtifactTool(d, actor)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+userArt.ID+`"}`)); err != nil {
		t.Fatalf("delete of user-created artifact should succeed: %v", err)
	}
	if _, err := d.GetArtifact(ctx, userArt.ID); err == nil {
		t.Fatal("user artifact should be gone")
	}

	agentArt, _ := d.CreateArtifact(ctx, db.Artifact{Title: "Agent", Kind: "text", Content: "y", AgentID: actor})
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+agentArt.ID+`"}`)); err != nil {
		t.Fatalf("delete of agent-created artifact should succeed: %v", err)
	}
	if _, err := d.GetArtifact(ctx, agentArt.ID); err == nil {
		t.Fatal("agent artifact should be gone")
	}
}

// TestDeleteAgentRefusesBusyTarget: the tool must honour the SAME in-flight guard
// the HTTP endpoint enforces. Without it an agent could do through a tool exactly
// what the user is refused in the UI — delete a peer mid-turn.
func TestDeleteAgentRefusesBusyTarget(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	target, err := d.CreateAgent(ctx, db.Agent{Name: "Worker", CreatedBy: "actor-1"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	busy := func(context.Context, string) (bool, string) { return true, "SES9" }
	del := NewDeleteAgentTool(d, "actor-1", nil, busy)
	_, err = del.Call(ctx, json.RawMessage(`{"id":"`+target.ID+`"}`))
	if err == nil {
		t.Fatal("expected delete of a busy agent to be rejected")
	}
	if !strings.Contains(err.Error(), "busy") || !strings.Contains(err.Error(), "SES9") {
		t.Fatalf("error must name what is busy, got: %v", err)
	}
	// Untouched: not even soft-deleted.
	got, err := d.GetAgent(ctx, target.ID)
	if err != nil {
		t.Fatalf("agent should still exist: %v", err)
	}
	if got.Deleted {
		t.Fatal("busy agent was marked deleted despite the guard")
	}
}

// TestDeleteAgentAllowsIdleTarget is the other half: the guard must not be a
// blanket refusal, or an agent-created agent could never be cleaned up.
func TestDeleteAgentAllowsIdleTarget(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	target, err := d.CreateAgent(ctx, db.Agent{Name: "Worker", CreatedBy: "actor-1"})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	idle := func(context.Context, string) (bool, string) { return false, "" }
	del := NewDeleteAgentTool(d, "actor-1", nil, idle)
	if _, err := del.Call(ctx, json.RawMessage(`{"id":"`+target.ID+`"}`)); err != nil {
		t.Fatalf("idle agent must be deletable: %v", err)
	}
	got, err := d.GetAgent(ctx, target.ID)
	if err != nil {
		t.Fatalf("soft delete must keep the row: %v", err)
	}
	if !got.Deleted {
		t.Fatal("agent was not marked deleted")
	}
}
