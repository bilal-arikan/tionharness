package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func summaryFixture() db.Trajectory {
	return db.Trajectory{
		ID: "RTA1", RootSessionID: "SES1", TemplateRef: "plan-dev@3", Status: db.TrajStatusDone, CreatedAt: 1000,
		Nodes: []db.TrajectoryNode{
			{ID: "p:plan", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStateDone, StartMs: 1000_000, EndMs: 1100_000},
			{ID: "p:code", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStateDone, StartMs: 1100_000, EndMs: 1300_000},
			{ID: "p:ship", Kind: db.TrajNodePhase, Origin: db.TrajOriginDeclared, State: db.TrajStatePending},
			{ID: "s:SES1", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefID: "SES1", Lane: 0, State: db.TrajStateDone},
			{ID: "s:W1", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefID: "W1", PhaseID: "p:plan", Lane: 1, State: db.TrajStateDone, EndMs: 1100_000},
			{ID: "s:W2", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefID: "W2", PhaseID: "p:code", Lane: 2, State: db.TrajStateFailed, EndMs: 1300_000},
			{ID: "s:W3", Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved, RefID: "W3", Lane: 3, State: db.TrajStateDone, EndMs: 1400_000},
			{ID: "r:RUN1", Kind: db.TrajNodeFlowRun, Origin: db.TrajOriginObserved, RefID: "RUN1", PhaseID: "p:code", Lane: 4, State: db.TrajStateFailed},
			{ID: "a:docs@code", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared, RefID: "docs", PhaseID: "p:code", State: db.TrajStateDone},
			{ID: "a:update-docs", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginDeclared, RefID: "update-docs", State: db.TrajStateGhost},
			{ID: "a:AUT9@plan", Kind: db.TrajNodeAutomation, Origin: db.TrajOriginObserved, RefID: "AUT9", PhaseID: "p:plan", State: db.TrajStateDone},
			{ID: "g:ASK1", Kind: db.TrajNodeGate, Origin: db.TrajOriginObserved, RefID: "ASK1", State: db.TrajStateDone, StartMs: 1200_000, EndMs: 1260_000},
		},
	}
}

func TestSummarizeTrajectory(t *testing.T) {
	cost := func(id string) (int64, float64, bool) {
		switch id {
		case "SES1":
			return 1000, 0.10, true
		case "W2":
			return 500, 0.05, false
		default:
			return 100, 0.01, true
		}
	}
	s := summarizeTrajectory(summaryFixture(), cost, 2000)
	if s.DurationSec != 400 { // last EndMs 1400s − CreatedAt 1000s
		t.Fatalf("duration = %d", s.DurationSec)
	}
	if s.Tokens != 1000+100+500+100 || s.Priced {
		t.Fatalf("tokens/priced = %d/%v", s.Tokens, s.Priced)
	}
	if s.Sessions != 3 || s.FailedSess != 1 || s.FlowRuns != 1 || s.FailedRuns != 1 {
		t.Fatalf("counts = %+v", s)
	}
	if s.Phases != 3 || s.PhasesDone != 2 || len(s.GhostPhases) != 1 || s.GhostPhases[0] != "ship" {
		t.Fatalf("phases = %+v", s)
	}
	if s.Unannounced != 1 { // W3 has no phase; RUN1 is under code
		t.Fatalf("unannounced = %d", s.Unannounced)
	}
	if s.Watchers != 2 || len(s.UnfiredWatchers) != 1 || s.UnfiredWatchers[0] != "update-docs" {
		t.Fatalf("watchers = %+v", s)
	}
	if s.Gates != 1 || s.GateWaitSec != 60 {
		t.Fatalf("gates = %d wait %d", s.Gates, s.GateWaitSec)
	}
	if st := s.PerPhase["code"]; st.Sessions != 1 || st.Failed != 1 || st.DurationSec != 200 {
		t.Fatalf("code phase stat = %+v", st)
	}
	// A live run closes its duration at now.
	live := summaryFixture()
	live.Status = db.TrajStatusRunning
	if ls := summarizeTrajectory(live, nil, 3000); ls.DurationSec != 2000 || ls.Tokens != 0 || !ls.Priced {
		t.Fatalf("live summary = %+v", ls)
	}
}

func TestRecipeStatsFromIndex(t *testing.T) {
	sum := func(dur, tok int64, unfired, ghost []string) *db.TrajectorySummary {
		return &db.TrajectorySummary{DurationSec: dur, Tokens: tok, Priced: true, Sessions: 2, UnfiredWatchers: unfired, GhostPhases: ghost}
	}
	rows := []db.TrajectoryIndexEntry{
		{ID: "RTA1", TemplateRef: "plan-dev@3", Status: db.TrajStatusDone, UpdatedAt: 10, Summary: sum(100, 1000, []string{"docs"}, []string{"ship"})},
		{ID: "RTA2", TemplateRef: "plan-dev@3", Status: db.TrajStatusFailed, UpdatedAt: 20, Summary: sum(300, 3000, []string{"docs"}, nil)},
		{ID: "RTA3", TemplateRef: "plan-dev@3", Status: db.TrajStatusRunning, UpdatedAt: 30},
		{ID: "RTA4", TemplateRef: "plan-dev@2", Status: db.TrajStatusAbandoned, UpdatedAt: 5},
		{ID: "RTA5", TemplateRef: "", Status: db.TrajStatusDone, UpdatedAt: 7},
	}
	stats := RecipeStatsFromIndex(rows)
	if len(stats) != 3 || stats[0].TemplateRef != "" || stats[1].TemplateRef != "plan-dev@2" || stats[2].TemplateRef != "plan-dev@3" {
		t.Fatalf("order = %+v", stats)
	}
	st := stats[2]
	if st.Slug != "plan-dev" || st.Version != "3" || st.Runs != 2 || st.Done != 1 || st.Failed != 1 || st.Live != 1 || st.Summarized != 2 {
		t.Fatalf("plan-dev@3 = %+v", st)
	}
	if st.AvgDurationSec != 200 || st.AvgTokens != 2000 || st.AvgSessions != 2 || st.LatestID != "RTA3" {
		t.Fatalf("averages = %+v", st)
	}
	if st.UnfiredWatchers["docs"] != 2 || st.GhostPhases["ship"] != 1 {
		t.Fatalf("watcher/ghost counts = %+v", st)
	}
	if stats[1].Runs != 1 || stats[1].Abandoned != 1 || stats[1].Summarized != 0 || stats[1].UnfiredWatchers != nil {
		t.Fatalf("plan-dev@2 = %+v", stats[1])
	}
}
