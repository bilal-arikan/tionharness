package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func TestSessionActivityEndpoint(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	ag, err := database.CreateAgent(ctx, db.Agent{Name: "Builder"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "Root"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	for _, text := range []string{"merhaba", "selam"} {
		if _, err := database.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: "user", Text: text}); err != nil {
			t.Fatalf("add message: %v", err)
		}
	}
	empty, err := database.CreateSession(ctx, db.Session{AgentID: ag.ID, Title: "Silent"})
	if err != nil {
		t.Fatalf("create empty session: %v", err)
	}

	server := newTestServer()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Test"}, DB: database}
	call := func(query string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/activity"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		rec := httptest.NewRecorder()
		mux := http.NewServeMux()
		mux.HandleFunc("GET /api/sessions/activity", server.handleSessionActivity)
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := call("?ids=" + sess.ID + ",SESGONE," + empty.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		GapSec   int64 `json:"gapSec"`
		Sessions map[string][]struct {
			Start int64 `json:"start"`
			End   int64 `json:"end"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.GapSec != 600 {
		t.Fatalf("gapSec = %d, want the default 600", resp.GapSec)
	}
	// The two messages land in the same minute, so they are one bout; an
	// unknown id and a session with no transcript are simply absent.
	spans := resp.Sessions[sess.ID]
	if len(spans) != 1 {
		t.Fatalf("spans = %v, want one bout", spans)
	}
	if spans[0].End < spans[0].Start {
		t.Fatalf("span %v runs backwards", spans[0])
	}
	if _, ok := resp.Sessions["SESGONE"]; ok {
		t.Fatalf("unknown session reported: %v", resp.Sessions)
	}
	if _, ok := resp.Sessions[empty.ID]; ok {
		t.Fatalf("session without messages reported: %v", resp.Sessions)
	}
}

func TestSessionActivityRejectsBadInput(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	server := newTestServer()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "Test"}, DB: database}
	call := func(query string) int {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/sessions/activity"+query, nil)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		rec := httptest.NewRecorder()
		server.handleSessionActivity(rec, req)
		return rec.Code
	}

	if code := call(""); code != http.StatusBadRequest {
		t.Fatalf("missing ids = %d, want 400", code)
	}
	if code := call("?ids=SES1&gap=0"); code != http.StatusBadRequest {
		t.Fatalf("gap=0 = %d, want 400", code)
	}
	if code := call("?ids=SES1&gap=abc"); code != http.StatusBadRequest {
		t.Fatalf("gap=abc = %d, want 400", code)
	}
}

func TestSplitActivityIDsDedupesAndKeepsOrder(t *testing.T) {
	got := splitActivityIDs(" SES2 , SES1,SES2, ,SES3")
	want := []string{"SES2", "SES1", "SES3"}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("id %d = %q, want %q", i, got[i], want[i])
		}
	}
}
