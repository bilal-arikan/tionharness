package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestSessionSummaryReportsTurnClaimFailure pins finding #5: when the slash
// command could not claim the session's turn slot the handler returned WITHOUT
// writing anything, so net/http sent a bare 200 with an empty body — the frontend
// then failed to parse the reply and the "working" bubble never cleared with a
// reason. The claim is only abandoned when the request context is cancelled (the
// queue itself never times out), so that is the case under test: it must be an
// error status carrying an explanatory JSON body.
func TestSessionSummaryReportsTurnClaimFailure(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()

	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "no-such-provider"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Someone else owns the slot, and the client gives up while queued behind it.
	release := wsp.Runtime.BeginSessionUserTurn(sess.ID)
	defer release()

	reqCtx, cancel := context.WithCancel(context.WithValue(ctx, workspaceCtxKey, wsp))
	cancel()

	req := httptest.NewRequest("POST", "/api/sessions/"+sess.ID+"/summary", strings.NewReader(`{"kind":"board"}`))
	req.SetPathValue("id", sess.ID)
	req = req.WithContext(reqCtx)
	rec := httptest.NewRecorder()
	s.handleSessionSummary(rec, req)

	if rec.Code == 200 {
		t.Fatalf("a refused slash command must not answer 200 (body %q)", rec.Body.String())
	}
	if rec.Code != statusClientClosedRequest {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, statusClientClosedRequest, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v (body %q)", err, rec.Body.String())
	}
	if !strings.Contains(body.Error, "/board") {
		t.Fatalf("error body must name the refused command, got %q", body.Error)
	}

	// Nothing durable may have happened: the command message is persisted only
	// AFTER the slot is held.
	msgs, err := wsp.DB.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("refused command persisted %d messages, want 0", len(msgs))
	}
}

// TestWriteTurnClaimErrorBusyIsConflict covers the other branch of the same
// helper: a claim that fails while the client is still connected is a busy
// session, and must be reported as 409 rather than the old silent 200.
func TestWriteTurnClaimErrorBusyIsConflict(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	rec := httptest.NewRecorder()

	s.writeTurnClaimError(context.Background(), rec, "SES1", "/compact", errors.New("slot unavailable"))

	if rec.Code != 409 {
		t.Fatalf("status = %d, want 409 (body %q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "slot unavailable") {
		t.Fatalf("body must carry the underlying cause, got %q", rec.Body.String())
	}
}
