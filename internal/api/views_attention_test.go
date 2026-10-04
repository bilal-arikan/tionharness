package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/view"
)

type attentionPayload struct {
	Attention map[string]view.GraphAttention `json:"attention"`
	Status    *view.GraphStatus              `json:"status"`
}

// TestGraphAttentionMarksOpenLoops pins the map's attention layer to the
// awareness open-loop scan: a stuck session and a failed card get their rings,
// the status strip counts them, and the light /graph/live endpoint carries the
// same layer without the structural walk.
func TestGraphAttentionMarksOpenLoops(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	agent, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Tester"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	stuck, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat", Title: "stuck one", Tags: []string{"stuck"}})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	calm, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat", Title: "calm one"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	failed, err := wsp.DB.CreateTask(ctx, db.Task{Title: "broken build", BoardState: db.BoardFailed})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	h := s.Routes()
	get := func(path string) attentionPayload {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Workspace-Id", wsp.ID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d: %s", path, rec.Code, rec.Body.String())
		}
		var out attentionPayload
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		return out
	}

	for _, path := range []string{"/api/views/graph", "/api/views/graph/live"} {
		got := get(path)
		stuckKey := view.Ref{Kind: view.KindSession, ID: stuck.ID}.String()
		if a, ok := got.Attention[stuckKey]; !ok || a.Level != view.AttentionDanger || len(a.Reasons) != 1 || a.Reasons[0] != view.ReasonStuck {
			t.Errorf("%s: stuck session attention = %+v, want danger/stuck", path, got.Attention[stuckKey])
		}
		if _, ok := got.Attention[view.Ref{Kind: view.KindSession, ID: calm.ID}.String()]; ok {
			t.Errorf("%s: calm session must carry no attention", path)
		}
		cardKey := view.Ref{Kind: view.KindBoard, ID: view.BoardRefID, Sub: failed.ID}.String()
		if a, ok := got.Attention[cardKey]; !ok || a.Level != view.AttentionDanger || a.Reasons[0] != view.ReasonFailedCard {
			t.Errorf("%s: failed card attention = %+v, want danger/failed-card", path, got.Attention[cardKey])
		}
		if got.Status == nil {
			t.Fatalf("%s: no status strip", path)
		}
		if got.Status.Stuck != 1 || got.Status.FailedCards != 1 || got.Status.Waiting != 0 || got.Status.Running != 0 {
			t.Errorf("%s: status = %+v, want stuck=1 failedCards=1 waiting=0 running=0", path, *got.Status)
		}
	}

	// An interactive prompt (ask_user in a chat turn) never reaches the durable
	// SessionAsk table; the map must still say "waiting for you" while it is
	// open and drop the marker the moment it is answered.
	pi := s.openInteraction(wsp.ID, calm.ID, "ask", map[string]any{"question": "mavi mi yesil mi?"})
	calmKey := view.Ref{Kind: view.KindSession, ID: calm.ID}.String()
	got := get("/api/views/graph/live")
	if a, ok := got.Attention[calmKey]; !ok || a.Level != view.AttentionWarn || a.Reasons[0] != view.ReasonWaitingAsk {
		t.Errorf("open prompt: attention = %+v, want warn/waiting-ask", got.Attention[calmKey])
	}
	if got.Status.Waiting != 1 {
		t.Errorf("open prompt: status.waiting = %d, want 1", got.Status.Waiting)
	}
	if !s.resolveInteraction(wsp.ID, calm.ID, pi.id, "mavi", "test") {
		t.Fatal("resolve interaction failed")
	}
	got = get("/api/views/graph/live")
	if _, ok := got.Attention[calmKey]; ok {
		t.Errorf("resolved prompt must clear the marker, got %+v", got.Attention[calmKey])
	}
	if got.Status.Waiting != 0 {
		t.Errorf("resolved prompt: status.waiting = %d, want 0", got.Status.Waiting)
	}
}
