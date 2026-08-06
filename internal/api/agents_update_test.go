package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// agentUpdateFixture returns a workspace-shaped harness with a real store and a
// bus so we can watch for the model-change event.
func agentUpdateFixture(t *testing.T) (*Server, *workspace.Workspace, db.Agent, *events.Bus) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	agent, err := database.CreateAgent(ctx, db.Agent{
		Name:     "TestAgent",
		Provider: "anthropic",
		Model:    "claude-sonnet-4-20250514",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	bus := events.NewBus()
	s := &Server{
		logger: slog.New(slog.NewTextHandler(os.Stderr, nil)),
		bus:    bus,
	}
	s.runs = newChatRuns()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "test"}, DB: database}
	return s, wsp, agent, bus
}

func TestHandleUpdateAgentModelChangeEvent(t *testing.T) {
	s, wsp, agent, bus := agentUpdateFixture(t)
	subID, ch := bus.Subscribe()
	defer bus.Unsubscribe(subID)

	// Update the model from sonnet to opus — must fire a model-changed event.
	body, _ := json.Marshal(map[string]string{"model": "claude-opus-4-20250805"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", agent.ID)
	rec := httptest.NewRecorder()

	s.handleUpdateAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the event was published.
	select {
	case ev := <-ch:
		if ev.Type != "agent-model-changed" {
			t.Fatalf("event type = %q, want agent-model-changed", ev.Type)
		}
		if ev.Target["agentId"] != agent.ID {
			t.Errorf("event agentId = %q", ev.Target["agentId"])
		}
	default:
		t.Fatal("expected a model-changed event but none was published")
	}

	// Verify the response body still carries the full agent.
	var resp struct {
		Agent   db.Agent `json:"agent"`
		Warning string   `json:"warning,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Agent.Model != "claude-opus-4-20250805" {
		t.Errorf("agent model = %q, want claude-opus-4-20250805", resp.Agent.Model)
	}
}

func TestHandleUpdateAgentUnknownModelWarning(t *testing.T) {
	s, wsp, agent, _ := agentUpdateFixture(t)

	// Set model to something not in the price table.
	body, _ := json.Marshal(map[string]string{"model": "claude-mega-9"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", agent.ID)
	rec := httptest.NewRecorder()

	s.handleUpdateAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Agent   db.Agent `json:"agent"`
		Warning string   `json:"warning,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Warning == "" {
		t.Fatal("expected a warning for unknown model, got none")
	}
	if resp.Agent.Model != "claude-mega-9" {
		t.Errorf("agent model = %q", resp.Agent.Model)
	}
}

func TestHandleUpdateAgentKnownModelNoWarning(t *testing.T) {
	s, wsp, agent, _ := agentUpdateFixture(t)

	// Update to a known model — no warning.
	body, _ := json.Marshal(map[string]string{"model": "claude-haiku-4-5-20251001"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", agent.ID)
	rec := httptest.NewRecorder()

	s.handleUpdateAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Agent   db.Agent `json:"agent"`
		Warning string   `json:"warning,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Warning != "" {
		t.Errorf("expected no warning for known model, got: %q", resp.Warning)
	}
}

func TestHandleUpdateAgentNoModelChangeNoEvent(t *testing.T) {
	s, wsp, agent, bus := agentUpdateFixture(t)

	// Update name only — no model change, no event.
	body, _ := json.Marshal(map[string]string{"name": "Renamed"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	req.SetPathValue("id", agent.ID)
	rec := httptest.NewRecorder()

	s.handleUpdateAgent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// No event should be published.
	subID, ch := bus.Subscribe()
	defer bus.Unsubscribe(subID)
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event published: %+v", ev)
	default:
		// ok
	}
}
