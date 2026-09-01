package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type recordingCLIProvider struct {
	requests chan providers.Request
}

func (p *recordingCLIProvider) Name() string { return "recording-cli" }

func (p *recordingCLIProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	p.requests <- req
	return &providers.Response{Text: "ok", StopReason: providers.StopEndTurn}, nil
}

func (*recordingCLIProvider) Preflight(context.Context) error      { return nil }
func (*recordingCLIProvider) SetConfigDir(string)                  {}
func (*recordingCLIProvider) ConfigureCLIMCP(providers.CLIMCPSpec) {}
func (*recordingCLIProvider) Installed() bool                      { return true }

var recordingCLIKindID atomic.Uint64

func newChatThinkingHarness(t *testing.T) (*Server, string, string, <-chan providers.Request) {
	t.Helper()
	s, wsp := newWorkspaceServer(t)
	recorder := &recordingCLIProvider{requests: make(chan providers.Request, 4)}
	kindID := fmt.Sprintf("recording-cli-%d", recordingCLIKindID.Add(1))
	providers.RegisterKind(providers.NewBuiltinKind(
		providers.Manifest{Kind: kindID, Label: "Recording CLI", Transport: providers.TransportCLI},
		func(providers.ResolvedConfig) bool { return true },
		func(providers.ResolvedConfig) (providers.Provider, error) { return recorder, nil },
	))
	s.providers.SetInstances([]providers.Instance{{ID: kindID, KindID: kindID, Enabled: true}})

	agentRow, err := wsp.DB.CreateAgent(context.Background(), db.Agent{
		Name:               "Codex",
		Provider:           "codex-cli",
		ProviderInstanceID: kindID,
		Model:              "gpt-5.6-sol",
		ThinkingLevel:      "high",
		PermissionMode:     "auto",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	session, err := wsp.DB.CreateSession(context.Background(), db.Session{
		Kind: "chat", Title: "thinking", AgentID: agentRow.ID,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s, wsp.ID, session.ID, recorder.requests
}

func postChatThinkingLevel(t *testing.T, s *Server, workspaceID, sessionID, level string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(chatReq{SessionID: sessionID, Message: "reply with OK", ThinkingLevel: level})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-Id", workspaceID)
	recorder := httptest.NewRecorder()
	s.Routes().ServeHTTP(recorder, req)
	return recorder
}

func TestChatEndpointCarriesPerTurnThinkingLevelToCLIRequest(t *testing.T) {
	for _, level := range []string{"low", "medium", "high", "xhigh", "max", "ultra"} {
		t.Run(level, func(t *testing.T) {
			s, workspaceID, sessionID, requests := newChatThinkingHarness(t)
			recorder := postChatThinkingLevel(t, s, workspaceID, sessionID, level)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
			}
			select {
			case got := <-requests:
				if got.CLIEffortLevel != level {
					t.Fatalf("CLIEffortLevel = %q, want %q", got.CLIEffortLevel, level)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("provider was not called")
			}
		})
	}
}

func TestChatEndpointRejectsUnknownThinkingLevelBeforeProviderCall(t *testing.T) {
	s, workspaceID, sessionID, requests := newChatThinkingHarness(t)
	recorder := postChatThinkingLevel(t, s, workspaceID, sessionID, "turbo")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	select {
	case req := <-requests:
		t.Fatalf("provider was called with %+v", req)
	default:
	}
}
