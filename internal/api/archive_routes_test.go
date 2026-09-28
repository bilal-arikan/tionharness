package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// archiveCase exercises one entity through the shared archive pair: archive,
// the archived list filter, the default list, and unarchive.
type archiveCase struct {
	name      string
	base      string // entity path, e.g. "/api/agents/AGT1"
	list      string // list endpoint
	id        string
	idOf      func(item map[string]any) string
	archived  func(t *testing.T) bool
	defaultOn bool // true when the default list still includes archived items
}

func TestEntityArchiveRoutesRoundTrip(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	h := s.Routes()

	agent, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "archivable"})
	if err != nil {
		t.Fatal(err)
	}
	art, err := wsp.DB.CreateArtifact(ctx, db.Artifact{Title: "doc", Kind: "markdown", Content: "x"})
	if err != nil {
		t.Fatal(err)
	}
	auto, err := wsp.DB.CreateAutomation(ctx, db.Automation{Name: "rule", TriggerKind: db.TriggerTag, TriggerTag: "done", TargetAgentID: agent.ID, PromptTemplate: "run", Enabled: true, MaxIterations: 3})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := wsp.DB.CreateGoal(ctx, db.Goal{Name: "goal", Status: db.GoalStatusActive}, db.GoalByUser, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wsp.Runtime.Skills().Create("arch-skill", skills.SkillInput{Name: "Arch", Description: "d", Shared: true, Body: "body"}); err != nil {
		t.Fatal(err)
	}

	byID := func(item map[string]any) string { s, _ := item["id"].(string); return s }
	cases := []archiveCase{
		{name: "agent", base: "/api/agents/" + agent.ID, list: "/api/agents", id: agent.ID, idOf: byID, defaultOn: false,
			archived: func(t *testing.T) bool { a, _ := wsp.DB.GetAgent(ctx, agent.ID); return a.Archived }},
		{name: "artifact", base: "/api/artifacts/" + art.ID, list: "/api/artifacts", id: art.ID, idOf: byID, defaultOn: false,
			archived: func(t *testing.T) bool { a, _ := wsp.DB.GetArtifact(ctx, art.ID); return a.Archived }},
		{name: "automation", base: "/api/automations/" + auto.ID, list: "/api/automations", id: auto.ID, idOf: byID,
			archived: func(t *testing.T) bool { a, _ := wsp.DB.GetAutomation(ctx, auto.ID); return a.Archived }},
		{name: "goal", base: "/api/goals/" + goal.ID, list: "/api/goals", id: goal.ID, idOf: byID, defaultOn: false,
			archived: func(t *testing.T) bool {
				g, _ := wsp.DB.GetGoal(ctx, goal.ID)
				return g.Status == db.GoalStatusArchived
			}},
		{name: "skill", base: "/api/skills/arch-skill", list: "/api/skills", id: "arch-skill",
			idOf:      func(item map[string]any) string { s, _ := item["slug"].(string); return s },
			defaultOn: false,
			archived: func(t *testing.T) bool {
				sk, _ := wsp.Runtime.Skills().Get("arch-skill")
				return sk.Archived
			}},
	}

	listed := func(t *testing.T, c archiveCase, query string) bool {
		t.Helper()
		var items []map[string]any
		rec := doJSON(t, h, http.MethodGet, c.list+query, nil, &items)
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s%s: status %d: %s", c.list, query, rec.Code, rec.Body.String())
		}
		for _, it := range items {
			if c.idOf(it) == c.id {
				return true
			}
		}
		return false
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A bare POST archives.
			rec := doJSON(t, h, http.MethodPost, c.base+"/archive", nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("archive: status %d: %s", rec.Code, rec.Body.String())
			}
			if !c.archived(t) {
				t.Fatal("entity not archived after archive")
			}
			if !listed(t, c, "?archived=true") {
				t.Fatal("archived entity missing from ?archived=true")
			}
			if !listed(t, c, "?archived=all") {
				t.Fatal("archived entity missing from explicit full list")
			}
			if rec := doJSON(t, h, http.MethodGet, c.base, nil, nil); rec.Code != http.StatusOK {
				t.Fatalf("archived detail: status %d: %s", rec.Code, rec.Body.String())
			}
			if listed(t, c, "?archived=false") {
				t.Fatal("archived entity listed under ?archived=false")
			}
			if got := listed(t, c, ""); got != c.defaultOn {
				t.Fatalf("default list includes archived = %v, want %v", got, c.defaultOn)
			}

			rec = doJSON(t, h, http.MethodPost, c.base+"/unarchive", nil, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("unarchive: status %d: %s", rec.Code, rec.Body.String())
			}
			if c.archived(t) {
				t.Fatal("entity still archived after unarchive")
			}
			if listed(t, c, "?archived=true") {
				t.Fatal("restored entity still under ?archived=true")
			}

			// The body form is kept for existing callers: {"archived": false} restores.
			rec = doJSON(t, h, http.MethodPost, c.base+"/archive", map[string]any{"archived": true}, nil)
			if rec.Code != http.StatusOK || !c.archived(t) {
				t.Fatalf("archive with body: status %d, archived=%v", rec.Code, c.archived(t))
			}
			rec = doJSON(t, h, http.MethodPost, c.base+"/archive", map[string]any{"archived": false}, nil)
			if rec.Code != http.StatusOK || c.archived(t) {
				t.Fatalf("archive {archived:false}: status %d, archived=%v", rec.Code, c.archived(t))
			}
		})
	}
}

func TestEntityArchiveRoutesRejectMissingAndBadFilter(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()
	for _, path := range []string{
		"/api/agents/AGT404/archive",
		"/api/artifacts/ART404/archive",
		"/api/automations/AUT404/archive",
		"/api/goals/GOL404/archive",
		"/api/skills/no-such-skill/archive",
	} {
		rec := doJSON(t, h, http.MethodPost, path, nil, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404: %s", path, rec.Code, rec.Body.String())
		}
	}
	rec := doJSON(t, h, http.MethodGet, "/api/agents?archived=maybe", nil, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad archived filter: status %d, want 400", rec.Code)
	}
}

func TestGoalUnarchiveReturnsToDraft(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	goal, err := wsp.DB.CreateGoal(ctx, db.Goal{Name: "goal", Status: db.GoalStatusActive}, db.GoalByUser, "")
	if err != nil {
		t.Fatal(err)
	}
	h := s.Routes()
	doJSON(t, h, http.MethodPost, "/api/goals/"+goal.ID+"/archive", nil, nil)
	doJSON(t, h, http.MethodPost, "/api/goals/"+goal.ID+"/unarchive", nil, nil)
	g, _ := wsp.DB.GetGoal(ctx, goal.ID)
	if g.Status != db.GoalStatusDraft {
		t.Fatalf("status after unarchive = %q, want draft (never straight back to active)", g.Status)
	}
}

func TestSystemAgentArchiveIsConflict(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	a, err := wsp.DB.CreateAgent(context.Background(), db.Agent{Name: "sys", System: true, SystemKey: "titler-test"})
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPost, "/api/agents/"+a.ID+"/archive", nil, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSessionRefusesArchivedAgent(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	a, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "gone"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wsp.DB.SetAgentArchived(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Routes(), http.MethodPost, "/api/sessions", map[string]any{"agentId": a.ID}, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409: %s", rec.Code, rec.Body.String())
	}
}
