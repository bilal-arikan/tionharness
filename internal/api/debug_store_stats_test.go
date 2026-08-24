package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func getStoreStats(t *testing.T, s *Server) storeStatsResp {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handleStoreStats(rec, httptest.NewRequest(http.MethodGet, "/api/debug/store-stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var got storeStatsResp
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return got
}

// TestHandleStoreStatsReportsMessageFootprint: the endpoint must attribute the
// footprint per workspace AND sum it, because the whole point is answering "which
// workspace is holding the memory, and how much in total?".
func TestHandleStoreStatsReportsMessageFootprint(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	base := getStoreStats(t, s)
	if len(base.Workspaces) != 1 {
		t.Fatalf("workspaces = %d, want 1", len(base.Workspaces))
	}
	if base.Workspaces[0].WorkspaceID != wsp.ID {
		t.Fatalf("workspaceId = %q, want %q", base.Workspaces[0].WorkspaceID, wsp.ID)
	}

	agentRec, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRec.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	text := make([]byte, 20_000)
	for i := range text {
		text[i] = 'x'
	}
	if _, err := wsp.DB.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: "user", Text: string(text)}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	got := getStoreStats(t, s)
	row := got.Workspaces[0]
	if row.Messages != 1 {
		t.Fatalf("messages = %d, want 1", row.Messages)
	}
	if row.LoadedSessions != 1 {
		t.Fatalf("loadedSessions = %d, want 1", row.LoadedSessions)
	}
	if row.MessageBytes < 20_000 {
		t.Fatalf("messageBytes = %d for a 20KB message, want >= 20000", row.MessageBytes)
	}
	// Totals must actually aggregate, not echo a single row by accident.
	if got.Totals.Messages != row.Messages || got.Totals.MessageBytes != row.MessageBytes {
		t.Fatalf("totals %+v do not match the single row %+v", got.Totals, row)
	}
}

// TestHandleStoreStatsSumsAcrossWorkspaces guards the aggregation itself: with
// two workspaces the totals must be the sum, since the headline number is what a
// "the app eats memory" report is answered with.
func TestHandleStoreStatsSumsAcrossWorkspaces(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	if _, err := s.workspaces.Create("second", "", "test"); err != nil {
		t.Fatalf("create second workspace: %v", err)
	}

	got := getStoreStats(t, s)
	if len(got.Workspaces) != 2 {
		t.Fatalf("workspaces = %d, want 2", len(got.Workspaces))
	}
	wantSessions := 0
	for _, r := range got.Workspaces {
		wantSessions += r.Sessions
	}
	if got.Totals.Sessions != wantSessions {
		t.Fatalf("totals.sessions = %d, want %d", got.Totals.Sessions, wantSessions)
	}
}

// TestHandleStoreStatsWithoutWorkspaceManager: a bare server must answer with an
// empty report rather than panicking on a nil manager.
func TestHandleStoreStatsWithoutWorkspaceManager(t *testing.T) {
	s := &Server{}
	got := getStoreStats(t, s)
	if len(got.Workspaces) != 0 || got.Totals.Sessions != 0 {
		t.Fatalf("bare server reported %+v, want an empty report", got)
	}
}
