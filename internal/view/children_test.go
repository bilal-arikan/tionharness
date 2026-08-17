package view

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/skills"
)

// fakeStore is a hand-built Store for exercising the Projector's structural
// Children walk without a real db. Only the reads Children uses carry data; the
// rest satisfy the interface with zero values.
type fakeStore struct {
	agents      []db.Agent
	sessions    []db.Session
	runs        []db.FlowRun
	tasks       []db.Task
	mcp         []db.MCPServer
	artifacts   []db.Artifact
	automations []db.Automation
}

func (s *fakeStore) ListSessions(_ context.Context, agentID string) ([]db.Session, error) {
	if agentID == "" {
		return s.sessions, nil
	}
	var out []db.Session
	for _, x := range s.sessions {
		if x.AgentID == agentID {
			out = append(out, x)
		}
	}
	return out, nil
}
func (s *fakeStore) ListFlowRuns(_ context.Context, _ string) ([]db.FlowRun, error) {
	return s.runs, nil
}
func (s *fakeStore) ListAgents(_ context.Context) ([]db.Agent, error)         { return s.agents, nil }
func (s *fakeStore) ListActiveTasks(_ context.Context) ([]db.Task, error)     { return s.tasks, nil }
func (s *fakeStore) ListTasks(_ context.Context) ([]db.Task, error)           { return s.tasks, nil }
func (s *fakeStore) ListMCPServers(_ context.Context) ([]db.MCPServer, error) { return s.mcp, nil }

func (s *fakeStore) GetFlowRun(context.Context, string) (db.FlowRun, error) { return db.FlowRun{}, nil }
func (s *fakeStore) GetFlow(context.Context, string) (db.Flow, error)       { return db.Flow{}, nil }
func (s *fakeStore) GetSession(context.Context, string) (db.Session, error) {
	return db.Session{}, nil
}
func (s *fakeStore) GetSessionUsage(context.Context, string) (db.SessionUsage, error) {
	return db.SessionUsage{}, nil
}
func (s *fakeStore) ListMessagesTail(context.Context, string, int) ([]db.Message, int, error) {
	return nil, 0, nil
}
func (s *fakeStore) ListWaitingSessionAsks(context.Context) ([]db.SessionAsk, error) {
	return nil, nil
}
func (s *fakeStore) ListSchedules(context.Context) ([]db.Schedule, error) { return nil, nil }
func (s *fakeStore) GetSchedule(context.Context, string) (db.Schedule, error) {
	return db.Schedule{}, nil
}
func (s *fakeStore) WorkspaceTokensToday(context.Context) int64              { return 0 }
func (s *fakeStore) UsageForDay(context.Context, string) ([]db.Usage, error) { return nil, nil }
func (s *fakeStore) GetAgent(context.Context, string) (db.Agent, error)      { return db.Agent{}, nil }
func (s *fakeStore) GetUsageToday(context.Context, string) (db.Usage, error) { return db.Usage{}, nil }
func (s *fakeStore) GetWorkspaceToolConfig(context.Context) (db.WorkspaceToolConfig, error) {
	return db.WorkspaceToolConfig{}, nil
}
func (s *fakeStore) GetArtifact(context.Context, string) (db.Artifact, error) {
	return db.Artifact{}, nil
}
func (s *fakeStore) ListArtifacts(_ context.Context, _ string) ([]db.Artifact, error) {
	return s.artifacts, nil
}
func (s *fakeStore) GetAutomation(context.Context, string) (db.Automation, error) {
	return db.Automation{}, nil
}
func (s *fakeStore) ListAutomations(_ context.Context) ([]db.Automation, error) {
	return s.automations, nil
}

// childrenFixture wires a projector over a store with one of each drillable node.
func childrenFixture() *Projector {
	now := time.Now().Unix()
	return NewProjector(&fakeStore{
		agents: []db.Agent{{ID: "AG1", Name: "builder"}, {ID: "AG2", Name: "researcher"}},
		sessions: []db.Session{
			{ID: "COORD", AgentID: "AG1", UpdatedAt: now, CoordinatorMode: true},
			{ID: "W1", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "COORD"},
			{ID: "W2", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "COORD", StuckTurns: 3},
			{ID: "S3", AgentID: "AG2", UpdatedAt: now},
			{ID: "ARCH", AgentID: "AG2", UpdatedAt: now, State: "archived"},
		},
		runs: []db.FlowRun{
			{ID: "RUN1", Status: db.FlowRunning, UpdatedAt: now},
			{ID: "RUN2", Status: db.FlowFailure, UpdatedAt: now},
		},
		tasks: []db.Task{
			{ID: "T1", BoardState: db.BoardInProgress, UpdatedAt: now},
			{ID: "T2", BoardState: db.BoardInProgress, UpdatedAt: now},
			{ID: "T3", BoardState: db.BoardFailed, UpdatedAt: now},
		},
	})
}

func kindsOf(hs []Handle) map[Kind]int {
	m := map[Kind]int{}
	for _, h := range hs {
		m[h.Ref.Kind]++
	}
	return m
}

func TestChildrenWorkspaceIsElevenNodes(t *testing.T) {
	hs, err := childrenFixture().Children(context.Background(), Ref{Kind: KindSpace, ID: WorkspaceRefID}, LensHealth)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(hs) != 11 {
		t.Fatalf("workspace children = %d, want 11: %+v", len(hs), hs)
	}
	// The map's shape is fixed: seven group nodes plus board/logs/budget/tools.
	want := map[Kind]int{KindCategory: 7, KindBoard: 1, KindLogs: 1, KindBudget: 1, KindTools: 1}
	got := kindsOf(hs)
	for k, n := range want {
		if got[k] != n {
			t.Errorf("workspace child kind %q = %d, want %d", k, got[k], n)
		}
	}
}

func TestChildrenSessionsCategoryExcludesArchived(t *testing.T) {
	hs, err := childrenFixture().Children(context.Background(), Ref{Kind: KindCategory, ID: CategorySessions}, LensHealth)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	// 5 sessions minus the archived one.
	if len(hs) != 4 {
		t.Fatalf("sessions children = %d, want 4: %+v", len(hs), hs)
	}
	for _, h := range hs {
		if h.Ref.ID == "ARCH" {
			t.Error("archived session must not be a child")
		}
		if h.Ref.Kind != KindSession {
			t.Errorf("child kind %q, want session", h.Ref.Kind)
		}
	}
}

func TestChildrenSessionsErrorsLensKeepsOnlyStuck(t *testing.T) {
	hs, err := childrenFixture().Children(context.Background(), Ref{Kind: KindCategory, ID: CategorySessions}, LensErrors)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(hs) != 1 || hs[0].Ref.ID != "W2" {
		t.Fatalf("errors lens should keep only the stuck session W2: %+v", hs)
	}
}

func TestChildrenAgentsAndFlows(t *testing.T) {
	p := childrenFixture()
	agents, err := p.Children(context.Background(), Ref{Kind: KindCategory, ID: CategoryAgents}, LensHealth)
	if err != nil {
		t.Fatalf("agents: %v", err)
	}
	if len(agents) != 2 || kindsOf(agents)[KindAgent] != 2 {
		t.Errorf("agents category wrong: %+v", agents)
	}
	flows, err := p.Children(context.Background(), Ref{Kind: KindCategory, ID: CategoryFlows}, LensHealth)
	if err != nil {
		t.Fatalf("flows: %v", err)
	}
	if len(flows) != 2 || kindsOf(flows)[KindFlowRun] != 2 {
		t.Errorf("flows category wrong: %+v", flows)
	}
	// Errors lens narrows flows to the failed run.
	failed, err := p.Children(context.Background(), Ref{Kind: KindCategory, ID: CategoryFlows}, LensErrors)
	if err != nil {
		t.Fatalf("flows errors: %v", err)
	}
	if len(failed) != 1 || failed[0].Ref.ID != "RUN2" {
		t.Errorf("errors lens should keep only the failed run: %+v", failed)
	}
}

func TestChildrenBoardIsColumnsAndColumnIsCards(t *testing.T) {
	p := childrenFixture()
	cols, err := p.Children(context.Background(), Ref{Kind: KindBoard, ID: BoardRefID}, LensHealth)
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	// Two columns exist on the board (in_progress, failed); each is a category node.
	if len(cols) != 2 || kindsOf(cols)[KindCategory] != 2 {
		t.Fatalf("board columns wrong: %+v", cols)
	}
	cards, err := p.Children(context.Background(), Ref{Kind: KindCategory, ID: categoryColumnPrefix + db.BoardInProgress}, LensHealth)
	if err != nil {
		t.Fatalf("column: %v", err)
	}
	if len(cards) != 2 {
		t.Fatalf("in_progress column should hold 2 cards: %+v", cards)
	}
	for _, h := range cards {
		if h.Ref.Kind != KindBoard || h.Ref.Sub == "" {
			t.Errorf("card handle must be a board drill-down: %+v", h.Ref)
		}
	}
}

func TestChildrenAgentDrillsIntoItsSessions(t *testing.T) {
	hs, err := childrenFixture().Children(context.Background(), Ref{Kind: KindAgent, ID: "AG1"}, LensHealth)
	if err != nil {
		t.Fatalf("agent children: %v", err)
	}
	// AG1 owns COORD, W1, W2 (all non-archived).
	if len(hs) != 3 || kindsOf(hs)[KindSession] != 3 {
		t.Errorf("agent sessions wrong: %+v", hs)
	}
}

func TestChildrenSessionDrillsIntoWorkers(t *testing.T) {
	hs, err := childrenFixture().Children(context.Background(), Ref{Kind: KindSession, ID: "COORD"}, LensHealth)
	if err != nil {
		t.Fatalf("session children: %v", err)
	}
	if len(hs) != 2 {
		t.Fatalf("coordinator should have 2 workers: %+v", hs)
	}
	// A plain (non-coordinator) session has no workers.
	none, err := childrenFixture().Children(context.Background(), Ref{Kind: KindSession, ID: "S3"}, LensHealth)
	if err != nil {
		t.Fatalf("plain session children: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("a plain session must have no worker children: %+v", none)
	}
}

func TestChildrenLeavesAndErrors(t *testing.T) {
	p := childrenFixture()
	ctx := context.Background()

	// Budget/tools are leaves in the map — empty, not an error.
	for _, ref := range []Ref{{Kind: KindBudget, ID: BudgetRefID}, {Kind: KindTools, ID: ToolsRefID}} {
		hs, err := p.Children(ctx, ref, LensHealth)
		if err != nil {
			t.Errorf("%s children errored: %v", ref.Kind, err)
		}
		if len(hs) != 0 {
			t.Errorf("%s must be a leaf, got %+v", ref.Kind, hs)
		}
	}
	// A single board card (Sub set) is a leaf.
	if hs, err := p.Children(ctx, Ref{Kind: KindBoard, ID: BoardRefID, Sub: "T1"}, LensHealth); err != nil || len(hs) != 0 {
		t.Errorf("board card must be a leaf: hs=%+v err=%v", hs, err)
	}
	// An unknown category id and an unsupported kind are BOTH errors, never empty.
	if _, err := p.Children(ctx, Ref{Kind: KindCategory, ID: "galaxy"}, LensHealth); err == nil {
		t.Error("unknown category must be an error")
	}
	if _, err := p.Children(ctx, Ref{Kind: KindSchedule, ID: "SCH1"}, LensHealth); err != nil {
		// schedule is a defined leaf → empty, no error.
		t.Errorf("schedule leaf should not error: %v", err)
	}
	if _, err := p.Children(ctx, Ref{Kind: Kind("galaxy"), ID: "x"}, LensHealth); err == nil {
		t.Error("unsupported kind must be an error")
	}
}

func TestChildrenCapsAtTopN(t *testing.T) {
	now := time.Now().Unix()
	var sessions []db.Session
	for i := 0; i < categoryTopN+15; i++ {
		sessions = append(sessions, db.Session{ID: fmt.Sprintf("S%d", i), AgentID: "AG1", UpdatedAt: now})
	}
	p := NewProjector(&fakeStore{sessions: sessions})
	hs, err := p.Children(context.Background(), Ref{Kind: KindCategory, ID: CategorySessions}, LensHealth)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(hs) != categoryTopN {
		t.Errorf("children capped wrong: got %d, want %d", len(hs), categoryTopN)
	}
}

// fakeSkillsSource is a hand-built SkillsSource for exercising the skills
// category without a real catalog.
type fakeSkillsSource struct{ catalog []skills.Skill }

func (f fakeSkillsSource) List() []skills.Skill { return f.catalog }
func (f fakeSkillsSource) Get(slug string) (skills.Skill, bool) {
	for _, sk := range f.catalog {
		if sk.Slug == slug {
			return sk, true
		}
	}
	return skills.Skill{}, false
}

// fakeFindingsSource is a hand-built FindingsSource for exercising the insights
// category without the insight sidecar.
type fakeFindingsSource struct{ findings []InsightFinding }

func (f fakeFindingsSource) ListFindings() []InsightFinding { return f.findings }

func TestChildrenArtifactsAndAutomations(t *testing.T) {
	now := time.Now().Unix()
	p := NewProjector(&fakeStore{
		artifacts: []db.Artifact{
			{ID: "ART1", Title: "görev raporu", UpdatedAt: now},
			{ID: "ART2", Title: "şema", UpdatedAt: now},
		},
		automations: []db.Automation{
			{ID: "AUT1", Name: "todo→review", CreatedAt: now},
			{ID: "AUT2", Name: "failed kart", CreatedAt: now, LastError: "provider 429"},
		},
	})
	ctx := context.Background()

	arts, err := p.Children(ctx, Ref{Kind: KindCategory, ID: CategoryArtifacts}, LensHealth)
	if err != nil {
		t.Fatalf("artifacts: %v", err)
	}
	if len(arts) != 2 || kindsOf(arts)[KindArtifact] != 2 {
		t.Errorf("artifacts category wrong: %+v", arts)
	}

	auts, err := p.Children(ctx, Ref{Kind: KindCategory, ID: CategoryAutomations}, LensHealth)
	if err != nil {
		t.Fatalf("automations: %v", err)
	}
	if len(auts) != 2 || kindsOf(auts)[KindAutomation] != 2 {
		t.Errorf("automations category wrong: %+v", auts)
	}

	// Errors lens keeps only the rule whose last fire failed.
	failed, err := p.Children(ctx, Ref{Kind: KindCategory, ID: CategoryAutomations}, LensErrors)
	if err != nil {
		t.Fatalf("automations errors: %v", err)
	}
	if len(failed) != 1 || failed[0].Ref.ID != "AUT2" {
		t.Errorf("errors lens should keep only the failed automation: %+v", failed)
	}
}

func TestChildrenSkillsAndInsightsNeedSources(t *testing.T) {
	ctx := context.Background()
	p := NewProjector(&fakeStore{})
	p.WithSources(Sources{
		Skills: fakeSkillsSource{catalog: []skills.Skill{
			{Slug: "tionswarm-build", Name: "Build"},
			{Slug: "tionswarm-guide", Name: "Guide"},
		}},
		Findings: fakeFindingsSource{findings: []InsightFinding{
			{ID: "FND1", Title: "provider 429", Status: "new"},
			{ID: "FND2", Title: "eski ders", Status: "applied", Regressed: true},
			{ID: "FND3", Title: "triaj edildi", Status: "triaged"},
		}},
	})

	sk, err := p.Children(ctx, Ref{Kind: KindCategory, ID: CategorySkills}, LensHealth)
	if err != nil {
		t.Fatalf("skills: %v", err)
	}
	if len(sk) != 2 || kindsOf(sk)[KindSkill] != 2 {
		t.Errorf("skills category wrong: %+v", sk)
	}

	ins, err := p.Children(ctx, Ref{Kind: KindCategory, ID: CategoryInsights}, LensHealth)
	if err != nil {
		t.Fatalf("insights: %v", err)
	}
	if len(ins) != 3 || kindsOf(ins)[KindInsight] != 3 {
		t.Errorf("insights category wrong: %+v", ins)
	}

	// Errors lens keeps fresh + regressed findings (needing attention).
	needy, err := p.Children(ctx, Ref{Kind: KindCategory, ID: CategoryInsights}, LensErrors)
	if err != nil {
		t.Fatalf("insights errors: %v", err)
	}
	if len(needy) != 2 {
		t.Errorf("errors lens should keep FND1 + FND2 (regressed): %+v", needy)
	}

	// Without sources the categories are a wiring error, never an empty list —
	// an empty bucket would read like a genuinely empty workspace.
	bare := NewProjector(&fakeStore{})
	if _, err := bare.Children(ctx, Ref{Kind: KindCategory, ID: CategorySkills}, LensHealth); err == nil {
		t.Error("skills category without a source must error")
	}
	if _, err := bare.Children(ctx, Ref{Kind: KindCategory, ID: CategoryInsights}, LensHealth); err == nil {
		t.Error("insights category without a source must error")
	}
}

func TestChildrenNewKindsAreLeaves(t *testing.T) {
	ctx := context.Background()
	p := childrenFixture()

	hs, err := p.Children(ctx, Ref{Kind: KindLogs, ID: LogsRefID}, LensHealth)
	if err != nil {
		t.Fatalf("logs children: %v", err)
	}
	if len(hs) != 0 {
		t.Errorf("logs must be a leaf, got %+v", hs)
	}
	// A single artifact/automation/skill/insight is a leaf too.
	for _, ref := range []Ref{
		{Kind: KindArtifact, ID: "ART1"},
		{Kind: KindAutomation, ID: "AUT1"},
		{Kind: KindSkill, ID: "x"},
		{Kind: KindInsight, ID: "FND1"},
	} {
		hs, err := p.Children(ctx, ref, LensHealth)
		if err != nil {
			t.Errorf("%s children errored: %v", ref.Kind, err)
		}
		if len(hs) != 0 {
			t.Errorf("%s must be a leaf, got %+v", ref.Kind, hs)
		}
	}
}
