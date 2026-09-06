package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// postSteer drives handleSessionControl's "steer" action for a session id.
func postSteer(t *testing.T, s *Server, wsp *workspace.Workspace, sessionID, text string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"action": "steer", "text": text})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/control", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleSessionControl(rec, req)
	return rec
}

// TestSessionSteerReportsFullQueue is the regression guard for the silently
// dropped steer: the handler used to answer 200 "ok" while a `default:` branch
// threw the message away whenever the channel buffer was full. A steer that
// cannot be handed to the turn must fail loudly, so the client keeps the text
// and the user knows their guidance did not land.
func TestSessionSteerReportsFullQueue(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	run := s.runs.register("r-full", sess.ID, wsp.ID, func() {})
	defer s.runs.unregister("r-full")

	// Fill the buffer: the turn is not draining (a stalled provider call).
	for cap(run.steer) > len(run.steer) {
		run.steer <- "filler"
	}
	rec := postSteer(t, s, wsp, sess.ID, "abort, wrong file")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("steer into a full queue = %d %s, want 503", rec.Code, rec.Body.String())
	}
}

// TestSessionSteerAcceptedWhenQueueHasRoom pins the happy path unchanged: the
// message reaches the run's steer channel and the client is told "ok".
func TestSessionSteerAcceptedWhenQueueHasRoom(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	run := s.runs.register("r-ok", sess.ID, wsp.ID, func() {})
	defer s.runs.unregister("r-ok")

	if rec := postSteer(t, s, wsp, sess.ID, "use the other endpoint"); rec.Code != http.StatusOK {
		t.Fatalf("steer = %d %s, want 200", rec.Code, rec.Body.String())
	}
	select {
	case got := <-run.steer:
		if got != "use the other endpoint" {
			t.Fatalf("steer channel got %q", got)
		}
	default:
		t.Fatal("accepted steer never reached the run's steer channel")
	}
}

// TestChatControlSteerReportsFullQueue covers the runId-scoped control endpoint,
// which had the same silent `default:` drop.
func TestChatControlSteerReportsFullQueue(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	run := s.runs.register("r-cc", "SES-cc", wsp.ID, func() {})
	defer s.runs.unregister("r-cc")
	for cap(run.steer) > len(run.steer) {
		run.steer <- "filler"
	}

	body, _ := json.Marshal(map[string]string{"runId": "r-cc", "action": "steer", "text": "abort"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat/control", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleChatControl(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("steer into a full queue = %d %s, want 503", rec.Code, rec.Body.String())
	}
}

// TestRecoverUndeliveredSteerRequeuesChannelMessages is the regression guard for
// the second loss path: a steer that arrives after the turn's last drain point
// (every steer sent during a tool-less turn's single completion) sat unread on
// the channel and died with the run — the turn-end fallback only rescued the
// claude-cli stash. Both sources must now land back in the queue, oldest first.
func TestRecoverUndeliveredSteerRequeuesChannelMessages(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", Title: "t"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	// Hold the session's turn slot so the queue worker cannot dispatch what the
	// fallback enqueues before the assertions read it.
	defer wsp.Runtime.BeginSessionUserTurn(sess.ID)()

	run := s.runs.register("r-rec", sess.ID, wsp.ID, func() {})
	defer s.runs.unregister("r-rec")
	run.steer <- "first guidance"
	run.steer <- "second guidance"
	run.setSteer("cli stash")

	s.recoverUndeliveredSteer(run, wsp.ID, chatReq{SessionID: sess.ID})

	s.inbox.lock()
	ib := s.inbox.at(wsp.ID, sess.ID)
	var queued []string
	if ib != nil {
		for _, it := range ib.items {
			queued = append(queued, it.Req.Message)
		}
	}
	s.inbox.unlock()

	want := []string{"first guidance", "second guidance", "cli stash"}
	if len(queued) != len(want) {
		t.Fatalf("requeued %v, want %v", queued, want)
	}
	for i := range want {
		if queued[i] != want[i] {
			t.Fatalf("requeued %v, want %v (order matters: oldest guidance first)", queued, want)
		}
	}
	if len(run.steer) != 0 {
		t.Fatal("steer channel must be empty after the turn-end recovery")
	}
}
