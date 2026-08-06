package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// dashboardTroubleFixture seeds one of every attention-worthy entity so the
// action queue, outcomes and cost blocks have something to report.
func dashboardTroubleFixture(t *testing.T) *db.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.CreateAgent(ctx, db.Agent{Name: "builder"}); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// A stuck session — the self-heal loop gave up. Danger action.
	if _, err := database.CreateSession(ctx, db.Session{AgentID: "AGT1", Title: "takıldı", StuckTurns: 2}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	// A failed card and a done card (for the outcome block's throughput).
	if _, err := database.CreateTask(ctx, db.Task{Title: "bozuk", BoardState: db.BoardFailed}); err != nil {
		t.Fatalf("create failed task: %v", err)
	}
	if _, err := database.CreateTask(ctx, db.Task{Title: "biten", BoardState: db.BoardDone}); err != nil {
		t.Fatalf("create done task: %v", err)
	}
	if _, err := database.CreateFlow(ctx, db.Flow{Name: "f", Graph: `{"start":"s","nodes":[{"id":"s","type":"start"}]}`}); err != nil {
		t.Fatalf("create flow: %v", err)
	}
	// A failed flow run — danger action AND the denominator of the success rate.
	if _, err := database.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1", Status: db.FlowFailure, Error: "boom"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	// A session waiting on a human answer — warn action.
	if _, err := database.CreateSessionAsk(ctx, db.SessionAsk{SessionID: "SES1"}); err != nil {
		t.Fatalf("create ask: %v", err)
	}
	return database
}

// TestDashboardExposesCostActionsDeltasOutcomes pins the four new blocks the
// CEO overview depends on and that the action queue reflects real trouble.
func TestDashboardExposesCostActionsDeltasOutcomes(t *testing.T) {
	database := dashboardTroubleFixture(t)

	rec := serveFlowRuns((&Server{}).handleDashboard, database, "/api/dashboard?days=14", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeView(t, rec.Body.Bytes())

	for _, key := range []string{"cost", "costByDay", "topAgentsCost", "actions", "deltas", "outcomes"} {
		if _, ok := got[key]; !ok {
			t.Errorf("dashboard missing %q block", key)
		}
	}

	// The action queue must surface every kind of trouble seeded above.
	actions, _ := got["actions"].([]any)
	if len(actions) == 0 {
		t.Fatalf("action queue empty, want stuck/failed/waiting items")
	}
	kinds := map[string]bool{}
	danger := 0
	for _, a := range actions {
		m, _ := a.(map[string]any)
		kinds[str(m["kind"])] = true
		if str(m["severity"]) == "danger" {
			danger++
		}
	}
	for _, want := range []string{"session", "run", "card"} {
		if !kinds[want] {
			t.Errorf("action queue missing a %q item; got kinds %v", want, kinds)
		}
	}
	if danger == 0 {
		t.Errorf("no danger-severity actions, want stuck session + failed run/card")
	}

	// Outcomes: one done card, one failed (zero success) run closed → rate 0.
	outcomes, _ := got["outcomes"].(map[string]any)
	if n, _ := outcomes["cardsDoneWindow"].(float64); n < 1 {
		t.Errorf("cardsDoneWindow = %v, want >= 1", outcomes["cardsDoneWindow"])
	}
	if rate, ok := outcomes["runSuccessRate"].(float64); !ok || rate != 0 {
		t.Errorf("runSuccessRate = %v, want 0 (one failed, zero success run)", outcomes["runSuccessRate"])
	}

	// Deltas expose the four headline series.
	deltas, _ := got["deltas"].(map[string]any)
	for _, key := range []string{"sessions", "runs", "tokens", "cost"} {
		if _, ok := deltas[key]; !ok {
			t.Errorf("deltas missing %q", key)
		}
	}
}

// str is a nil-safe string coercion for decoded JSON values.
func str(v any) string {
	s, _ := v.(string)
	return s
}
