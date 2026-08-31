package api

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// sessionEaterKind is the kind id the fake transport below registers itself as.
const sessionEaterKind = "test-session-eater"

// sessionEaterProvider is a do-nothing transport whose only job is to give the
// test a deterministic hook INSIDE the turn: the registry builds a provider on
// every Get, and runChatTurn's per-agent Get sits between the user message being
// persisted and the session being re-read. onBuild therefore fires in exactly the
// window where the session can disappear under a running turn.
type sessionEaterProvider struct{}

func (p *sessionEaterProvider) Name() string { return sessionEaterKind }

func (p *sessionEaterProvider) Complete(ctx context.Context, req providers.Request) (*providers.Response, error) {
	return nil, errors.New("sessionEaterProvider must never be reached")
}

// useSessionEaterProvider registers the fake transport and runs onBuild each time
// the registry builds it. Registration is idempotent (the kind registry is keyed
// by kind id) and this package runs its tests sequentially, so the global write is
// safe here.
func useSessionEaterProvider(t *testing.T, s *Server, onBuild func()) {
	t.Helper()
	providers.RegisterKind(providers.NewBuiltinKind(
		providers.Manifest{
			Kind:      sessionEaterKind,
			Label:     "Session eater (test)",
			Order:     9001,
			Transport: "api",
		},
		func(providers.ResolvedConfig) bool { return true },
		func(providers.ResolvedConfig) (providers.Provider, error) {
			onBuild()
			return &sessionEaterProvider{}, nil
		},
	))
	s.providers.SetInstances([]providers.Instance{{
		ID:      sessionEaterKind,
		KindID:  sessionEaterKind,
		Label:   "Session eater (test)",
		Enabled: true,
	}})
}

// TestTurnAbortsWhenSessionVanishesMidTurn pins finding #4: the per-agent refresh
// used to be `session, _ = database.GetSession(...)`, so a session deleted while
// the turn ran silently replaced `session` with the zero value. The rest of the
// turn then wrote its reply under an EMPTY session id and published to a hub scope
// no window watches. The turn must fail loudly with session_not_found instead.
func TestTurnAbortsWhenSessionVanishesMidTurn(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()

	var once sync.Once
	var sessID string
	useSessionEaterProvider(t, s, func() {
		// Delete the session at the one point that is provably after the user
		// message was persisted and before the per-agent GetSession.
		once.Do(func() {
			if err := wsp.DB.DeleteSession(context.Background(), sessID); err != nil {
				t.Errorf("delete session mid-turn: %v", err)
			}
		})
	})

	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{
		Name:       "eater",
		Provider:   sessionEaterKind,
		Model:      "test-model",
		MCPEnabled: false,
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessID = sess.ID

	var mu sync.Mutex
	events := map[string]map[string]any{}
	s.runChatTurn(ctx, wsp, chatReq{SessionID: sess.ID, Message: "merhaba"}, func(event string, data any) {
		mu.Lock()
		defer mu.Unlock()
		payload, _ := data.(map[string]any)
		events[event] = payload
	})

	mu.Lock()
	defer mu.Unlock()
	if _, ok := events["done"]; ok {
		t.Fatal("turn reported success although its session was deleted mid-turn")
	}
	payload, ok := events["error"]
	if !ok {
		t.Fatalf("turn emitted no error event; events = %v", events)
	}
	if payload["reason"] != "session_not_found" {
		t.Fatalf("error reason = %v, want session_not_found (payload %v)", payload["reason"], payload)
	}
}
