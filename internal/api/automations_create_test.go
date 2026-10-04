package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestCreateAutomationPromptRequirements(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	agent, err := wsp.DB.CreateAgent(t.Context(), db.Agent{Name: "agent"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		body       map[string]any
		wantStatus int
		wantError  string
	}{
		{
			name:       "archive without prompt",
			body:       map[string]any{"triggerKind": db.TriggerBoard, "boardAction": db.BoardActionArchive},
			wantStatus: http.StatusCreated,
		},
		{
			name: "move without prompt",
			body: map[string]any{
				"triggerKind":      db.TriggerBoard,
				"boardAction":      db.BoardActionMove,
				"boardToState":     "review",
				"boardMoveToState": "done",
			},
			wantStatus: http.StatusCreated,
		},
		{
			name:       "spawn without prompt",
			body:       map[string]any{"triggerKind": db.TriggerBoard, "boardAction": db.BoardActionSpawn, "targetAgentId": agent.ID},
			wantStatus: http.StatusBadRequest,
			wantError:  "promptTemplate is required",
		},
		{
			// The token-spend trigger was removed (2026-10-04): it must be refused as
			// an unknown kind, even with a valid target and prompt.
			name: "retired token kind",
			body: map[string]any{
				"triggerKind": "token", "tokenThreshold": 100000,
				"targetAgentId": agent.ID, "promptTemplate": "run",
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "unknown triggerKind",
		},
		{
			name:       "retired token kind without target",
			body:       map[string]any{"triggerKind": "token", "promptTemplate": "run"},
			wantStatus: http.StatusBadRequest,
			wantError:  "unknown triggerKind",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, s.Routes(), http.MethodPost, "/api/automations", tc.body, nil)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantError != "" && !strings.Contains(rec.Body.String(), tc.wantError) {
				t.Fatalf("body = %q, want error %q", rec.Body.String(), tc.wantError)
			}
		})
	}
}
