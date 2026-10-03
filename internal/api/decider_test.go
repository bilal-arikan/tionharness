package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

func deciderRequest(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func decodeInto[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

func TestDeciderSettingsRoundTrip(t *testing.T) {
	s, _ := newWorkspaceServer(t)

	rec := deciderRequest(t, s, http.MethodGet, "/api/decider", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	view := decodeInto[deciderView](t, rec)
	if view.Config.Enabled || len(view.Authorities) != 13 || len(view.Backends) != 3 || len(view.Groups) == 0 {
		t.Errorf("initial view = %+v", view)
	}
	// The first run seeds one model: Jev through the first OpenRouter account.
	if len(view.Models) != 1 || view.Models[0].ID != "DM1" || view.Models[0].Credentials != decider.CredentialsProvider {
		t.Fatalf("seeded models = %+v", view.Models)
	}
	if view.Stats == nil || view.Recent == nil || view.Models[0].UsedBy == nil || view.ProviderCandidates[decider.OpenRouterBackendID] == nil {
		t.Error("list fields must serialise as [] not null")
	}
	if view.Status.Ready || view.Models[0].Status.Ready {
		t.Error("no OpenRouter instance is configured, the default model cannot be ready")
	}

	cfg := view.Config
	cfg.Enabled = true
	cfg.Authorities["tool-risk"] = decider.AuthorityConfig{Mode: decider.ModeOn, Threshold: 0.85}
	body, _ := json.Marshal(cfg)
	rec = deciderRequest(t, s, http.MethodPut, "/api/decider", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	view = decodeInto[deciderView](t, rec)
	if !view.Config.Enabled || view.Config.Authorities["tool-risk"].Threshold != 0.85 {
		t.Errorf("saved config = %+v", view.Config)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "decider.json")); err != nil {
		t.Errorf("config not persisted: %v", err)
	}
	if got := s.tun.Decider().Mode("tool-risk"); got != decider.ModeOn {
		t.Errorf("runtime sees tool-risk as %s, want on", got)
	}

	for body, why := range map[string]string{
		`{"authorities":{"mystery":{"mode":"on"}}}`:                     "unknown authority",
		`{"enabled":true,"surprise":1}`:                                 "unknown field (strict decode)",
		`{"defaultModel":"DM9"}`:                                        "unknown default model",
		`{"authorities":{"tool-risk":{"mode":"on","fallback":"nope"}}}`: "unknown fallback model",
	} {
		if rec := deciderRequest(t, s, http.MethodPut, "/api/decider", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", why, rec.Code, rec.Body.String())
		}
	}
}

func TestDeciderModelsCRUD(t *testing.T) {
	s, _ := newWorkspaceServer(t)

	rec := deciderRequest(t, s, http.MethodPost, "/api/decider/models",
		`{"backend":"systemone","label":"OpenJev","enabled":true,"model":"openjev-latest","credentials":"own","baseUrl":"http://127.0.0.1:1/v1","secrets":{"key":"sk-local-0000aaaa1111bbbb"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	saved := decodeInto[deciderModelSaved](t, rec)
	if saved.Model.ID != "DM2" || !saved.Model.SecretsSet["key"] || len(saved.View.Models) != 2 {
		t.Fatalf("created = %+v", saved.Model)
	}
	if strings.Contains(rec.Body.String(), "sk-local") {
		t.Error("the API echoed a model's key")
	}
	raw, err := os.ReadFile(filepath.Join(s.dataDir, "decider", "models.json"))
	if err != nil || strings.Contains(string(raw), "sk-local") {
		t.Errorf("models.json must hold the key sealed: err=%v", err)
	}

	// Update keeps the key when no secret is sent; the backend is locked.
	rec = deciderRequest(t, s, http.MethodPut, "/api/decider/models/DM2",
		`{"backend":"systemone","label":"OpenJev GPU","enabled":true,"model":"openjev-latest","credentials":"own","baseUrl":"http://127.0.0.1:1/v1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if m := decodeInto[deciderModelSaved](t, rec).Model; m.Label != "OpenJev GPU" || !m.SecretsSet["key"] {
		t.Errorf("updated = %+v", m)
	}
	if rec := deciderRequest(t, s, http.MethodPut, "/api/decider/models/DM2", `{"backend":"llm-logprobs","model":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("backend change: %d", rec.Code)
	}
	if rec := deciderRequest(t, s, http.MethodPut, "/api/decider/models/DM9", `{"backend":"systemone"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown model: %d", rec.Code)
	}
	if rec := deciderRequest(t, s, http.MethodPost, "/api/decider/models", `{"id":"DM2","backend":"systemone"}`); rec.Code != http.StatusConflict {
		t.Errorf("create over an existing id: %d", rec.Code)
	}
	if rec := deciderRequest(t, s, http.MethodPost, "/api/decider/models", `{"backend":"openrouter","credentials":"own"}`); rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "API key") {
		t.Errorf("hosted model without a key: %d %s", rec.Code, rec.Body.String())
	}

	// Bind an authority to the model, then delete it: the reference is reported
	// and cleared.
	cfg := s.tun.Decider().Config()
	cfg.Authorities["stall-judge"] = decider.AuthorityConfig{Mode: decider.ModeShadow, Threshold: 0.7, Challenger: "DM2"}
	if _, err := s.tun.Decider().Update(cfg); err != nil {
		t.Fatal(err)
	}
	rec = deciderRequest(t, s, http.MethodGet, "/api/decider", "")
	for _, m := range decodeInto[deciderView](t, rec).Models {
		if m.ID == "DM2" && (len(m.UsedBy) != 1 || m.UsedBy[0] != "stall-judge") {
			t.Errorf("usedBy = %v", m.UsedBy)
		}
	}
	rec = deciderRequest(t, s, http.MethodDelete, "/api/decider/models/DM2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	del := decodeInto[deciderModelDeleted](t, rec)
	if !del.Deleted || len(del.UsedBy) != 1 || del.View.Config.Authorities["stall-judge"].Challenger != "" || len(del.View.Models) != 1 {
		t.Errorf("delete = %+v", del)
	}

	// Testing an unreachable local model reports the failure, not a 500.
	rec = deciderRequest(t, s, http.MethodPost, "/api/decider/models", `{"backend":"llm-logprobs","label":"Nowhere","enabled":true,"model":"qwen3:4b","baseUrl":"http://127.0.0.1:1/v1","timeoutMs":500}`)
	id := decodeInto[deciderModelSaved](t, rec).Model.ID
	rec = deciderRequest(t, s, http.MethodPost, "/api/decider/models/"+id+"/test", "")
	if res := decodeInto[deciderTestResult](t, rec); rec.Code != http.StatusOK || res.OK || res.Error == "" {
		t.Errorf("test of an unreachable model = %d %+v", rec.Code, res)
	}
}

func TestDeciderTestReportsMissingInstance(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	rec := deciderRequest(t, s, http.MethodPost, "/api/decider/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("test: %d %s", rec.Code, rec.Body.String())
	}
	res := decodeInto[deciderTestResult](t, rec)
	if res.OK || !strings.Contains(res.Error, "no provider instance") {
		t.Errorf("result = %+v, want a clear no-instance failure", res)
	}
	rec = deciderRequest(t, s, http.MethodGet, "/api/decider/stats?days=3", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"statsDays":3`) {
		t.Errorf("stats: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAgentAPIRefusesDecisionModels(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	for _, model := range []string{"typesafe/jev-1.13", "openjev-latest"} {
		rec := postAgent(t, s, wsp, `{"name":"Judge","provider":"claude-cli","model":"`+model+`","thinkingLevel":"off"}`)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "decision model") {
			t.Fatalf("create with decision model %s: %d %s", model, rec.Code, rec.Body.String())
		}
	}
	created := postAgent(t, s, wsp, `{"name":"Ada","provider":"claude-cli","model":"","thinkingLevel":"off"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.Code, created.Body.String())
	}
	var a db.Agent
	_ = json.Unmarshal(created.Body.Bytes(), &a)
	rec := putAgent(t, s, wsp, a.ID, `{"model":"~typesafe/jev-latest"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "decision model") {
		t.Fatalf("update to a decision model: %d %s", rec.Code, rec.Body.String())
	}
}
