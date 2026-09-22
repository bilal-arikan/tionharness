package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

func callSetArchived(t *testing.T, tool SetArchivedTool, kind, id string, archived bool) (string, error) {
	t.Helper()
	in, err := json.Marshal(map[string]any{"kind": kind, "id": id, "archived": archived})
	if err != nil {
		t.Fatal(err)
	}
	return tool.Call(context.Background(), in)
}

func mustSetArchived(t *testing.T, tool SetArchivedTool, kind, id string, archived bool) {
	t.Helper()
	out, err := callSetArchived(t, tool, kind, id, archived)
	if err != nil {
		t.Fatalf("set_archived %s %s=%v: %v", kind, id, archived, err)
	}
	var res struct {
		Kind     string
		ID       string
		Archived bool
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if res.Kind != kind || res.ID != id || res.Archived != archived {
		t.Fatalf("result = %+v, want kind=%s id=%s archived=%v", res, kind, id, archived)
	}
}

func TestSetArchivedAgent(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	a, err := d.CreateAgent(ctx, db.Agent{Name: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	tool := NewSetArchivedTool(d, "actor-1", nil)

	mustSetArchived(t, tool, "agent", a.ID, true)
	if got, _ := d.GetAgent(ctx, a.ID); !got.Archived {
		t.Fatal("agent not archived")
	}
	mustSetArchived(t, tool, "agent", a.ID, false)
	if got, _ := d.GetAgent(ctx, a.ID); got.Archived {
		t.Fatal("agent not restored")
	}
}

func TestSetArchivedRefusesSystemAgent(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	sys, err := d.CreateAgent(ctx, db.Agent{Name: "sys", System: true, SystemKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = callSetArchived(t, NewSetArchivedTool(d, "actor-1", nil), "agent", sys.ID, true)
	if err == nil || !strings.Contains(err.Error(), db.ErrSystemAgentArchive.Error()) {
		t.Fatalf("want system-agent refusal, got %v", err)
	}
	if got, _ := d.GetAgent(ctx, sys.ID); got.Archived {
		t.Fatal("system agent was archived")
	}
}

func TestSetArchivedArtifact(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	art, err := d.CreateArtifact(ctx, db.Artifact{Title: "doc", Kind: "markdown", Content: "x"})
	if err != nil {
		t.Fatal(err)
	}
	tool := NewSetArchivedTool(d, "actor-1", nil)

	mustSetArchived(t, tool, "artifact", art.ID, true)
	if got, _ := d.GetArtifact(ctx, art.ID); !got.Archived {
		t.Fatal("artifact not archived")
	}
	mustSetArchived(t, tool, "artifact", art.ID, false)
	if got, _ := d.GetArtifact(ctx, art.ID); got.Archived {
		t.Fatal("artifact not restored")
	}
}

func TestSetArchivedAutomation(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	agent, err := d.CreateAgent(ctx, db.Agent{Name: "target"})
	if err != nil {
		t.Fatal(err)
	}
	auto, err := d.CreateAutomation(ctx, db.Automation{Name: "rule", TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: agent.ID, PromptTemplate: "run", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	tool := NewSetArchivedTool(d, "actor-1", nil)

	mustSetArchived(t, tool, "automation", auto.ID, true)
	if got, _ := d.GetAutomation(ctx, auto.ID); !got.Archived {
		t.Fatal("automation not archived")
	}
	mustSetArchived(t, tool, "automation", auto.ID, false)
	if got, _ := d.GetAutomation(ctx, auto.ID); got.Archived {
		t.Fatal("automation not restored")
	}
}

// TestSetArchivedGoal verifies the goal mapping shared with REST: archive moves
// the status to archived, restore returns it to draft (never straight back to
// active), and the acting agent is recorded in the revision history.
func TestSetArchivedGoal(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)
	g, err := d.CreateGoal(ctx, db.Goal{Name: "goal", Status: db.GoalStatusActive}, db.GoalByUser, "")
	if err != nil {
		t.Fatal(err)
	}
	tool := NewSetArchivedTool(d, "actor-1", nil)

	mustSetArchived(t, tool, "goal", g.ID, true)
	got, _ := d.GetGoal(ctx, g.ID)
	if got.Status != db.GoalStatusArchived {
		t.Fatalf("status = %q, want archived", got.Status)
	}
	if last := got.History[len(got.History)-1]; last.By != "agent:actor-1" {
		t.Fatalf("revision by = %q, want agent:actor-1", last.By)
	}
	mustSetArchived(t, tool, "goal", g.ID, false)
	if got, _ := d.GetGoal(ctx, g.ID); got.Status != db.GoalStatusDraft {
		t.Fatalf("status = %q, want draft", got.Status)
	}
}

func TestSetArchivedSkill(t *testing.T) {
	d := openTestDB(t)
	store := skills.New(t.TempDir(), t.TempDir())
	if _, err := store.Create("arch-skill", skills.SkillInput{Name: "Arch", Description: "d", Body: "body"}); err != nil {
		t.Fatal(err)
	}
	tool := NewSetArchivedTool(d, "actor-1", func(slug string, archived bool) error {
		_, err := store.SetArchived(slug, archived)
		return err
	})

	mustSetArchived(t, tool, "skill", "arch-skill", true)
	if sk, _ := store.Get("arch-skill"); !sk.Archived {
		t.Fatal("skill not archived")
	}
	mustSetArchived(t, tool, "skill", "arch-skill", false)
	if sk, _ := store.Get("arch-skill"); sk.Archived {
		t.Fatal("skill not restored")
	}
}

func TestSetArchivedRejects(t *testing.T) {
	d := openTestDB(t)
	tool := NewSetArchivedTool(d, "actor-1", nil)
	cases := []struct {
		name, input, want string
	}{
		{"unknown kind", `{"kind":"flow","id":"x","archived":true}`, `invalid kind "flow"`},
		{"missing id", `{"kind":"agent","id":" ","archived":true}`, "id is required"},
		{"missing archived", `{"kind":"agent","id":"x"}`, "archived is required"},
		{"missing agent", `{"kind":"agent","id":"AGT404","archived":true}`, `no agent with id "AGT404"`},
		{"missing goal", `{"kind":"goal","id":"GOL404","archived":false}`, `no goal with id "GOL404"`},
		{"no skill store", `{"kind":"skill","id":"s","archived":true}`, "no skill store"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Call(context.Background(), json.RawMessage(tc.input))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}
