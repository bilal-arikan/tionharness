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

func TestDeciderSettingsRoundTrip(t *testing.T) {
	s, _ := newWorkspaceServer(t)

	rec := deciderRequest(t, s, http.MethodGet, "/api/decider", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var view deciderView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view.Config.Enabled || view.Config.Model != decider.JevModel || len(view.Sites) != 4 || len(view.Backends) == 0 {
		t.Errorf("initial view = %+v", view)
	}
	if view.Candidates == nil || view.Stats == nil || view.Recent == nil {
		t.Error("list fields must serialise as [] not null")
	}
	if view.Status.Ready {
		t.Error("no OpenRouter instance is configured, the hub cannot be ready")
	}

	cfg := view.Config
	cfg.Enabled = true
	cfg.Sites[decider.SiteToolRisk] = decider.SiteConfig{Mode: decider.ModeOn, Threshold: 0.85}
	body, _ := json.Marshal(cfg)
	rec = deciderRequest(t, s, http.MethodPut, "/api/decider", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Config.Enabled || view.Config.Sites[decider.SiteToolRisk].Threshold != 0.85 {
		t.Errorf("saved config = %+v", view.Config)
	}
	if _, err := os.Stat(filepath.Join(s.dataDir, "decider.json")); err != nil {
		t.Errorf("config not persisted: %v", err)
	}
	if got := s.tun.Decider().Mode(decider.SiteToolRisk); got != decider.ModeOn {
		t.Errorf("runtime sees tool-risk as %s, want on", got)
	}

	rec = deciderRequest(t, s, http.MethodPut, "/api/decider", `{"sites":{"mystery":{"mode":"on"}}}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown site: %d %s", rec.Code, rec.Body.String())
	}
	rec = deciderRequest(t, s, http.MethodPut, "/api/decider", `{"enabled":true,"surprise":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown field must be refused (strict decode): %d", rec.Code)
	}
}

func TestDeciderTestReportsMissingInstance(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	rec := deciderRequest(t, s, http.MethodPost, "/api/decider/test", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("test: %d %s", rec.Code, rec.Body.String())
	}
	var res deciderTestResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
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
	rec := postAgent(t, s, wsp, `{"name":"Judge","provider":"claude-cli","model":"typesafe/jev-1.13","thinkingLevel":"off"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "decision model") {
		t.Fatalf("create with a decision model: %d %s", rec.Code, rec.Body.String())
	}
	created := postAgent(t, s, wsp, `{"name":"Ada","provider":"claude-cli","model":"","thinkingLevel":"off"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", created.Code, created.Body.String())
	}
	var a db.Agent
	_ = json.Unmarshal(created.Body.Bytes(), &a)
	rec = putAgent(t, s, wsp, a.ID, `{"model":"~typesafe/jev-latest"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "decision model") {
		t.Fatalf("update to a decision model: %d %s", rec.Code, rec.Body.String())
	}
}
