package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/settings"
)

// doJSON issues an HTTP request against h with an optional JSON body and
// decodes the JSON response into out (skipped if out is nil).
func doJSON(t testing.TB, h http.Handler, method, path string, body any, out any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("decode response (status %d, body %s): %v", rec.Code, rec.Body.String(), err)
		}
	}
	return rec
}

func TestProviders_CLIHomeIsolationOnCreateOnly(t *testing.T) {
	for _, kind := range []string{"claude-cli", "codex-cli"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := newWorkspaceServer(t)
			h := s.Routes()
			var created struct {
				ID     string            `json:"id"`
				Config map[string]string `json:"config"`
			}
			rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{KindID: kind, Label: kind}, &created)
			if rec.Code != http.StatusOK {
				t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
			}
			want := filepath.Join(filepath.Dir(s.providerStore.Path()), "provider-homes", created.ID)
			if created.Config["configDir"] != want {
				t.Fatalf("configDir = %q, want %q", created.Config["configDir"], want)
			}
			if info, err := os.Stat(want); err != nil || !info.IsDir() {
				t.Fatalf("isolated home not created: %v", err)
			}

			legacy, err := s.providerStore.Upsert(settings.ProviderInstanceInput{ID: "legacy-" + kind, KindID: kind, Config: map[string]string{"configDir": ""}})
			if err != nil {
				t.Fatalf("seed legacy instance: %v", err)
			}
			rec = doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{ID: legacy.ID, KindID: kind, Label: "updated", Config: map[string]string{"configDir": ""}}, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
			}
			stored, ok := s.providerStore.Get(legacy.ID)
			if !ok || stored.Config["configDir"] != "" {
				t.Fatalf("legacy configDir was rewritten: %+v", stored.Config)
			}
		})
	}
}

func TestProviderAuthRoutes(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		method     string
		pathSuffix string
		wantStatus int
	}{
		{name: "claude status", kind: "claude-cli", method: http.MethodGet, pathSuffix: "/auth", wantStatus: http.StatusOK},
		{name: "codex status", kind: "codex-cli", method: http.MethodGet, pathSuffix: "/auth", wantStatus: http.StatusOK},
		{name: "claude route on codex", kind: "codex-cli", method: http.MethodPost, pathSuffix: "/auth/oauth/start", wantStatus: http.StatusBadRequest},
		{name: "codex route on claude", kind: "claude-cli", method: http.MethodGet, pathSuffix: "/auth/device/status", wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			config := map[string]string{"configDir": filepath.Join(t.TempDir(), "auth-home")}
			inst, err := s.providerStore.Upsert(settings.ProviderInstanceInput{ID: "auth-test", KindID: tt.kind, Config: config})
			if err != nil {
				t.Fatalf("seed provider: %v", err)
			}
			req := httptest.NewRequest(tt.method, "/api/providers/"+inst.ID+tt.pathSuffix, nil)
			req.Header.Set("X-Workspace-Id", wsp.ID)
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.pathSuffix == "/auth" && tt.wantStatus == http.StatusOK {
				var got providerAuthDTO
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode auth status: %v", err)
				}
				if got.Kind != tt.kind || got.HomeDir != config["configDir"] {
					t.Fatalf("auth status = %+v", got)
				}
			}
		})
	}

	registered := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/auth"},
		{http.MethodPost, "/auth/oauth/start"},
		{http.MethodPost, "/auth/oauth/complete"},
		{http.MethodPost, "/auth/oauth/loopback/start"},
		{http.MethodGet, "/auth/oauth/loopback/status"},
		{http.MethodPost, "/auth/device/start"},
		{http.MethodGet, "/auth/device/status"},
		{http.MethodPost, "/auth/device/cancel"},
		{http.MethodPost, "/auth/api-key"},
	}
	for _, route := range registered {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			req := httptest.NewRequest(route.method, "/api/providers/missing"+route.path, nil)
			req.Header.Set("X-Workspace-Id", wsp.ID)
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestProviderAuthHomeResolution(t *testing.T) {
	for _, kind := range []string{"claude-cli", "codex-cli"} {
		t.Run(kind+" app-global fallback", func(t *testing.T) {
			s, wsp := newWorkspaceServer(t)
			inst, err := s.providerStore.Upsert(settings.ProviderInstanceInput{
				ID: kind + "-shared", KindID: kind, Config: map[string]string{"configDir": ""},
			})
			if err != nil {
				t.Fatalf("seed provider: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/providers/"+inst.ID+"/auth", nil)
			req.SetPathValue("id", inst.ID)
			rec := httptest.NewRecorder()
			target, ok := s.resolveProviderAuthTarget(rec, req, kind)
			if !ok {
				t.Fatalf("resolve failed: status=%d body=%s", rec.Code, rec.Body.String())
			}
			want := filepath.Join(s.dataDir, strings.TrimSuffix(kind, "-cli")+"-home")
			if target.homeDir != want {
				t.Fatalf("home = %q, want app-global %q", target.homeDir, want)
			}
			workspaceHome := filepath.Join(wsp.DataDir, strings.TrimSuffix(kind, "-cli")+"-home")
			if target.homeDir == workspaceHome {
				t.Fatalf("home unexpectedly resolved under workspace: %q", target.homeDir)
			}
		})
	}

	t.Run("explicit configDir unchanged", func(t *testing.T) {
		s, _ := newWorkspaceServer(t)
		explicit := filepath.Join(t.TempDir(), "dedicated-home")
		inst, err := s.providerStore.Upsert(settings.ProviderInstanceInput{
			ID: "claude-dedicated", KindID: "claude-cli", Config: map[string]string{"configDir": explicit},
		})
		if err != nil {
			t.Fatalf("seed provider: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/providers/"+inst.ID+"/auth", nil)
		req.SetPathValue("id", inst.ID)
		target, ok := s.resolveProviderAuthTarget(httptest.NewRecorder(), req, "claude-cli")
		if !ok {
			t.Fatal("resolve failed")
		}
		if target.homeDir != explicit {
			t.Fatalf("home = %q, want explicit %q", target.homeDir, explicit)
		}
	})

	t.Run("missing app data dir returns error", func(t *testing.T) {
		s, _ := newWorkspaceServer(t)
		s.dataDir = ""
		inst, err := s.providerStore.Upsert(settings.ProviderInstanceInput{
			ID: "codex-no-data", KindID: "codex-cli", Config: map[string]string{"configDir": ""},
		})
		if err != nil {
			t.Fatalf("seed provider: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/providers/"+inst.ID+"/auth", nil)
		req.SetPathValue("id", inst.ID)
		rec := httptest.NewRecorder()
		if _, ok := s.resolveProviderAuthTarget(rec, req, "codex-cli"); ok {
			t.Fatal("resolve unexpectedly succeeded")
		}
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

// TestProviderKinds_SchemaCoversEveryRegisteredKind verifies GET
// /api/provider-kinds walks the live kind registry rather than a hard-coded
// list, and that every field the anthropic kind declares (a required secret
// "key") round-trips into the DTO.
func TestProviderKinds_SchemaCoversEveryRegisteredKind(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	var kinds []providerKindDTO
	rec := doJSON(t, h, http.MethodGet, "/api/provider-kinds", nil, &kinds)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(kinds) == 0 {
		t.Fatal("expected at least one registered provider kind")
	}

	byID := map[string]providerKindDTO{}
	for _, k := range kinds {
		byID[k.ID] = k
	}
	cli, ok := byID["claude-cli"]
	if !ok {
		t.Fatalf("expected \"claude-cli\" kind in schema, got %+v", byID)
	}
	if cli.Transport != "cli" {
		t.Fatalf("expected claude-cli transport \"cli\", got %q", cli.Transport)
	}
	found := false
	for _, f := range cli.Fields {
		if f.Key == "authToken" {
			found = true
			if !f.Secret {
				t.Fatalf("expected claude-cli \"authToken\" field to be secret, got %+v", f)
			}
		}
	}
	if !found {
		t.Fatalf("expected claude-cli kind to declare an \"authToken\" field, got %+v", cli.Fields)
	}
}

// TestProviders_CRUDRoundTrip exercises create → get → list → update → delete
// through the HTTP handlers, and confirms applySettings pushed the create
// into the live registry immediately (the bug this task was scoped around:
// a provider written to providers.json but invisible to agents).
func TestProviders_CRUDRoundTrip(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	created := struct {
		ID         string          `json:"id"`
		KindID     string          `json:"kindId"`
		Label      string          `json:"label"`
		SecretsSet map[string]bool `json:"secretsSet"`
	}{}
	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID: "claude-cli",
		Label:  "İş hesabı",
		Secrets: map[string]string{
			"authToken": "sk-live-abc",
		},
	}, &created)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if created.ID == "" {
		t.Fatal("expected a generated instance id")
	}
	if !created.SecretsSet["authToken"] {
		t.Fatalf("expected secretsSet[key]=true, got %+v", created.SecretsSet)
	}

	// The registry must see it immediately — no separate reload step.
	if _, err := s.providers.Get(created.ID); err != nil {
		t.Fatalf("expected registry to resolve newly created instance %q: %v", created.ID, err)
	}

	// GET single.
	var fetched struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	}
	rec = doJSON(t, h, http.MethodGet, "/api/providers/"+created.ID, nil, &fetched)
	if rec.Code != http.StatusOK || fetched.ID != created.ID {
		t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// LIST includes it.
	var list []struct {
		ID string `json:"id"`
	}
	rec = doJSON(t, h, http.MethodGet, "/api/providers", nil, &list)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}
	seen := false
	for _, p := range list {
		if p.ID == created.ID {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("expected created instance %q in list, got %+v", created.ID, list)
	}

	// UPDATE label without resending the secret — key must be preserved.
	var updated struct {
		ID         string          `json:"id"`
		Label      string          `json:"label"`
		SecretsSet map[string]bool `json:"secretsSet"`
	}
	rec = doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		ID:     created.ID,
		KindID: "claude-cli",
		Label:  "İş hesabı v2",
	}, &updated)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if updated.Label != "İş hesabı v2" {
		t.Fatalf("expected updated label, got %q", updated.Label)
	}
	if !updated.SecretsSet["authToken"] {
		t.Fatal("expected key to survive an update that omits it")
	}

	// DELETE.
	var delResp deleteProviderResp
	rec = doJSON(t, h, http.MethodDelete, "/api/providers/"+created.ID, nil, &delResp)
	if rec.Code != http.StatusOK || !delResp.Deleted {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(delResp.AffectedAgents) != 0 {
		t.Fatalf("expected no affected agents, got %v", delResp.AffectedAgents)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/providers/"+created.ID, nil, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rec.Code)
	}
	if _, err := s.providers.Get(created.ID); err == nil {
		t.Fatal("expected registry to drop the deleted instance immediately")
	}
}

// TestProviders_UnknownKindRejected verifies an unknown kindId returns 400
// rather than being silently accepted.
func TestProviders_UnknownKindRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID: "not-a-real-kind",
		Label:  "x",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown kind, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = s
}

// TestProviders_MissingRequiredFieldRejected verifies a kind's required
// secret field (anthropic's "key") must be present on create.
func TestProviders_RemovedKindRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID: "openrouter", // removed with the HTTP transports (_Docs/95)
		Label:  "legacy kind",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unregistered kind, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = s
}

// TestProviders_UnknownFieldRejected verifies a config/secret key not
// declared by the kind's manifest is rejected rather than silently stored.
func TestProviders_UnknownFieldRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID:  "claude-cli",
		Label:   "x",
		Secrets: map[string]string{"authToken": "sk-live-abc"},
		Config:  map[string]string{"totallyBogusField": "y"},
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown config field, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestProviders_SecretsNeverLeakInResponses walks the list/get/create/update
// JSON responses byte-for-byte and fails if the plaintext key or its
// encrypted-at-rest form ever appears.
func TestProviders_SecretsNeverLeakInResponses(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	const secret = "sk-live-do-not-leak-987654321"
	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID:  "claude-cli",
		Label:   "leak check",
		Secrets: map[string]string{"authToken": secret},
	}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("create response leaks plaintext secret: %s", rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet, "/api/providers", nil, nil)
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("list response leaks plaintext secret: %s", rec.Body.String())
	}

	// The stored ciphertext must not leak either (basic sanity: it must not
	// equal the plaintext prefix used by the real AES-GCM cipher output, and
	// must not be echoed at all).
	list := s.providerStore.List()
	if len(list) == 0 {
		t.Fatal("expected at least one stored instance")
	}
	for _, inst := range list {
		enc, ok := inst.SecretsEnc["authToken"]
		if !ok || enc == "" {
			continue
		}
		if strings.Contains(rec.Body.String(), enc) {
			t.Fatalf("list response leaks encrypted secret ciphertext: %s", rec.Body.String())
		}
	}
}

// TestProviders_DeleteReportsAffectedAgents verifies deleting an in-use
// provider instance still deletes it (plan is silent on blocking) but the
// response names every agent left pointing at it — never a silent swallow.
func TestProviders_DeleteReportsAffectedAgents(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	h := s.Routes()

	var created struct {
		ID string `json:"id"`
	}
	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID:  "claude-cli",
		Label:   "used by agent",
		Secrets: map[string]string{"authToken": "sk-live-in-use"},
	}, &created)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}

	ctx := context.Background()
	agent1, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Bound-1", ProviderInstanceID: created.ID})
	if err != nil {
		t.Fatalf("create agent 1: %v", err)
	}
	agent2, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Bound-2", ProviderInstanceID: created.ID})
	if err != nil {
		t.Fatalf("create agent 2: %v", err)
	}
	// An unrelated agent must not show up as affected.
	if _, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Unrelated"}); err != nil {
		t.Fatalf("create agent 3: %v", err)
	}

	var delResp deleteProviderResp
	rec = doJSON(t, h, http.MethodDelete, "/api/providers/"+created.ID, nil, &delResp)
	if rec.Code != http.StatusOK || !delResp.Deleted {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := map[string]bool{}
	for _, id := range delResp.AffectedAgents {
		got[id] = true
	}
	if !got[agent1.ID] || !got[agent2.ID] {
		t.Fatalf("expected both bound agents reported, got %v (want %s, %s)", delResp.AffectedAgents, agent1.ID, agent2.ID)
	}
	if len(delResp.AffectedAgents) != 2 {
		t.Fatalf("expected exactly 2 affected agents, got %v", delResp.AffectedAgents)
	}
}

// providerPack builds a minimal market.Pack carrying a ProviderPayload, the
// shape installProviderPack consumes.
func providerPack(id, kind, baseURL string) market.Pack {
	return market.Pack{
		Schema: market.SchemaV1,
		ID:     "provider." + id,
		Kind:   market.KindProvider,
		Name:   "Test Provider " + id,
		Payload: market.Payload{
			Provider: &market.ProviderPayload{
				Label:   "Test Provider " + id,
				Kind:    kind,
				BaseURL: baseURL,
			},
		},
	}
}

// TestInstallProviderPack_Rejected pins the post-removal contract: TionHarness
// has no HTTP provider kinds any more, so a provider pack of either legacy kind
// is refused with the opencode hint and nothing lands in providers.json.
func TestInstallProviderPack_Rejected(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	pack := providerPack("myrouter", "openai", "https://myrouter.ai/api/v1")
	if _, err := s.installProviderPack(wsp, pack, "sk-live-pack-key"); err == nil || !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("openai pack must be rejected with the opencode hint, got %v", err)
	}
	if _, ok := s.providerStore.Get(providerIDFromPack(pack)); ok {
		t.Fatal("a rejected pack must not create a provider instance")
	}
}

// TestInstallProviderPack_AnthropicKindRejected pins the post-removal contract:
// the Anthropic-protocol HTTP kinds no longer exist, so an "anthropic" pack is
// refused with a clear message instead of landing an instance of a kind the
// registry cannot build.
func TestInstallProviderPack_AnthropicKindRejected(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	pack := providerPack("legacyanthropic", "anthropic", "https://example.com/v1")
	if _, err := s.installProviderPack(wsp, pack, "sk-x"); err == nil || !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("anthropic pack must be rejected with the opencode hint, got %v", err)
	}
}

// TestInstallProviderPack_MissingBaseURLRejected verifies a pack missing its
// required baseUrl payload fails the install with a clear error rather than
// landing a broken instance.
func TestInstallProviderPack_MissingBaseURLRejected(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	pack := providerPack("nobase", "openai", "")
	if _, err := s.installProviderPack(wsp, pack, "sk-live"); err == nil {
		t.Fatal("expected an error installing a provider pack with an empty baseUrl")
	}
}
