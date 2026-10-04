package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/notes"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// The delivery gate (_Docs/94 §8): the one test that asks the provider what
// context actually arrived. A brief that is composed but never shipped, a pulse
// that re-sends unchanged, a digest that is never written — each would pass
// every unit test and still leave the agent blind. This drives the real chat
// endpoint with a recording provider and reads the requests it received.

func newAwarenessHarness(t *testing.T) (*Server, string, string, <-chan providers.Request) {
	t.Helper()
	s, wsp := newWorkspaceServer(t)
	recorder := &recordingCLIProvider{requests: make(chan providers.Request, 8)}
	kindID := fmt.Sprintf("recording-cli-%d", recordingCLIKindID.Add(1))
	providers.RegisterKind(providers.NewBuiltinKind(
		providers.Manifest{Kind: kindID, Label: "Recording CLI", Transport: providers.TransportCLI},
		func(providers.ResolvedConfig) bool { return true },
		func(providers.ResolvedConfig) (providers.Provider, error) { return recorder, nil },
	))
	s.providers.SetInstances([]providers.Instance{{ID: kindID, KindID: kindID, Enabled: true}})
	ctx := context.Background()
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{
		Name: "Ada", Provider: "codex-cli", ProviderInstanceID: kindID, Model: "gpt-5.6-sol", PermissionMode: "auto",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// A note that reaches every session, and a stuck sibling session so the
	// first pulse has something to say.
	if _, err := wsp.Runtime.Notes().Put(notes.Note{
		Kind: notes.KindLesson, Title: "Quote shell paths", Body: "Paths with spaces break unquoted.",
		Scope: notes.ScopeWorkspace, Confidence: notes.ConfidenceVerified, Verification: "seen twice", Source: notes.SourceUser,
	}); err != nil {
		t.Fatalf("seed note: %v", err)
	}
	if _, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "Stuck sibling", AgentID: agentRow.ID, StuckTurns: 2}); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	session, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "Delivery", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s, wsp.ID, session.ID, recorder.requests
}

func postChat(t *testing.T, s *Server, workspaceID, sessionID, message string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(chatReq{SessionID: sessionID, Message: message})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-Id", workspaceID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("chat status = %d: %s", rec.Code, rec.Body.String())
	}
	return rec
}

func nextRequest(t *testing.T, requests <-chan providers.Request) providers.Request {
	t.Helper()
	select {
	case req := <-requests:
		return req
	case <-time.After(5 * time.Second):
		t.Fatal("provider was not called")
	}
	return providers.Request{}
}

func TestAwarenessDeliveryGate(t *testing.T) {
	s, wsID, sessID, requests := newAwarenessHarness(t)
	wsp, _ := s.workspaces.Get(wsID)

	// Turn 1: the brief rides the static prefix, the metered suffix rides the
	// dynamic half, and the changed pulse is delivered.
	postChat(t, s, wsID, sessID, "hello")
	first := nextRequest(t, requests)
	for _, want := range []string{"# Session briefing", "## Workspace now", "## Memory (notes that reach this session)", "Quote shell paths", "[context meter · brief"} {
		if !strings.Contains(first.System, want) {
			t.Fatalf("turn 1 static prefix lacks %q:\n%s", want, first.System)
		}
	}
	for _, want := range []string{"Current date and time", "[workspace pulse", "1 stuck", "[context meter · turn"} {
		if !strings.Contains(first.SystemDynamic, want) {
			t.Fatalf("turn 1 dynamic suffix lacks %q:\n%s", want, first.SystemDynamic)
		}
	}
	if strings.Contains(first.SystemDynamic, "# Session briefing") {
		t.Fatal("the brief must not also ride the volatile suffix")
	}

	// The digest was recorded for the finished turn (the funnel every
	// completion path hits).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if d, ok := wsp.Runtime.Awareness().LoadDigest(sessID); ok && d.Messages >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("digest was not recorded after the turn")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Turn 2, nothing changed in the workspace: the brief is byte-identical
	// (frozen into the cached prefix) and the unchanged pulse stays silent.
	postChat(t, s, wsID, sessID, "again")
	second := nextRequest(t, requests)
	if second.System != first.System {
		t.Fatalf("static prefix changed between turns without an adopt point:\n--- first\n%s\n--- second\n%s", first.System, second.System)
	}
	if strings.Contains(second.SystemDynamic, "[workspace pulse") {
		t.Fatalf("an unchanged pulse must not be re-sent:\n%s", second.SystemDynamic)
	}
	if !strings.Contains(second.SystemDynamic, "[context meter · turn") {
		t.Fatal("every turn is metered")
	}

	// The API exposes what the agent saw.
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sessID+"/awareness", nil)
	req.Header.Set("X-Workspace-Id", wsID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("awareness endpoint: %d %s", rec.Code, rec.Body.String())
	}
	var seen struct {
		Turns int `json:"turns"`
		Brief *struct {
			Meter string `json:"meter"`
		} `json:"brief"`
		Digest *struct {
			SessionID string `json:"sessionId"`
		} `json:"digest"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &seen); err != nil {
		t.Fatal(err)
	}
	if seen.Turns != 2 || seen.Brief == nil || !strings.Contains(seen.Brief.Meter, "brief") || seen.Digest == nil || seen.Digest.SessionID != sessID {
		t.Fatalf("seen: %s", rec.Body.String())
	}
}
