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

// postSteerQueued drives handleSteerQueued for one queued message id.
func postSteerQueued(t *testing.T, s *Server, wsp *workspace.Workspace, sessionID, msgID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/queue/"+msgID+"/steer", nil)
	req.SetPathValue("id", sessionID)
	req.SetPathValue("msgId", msgID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleSteerQueued(rec, req)
	return rec
}

// steerResult reads the {"result": ...} body of a 200 answer.
func steerResult(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body["result"]
}

// queuedMessages snapshots the session's WAITING queue texts, in order.
func queuedMessages(s *Server, wsID, sessionID string) []string {
	s.inbox.lock()
	defer s.inbox.unlock()
	ib := s.inbox.at(wsID, sessionID)
	if ib == nil {
		return nil
	}
	out := make([]string, 0, len(ib.items))
	for _, it := range ib.items {
		out = append(out, it.Req.Message)
	}
	return out
}

// steerQueuedFixture sets up a session with one waiting message and a registered
// in-flight run. The session's turn slot is held for the test's lifetime so the
// serial worker cannot dispatch the queued message out from under the assertions.
func steerQueuedFixture(t *testing.T, msg string) (*Server, *workspace.Workspace, string, *chatRun) {
	t.Helper()
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(wsp.Runtime.BeginSessionUserTurn(sess.ID))

	if !s.enqueueMessage(wsp.ID, chatReq{SessionID: sess.ID, Message: msg}, "m-1") {
		t.Fatal("enqueue rejected")
	}
	run := s.runs.register("r-q", sess.ID, wsp.ID, func() {})
	t.Cleanup(func() { s.runs.unregister("r-q") })
	return s, wsp, sess.ID, run
}

// TestSteerQueuedConvertsWaitingMessage is the happy path: the queued text
// reaches the running turn AND leaves the queue, so it is delivered exactly once
// — never both steered and re-run as its own turn.
func TestSteerQueuedConvertsWaitingMessage(t *testing.T) {
	s, wsp, sessionID, run := steerQueuedFixture(t, "use the other endpoint")

	rec := postSteerQueued(t, s, wsp, sessionID, "m-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("steer queued = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if got := steerResult(t, rec); got != "steered" {
		t.Fatalf("result = %q, want \"steered\"", got)
	}
	select {
	case got := <-run.steer:
		if got != "use the other endpoint" {
			t.Fatalf("steer channel got %q", got)
		}
	default:
		t.Fatal("converted message never reached the run's steer channel")
	}
	if left := queuedMessages(s, wsp.ID, sessionID); len(left) != 0 {
		t.Fatalf("queue still holds %v after a successful conversion — it would run twice", left)
	}
}

// TestSteerQueuedKeepsMessageWhenUnsupported is the critical no-loss guard: when
// the running turn has no boundary a steer can ride, the conversion must refuse
// AND leave the message in the queue, so the user's text still runs as its own
// turn instead of disappearing.
func TestSteerQueuedKeepsMessageWhenUnsupported(t *testing.T) {
	s, wsp, sessionID, run := steerQueuedFixture(t, "check the other file first")
	// claude-cli in "auto"/"read-only": no permission-prompt boundary, so
	// steerableFor() stays false.
	run.setProvider("claude-cli")
	run.setSteerable(false)

	rec := postSteerQueued(t, s, wsp, sessionID, "m-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("steer queued = %d %s, want 200", rec.Code, rec.Body.String())
	}
	if got := steerResult(t, rec); got != "unsupported" {
		t.Fatalf("result = %q, want \"unsupported\"", got)
	}
	if got := queuedMessages(s, wsp.ID, sessionID); len(got) != 1 || got[0] != "check the other file first" {
		t.Fatalf("queue = %v, want the message still waiting — a refused steer must not consume it", got)
	}
	if run.takeSteer() != "" {
		t.Fatal("an unsupported turn must not be handed the guidance")
	}
}

// TestSteerQueuedReportsFullBufferAndKeepsMessage: the turn is not consuming
// guidance, so the conversion fails loudly (503) and the message stays queued.
func TestSteerQueuedReportsFullBufferAndKeepsMessage(t *testing.T) {
	s, wsp, sessionID, run := steerQueuedFixture(t, "abort, wrong file")
	for cap(run.steer) > len(run.steer) {
		run.steer <- "filler"
	}

	rec := postSteerQueued(t, s, wsp, sessionID, "m-1")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("steer into a full queue = %d %s, want 503", rec.Code, rec.Body.String())
	}
	if got := queuedMessages(s, wsp.ID, sessionID); len(got) != 1 || got[0] != "abort, wrong file" {
		t.Fatalf("queue = %v, want the message still waiting after a rejected steer", got)
	}
}

// TestSteerQueuedUnknownMessage: an id that is not waiting (already dispatched or
// cancelled) is a 404 — not a silent no-op the client would read as success.
func TestSteerQueuedUnknownMessage(t *testing.T) {
	s, wsp, sessionID, _ := steerQueuedFixture(t, "still queued")

	rec := postSteerQueued(t, s, wsp, sessionID, "m-nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("steer of an unknown message = %d %s, want 404", rec.Code, rec.Body.String())
	}
	if got := queuedMessages(s, wsp.ID, sessionID); len(got) != 1 {
		t.Fatalf("queue = %v, want the unrelated message untouched", got)
	}
}

// TestSteerQueuedRejectsAttachments: a steer carries text only, so converting a
// message with uploads would drop them. Refuse and keep it queued.
func TestSteerQueuedRejectsAttachments(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(wsp.Runtime.BeginSessionUserTurn(sess.ID))
	req := chatReq{
		SessionID:   sess.ID,
		Message:     "look at this",
		Attachments: []db.Attachment{{ID: "att-1", Name: "a.png", RelPath: "uploads/a.png"}},
	}
	if !s.enqueueMessage(wsp.ID, req, "m-att") {
		t.Fatal("enqueue rejected")
	}
	s.runs.register("r-att", sess.ID, wsp.ID, func() {})
	t.Cleanup(func() { s.runs.unregister("r-att") })

	rec := postSteerQueued(t, s, wsp, sess.ID, "m-att")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("steer of a message with attachments = %d %s, want 400", rec.Code, rec.Body.String())
	}
	if got := queuedMessages(s, wsp.ID, sess.ID); len(got) != 1 {
		t.Fatalf("queue = %v, want the message still waiting", got)
	}
}

// TestSteerQueuedWithoutRunningTurn: with no in-flight turn there is nothing to
// steer, so the queued message is left alone and the caller told 404.
func TestSteerQueuedWithoutRunningTurn(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(wsp.Runtime.BeginSessionUserTurn(sess.ID))
	if !s.enqueueMessage(wsp.ID, chatReq{SessionID: sess.ID, Message: "no turn yet"}, "m-1") {
		t.Fatal("enqueue rejected")
	}

	rec := postSteerQueued(t, s, wsp, sess.ID, "m-1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("steer with no in-flight turn = %d %s, want 404", rec.Code, rec.Body.String())
	}
	if got := queuedMessages(s, wsp.ID, sess.ID); len(got) != 1 {
		t.Fatalf("queue = %v, want the message still waiting", got)
	}
}
