package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestCodexGPT6AgentCreateAndUpdate(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	for _, model := range []string{"gpt-6.1-sol", "gpt-6-astra", "gpt-6-sol", "gpt-6-luna"} {
		t.Run(model, func(t *testing.T) {
			created := postAgent(t, s, wsp, `{"name":"GPT6","provider":"codex-cli","model":"`+model+`","thinkingLevel":"high"}`)
			if created.Code != http.StatusCreated {
				t.Fatalf("create: %d %s", created.Code, created.Body.String())
			}
			var agent db.Agent
			if err := json.Unmarshal(created.Body.Bytes(), &agent); err != nil {
				t.Fatal(err)
			}
			effort := "ultra"
			if model == "gpt-6-luna" {
				effort = "max"
			}
			updated := putAgent(t, s, wsp, agent.ID, `{"thinkingLevel":"`+effort+`"}`)
			if updated.Code != http.StatusOK {
				t.Fatalf("update: %d %s", updated.Code, updated.Body.String())
			}
			rejected := putAgent(t, s, wsp, agent.ID, `{"thinkingLevel":"off"}`)
			if rejected.Code != http.StatusBadRequest {
				t.Fatalf("unsupported off: %d %s", rejected.Code, rejected.Body.String())
			}
			if model == "gpt-6-luna" {
				rejected = putAgent(t, s, wsp, agent.ID, `{"thinkingLevel":"ultra"}`)
				if rejected.Code != http.StatusBadRequest {
					t.Fatalf("unsupported ultra: %d %s", rejected.Code, rejected.Body.String())
				}
			}
		})
	}
}
