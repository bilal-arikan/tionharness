package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// postMessage drives handleEnqueueMessage for a session id.
func postMessage(t *testing.T, s *Server, wsp *workspace.Workspace, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/messages",
		bytes.NewReader([]byte(`{"message":"merhaba"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleEnqueueMessage(rec, req)
	return rec
}

// postControl drives handleSessionControl ("stop") for a session id.
func postControl(t *testing.T, s *Server, wsp *workspace.Workspace, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/control",
		bytes.NewReader([]byte(`{"action":"stop"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleSessionControl(rec, req)
	return rec
}

// postInteractionAnswer drives handleInteractionAnswer for a session id.
func postInteractionAnswer(t *testing.T, s *Server, wsp *workspace.Workspace, sessionID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/interactions/INT-1",
		bytes.NewReader([]byte(`{"answer":"evet"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sessionID)
	req.SetPathValue("iid", "INT-1")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleInteractionAnswer(rec, req)
	return rec
}

// postRewind drives handleRewindSession for a session id.
func postRewind(t *testing.T, s *Server, wsp *workspace.Workspace, sessionID, msgID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessionID+"/rewind",
		bytes.NewReader([]byte(`{"messageId":"`+msgID+`"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sessionID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleRewindSession(rec, req)
	return rec
}

// TestRewindRejectedOnReadOnlySessions: a rewind truncates the transcript at a
// past checkpoint. On an orchestrator-owned run log there is no composer to
// re-drive the conversation from that checkpoint, so the rewind would only
// destroy the record — every non-writable kind must refuse it with 403, and the
// messages must survive.
func TestRewindRejectedOnReadOnlySessions(t *testing.T) {
	for _, kind := range []string{"task", "flow", "schedule", "automation", "flow-coordinator", "worker", db.SessionKindInsight} {
		t.Run(kind, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			ctx := context.Background()
			sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: kind, Title: kind})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			msg, err := wsp.DB.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: "user", Text: "ilk"})
			if err != nil {
				t.Fatalf("add message: %v", err)
			}
			rec := postRewind(t, s, wsp, sess.ID, msg.ID)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "read-only") {
				t.Fatalf("error must explain the refusal, got: %s", rec.Body.String())
			}
			msgs, err := wsp.DB.ListMessages(ctx, sess.ID)
			if err != nil {
				t.Fatalf("list messages: %v", err)
			}
			if len(msgs) != 1 {
				t.Fatalf("a refused rewind must not touch the transcript, got %d messages", len(msgs))
			}
		})
	}
}

// TestRewindAllowedOnWritableKinds is the no-regression companion: an ordinary
// chat still rewinds with the guard in place.
func TestRewindAllowedOnWritableKinds(t *testing.T) {
	for _, kind := range db.WritableSessionKinds() {
		t.Run("kind="+kind, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			ctx := context.Background()
			sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: kind})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			msg, err := wsp.DB.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: "user", Text: "ilk"})
			if err != nil {
				t.Fatalf("add message: %v", err)
			}
			if rec := postRewind(t, s, wsp, sess.ID, msg.ID); rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestEnqueueMessageRejectsOrchestratorSessions: task/flow/schedule/automation
// transcripts are orchestrator-owned run logs — the UI hides the composer for
// them and the API must agree, instead of silently accepting a turn that has no
// run to attach to.
func TestEnqueueMessageRejectsOrchestratorSessions(t *testing.T) {
	for _, kind := range []string{"task", "flow", "schedule", "automation", "flow-coordinator", "worker"} {
		t.Run(kind, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: kind, Title: kind})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			rec := postMessage(t, s, wsp, sess.ID)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), kind) {
				t.Fatalf("error must name the kind, got: %s", rec.Body.String())
			}
			s.inbox.lock()
			defer s.inbox.unlock()
			if ib := s.inbox.at(wsp.ID, sess.ID); ib != nil {
				t.Fatal("a refused message must not create a queue for the session")
			}
		})
	}
}

// TestRunningTurnControlsAllowedOnOrchestratorSessions: stop/steer and answering
// an ask_user belong to an ALREADY RUNNING turn, which a task/flow session very
// much has. The immutable guard must not touch them (they may still fail for
// their own reasons — no live run, no such interaction — just never with 403).
func TestRunningTurnControlsAllowedOnOrchestratorSessions(t *testing.T) {
	for _, kind := range []string{"task", "flow"} {
		t.Run(kind, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: kind})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			if rec := postControl(t, s, wsp, sess.ID); rec.Code == http.StatusForbidden {
				t.Fatalf("session control must not be blocked on a %s session: %s", kind, rec.Body.String())
			}
			if rec := postInteractionAnswer(t, s, wsp, sess.ID); rec.Code == http.StatusForbidden {
				t.Fatalf("interaction answer must not be blocked on a %s session: %s", kind, rec.Body.String())
			}
		})
	}
}

// TestImmutableSessionRejectsBothGuards: an insight record runs no turn at all,
// so it trips the writable guard AND the immutable one.
func TestImmutableSessionRejectsBothGuards(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{
		Kind:     db.SessionKindInsight,
		SourceID: "IRUN-9",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if rec := postMessage(t, s, wsp, sess.ID); rec.Code != http.StatusForbidden {
		t.Fatalf("message: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := postControl(t, s, wsp, sess.ID); rec.Code != http.StatusForbidden {
		t.Fatalf("control: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := postInteractionAnswer(t, s, wsp, sess.ID); rec.Code != http.StatusForbidden {
		t.Fatalf("interaction answer: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEnqueueMessageAllowsWritableKinds: the manual-chat kinds are untouched.
func TestEnqueueMessageAllowsWritableKinds(t *testing.T) {
	for _, kind := range db.WritableSessionKinds() {
		t.Run("kind="+kind, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			sess, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: kind})
			if err != nil {
				t.Fatalf("create session: %v", err)
			}
			if rec := postMessage(t, s, wsp, sess.ID); rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
