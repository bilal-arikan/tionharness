package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestEnqueueMessageRejectsInsightSession: an insight scan session is a record,
// not a chat. Posting a user message to it must fail loudly (403) and must NOT
// enqueue anything.
func TestEnqueueMessageRejectsInsightSession(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{
		Kind:     db.SessionKindInsight,
		SourceID: "IRUN-1",
		Title:    "İçgörü taraması",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/messages",
		bytes.NewReader([]byte(`{"message":"merhaba"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sess.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleEnqueueMessage(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "read-only") || !strings.Contains(rec.Body.String(), "insight") {
		t.Fatalf("error must explain the refusal and name the kind, got: %s", rec.Body.String())
	}
	s.inbox.lock()
	defer s.inbox.unlock()
	if ib := s.inbox.at(wsp.ID, sess.ID); ib != nil {
		t.Fatal("a refused message must not create a queue for the session")
	}
}

// TestEnqueueMessageAllowsNormalSession is the no-regression companion: an
// ordinary session still accepts messages with the guard in place.
func TestEnqueueMessageAllowsNormalSession(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Title: "normal"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/messages",
		bytes.NewReader([]byte(`{"message":"merhaba"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sess.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleEnqueueMessage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestEnqueueRefusalReasonDistinguishesGuards: both guards answer 403, but they
// must not answer with the same sentence. A task session is non-writable yet
// mutable ("no run to attach to"); an insight session is a finished record
// ("read-only record"). Mixing the two wordings would tell a user their task
// transcript is frozen forever, which is false.
func TestEnqueueRefusalReasonDistinguishesGuards(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	task, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "task"})
	if err != nil {
		t.Fatalf("create task session: %v", err)
	}
	insight, err := wsp.DB.CreateSession(ctx, db.Session{Kind: db.SessionKindInsight, SourceID: "IRUN-3"})
	if err != nil {
		t.Fatalf("create insight session: %v", err)
	}

	taskBody := postMessage(t, s, wsp, task.ID).Body.String()
	if strings.Contains(taskBody, "read-only record") {
		t.Fatalf("a task session is not an immutable record, wrong reason: %s", taskBody)
	}
	if !strings.Contains(taskBody, "no run to attach to") {
		t.Fatalf("task refusal must explain the missing run, got: %s", taskBody)
	}

	insightBody := postMessage(t, s, wsp, insight.ID).Body.String()
	if !strings.Contains(insightBody, "read-only record") {
		t.Fatalf("insight refusal must name it a read-only record, got: %s", insightBody)
	}
}

// TestSessionControlRejectsInsightSession: steering/stopping a turn on a
// read-only session is refused by the same guard, before any run lookup.
func TestSessionControlRejectsInsightSession(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	sess, err := wsp.DB.CreateSession(context.Background(), db.Session{
		Kind:     db.SessionKindInsight,
		SourceID: "IRUN-2",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/control",
		bytes.NewReader([]byte(`{"action":"steer","text":"dur"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", sess.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleSessionControl(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
