package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestClaude55AgentCreateAndUpdate(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	for _, provider := range []string{"anthropic", "claude-cli", "openrouter"} {
		providerRef := provider
		if provider != "claude-cli" {
			var instance struct {
				ID string `json:"id"`
			}
			created := doJSON(t, s.Routes(), http.MethodPut, "/api/providers", upsertProviderInstanceReq{
				KindID: provider, Label: "Local Claude55 test", Secrets: map[string]string{"key": "local-test-key"},
			}, &instance)
			if created.Code != http.StatusOK || instance.ID == "" {
				t.Fatalf("provider registration: %d %s", created.Code, created.Body.String())
			}
			providerRef = instance.ID
		}
		for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5"} {
			if provider == "openrouter" {
				if model == "claude-opus-5-5" {
					model = "anthropic/claude-opus-5.5"
				} else {
					model = "anthropic/claude-sonnet-5.5"
				}
			}
			t.Run(provider+"/"+model, func(t *testing.T) {
				created := postAgent(t, s, wsp, `{"name":"Claude55","provider":"`+providerRef+`","model":"`+model+`","thinkingLevel":"high"}`)
				if created.Code != http.StatusCreated {
					t.Fatalf("create: %d %s", created.Code, created.Body.String())
				}
				var agent db.Agent
				if err := json.Unmarshal(created.Body.Bytes(), &agent); err != nil {
					t.Fatal(err)
				}
				if agent.Model != model || agent.Provider != provider || provider != "claude-cli" && agent.ProviderInstanceID != providerRef {
					t.Fatalf("model selection not persisted: %+v", agent)
				}
				for _, level := range []string{"max", "off"} {
					updated := putAgent(t, s, wsp, agent.ID, `{"thinkingLevel":"`+level+`"}`)
					if updated.Code != http.StatusOK {
						t.Fatalf("update %s: %d %s", level, updated.Code, updated.Body.String())
					}
				}
			})
		}
	}
}
