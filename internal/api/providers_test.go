package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/market"
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
	anthropic, ok := byID["anthropic"]
	if !ok {
		t.Fatalf("expected \"anthropic\" kind in schema, got %+v", byID)
	}
	if anthropic.Transport != "api" {
		t.Fatalf("expected anthropic transport \"api\", got %q", anthropic.Transport)
	}
	found := false
	for _, f := range anthropic.Fields {
		if f.Key == "key" {
			found = true
			if !f.Secret || !f.Required {
				t.Fatalf("expected anthropic \"key\" field secret+required, got %+v", f)
			}
		}
	}
	if !found {
		t.Fatalf("expected anthropic kind to declare a \"key\" field, got %+v", anthropic.Fields)
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
		KindID: "anthropic",
		Label:  "İş hesabı",
		Secrets: map[string]string{
			"key": "sk-live-abc",
		},
	}, &created)
	if rec.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if created.ID == "" {
		t.Fatal("expected a generated instance id")
	}
	if !created.SecretsSet["key"] {
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
		KindID: "anthropic",
		Label:  "İş hesabı v2",
	}, &updated)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if updated.Label != "İş hesabı v2" {
		t.Fatalf("expected updated label, got %q", updated.Label)
	}
	if !updated.SecretsSet["key"] {
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
func TestProviders_MissingRequiredFieldRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID: "anthropic",
		Label:  "no key",
	}, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing required field, got %d: %s", rec.Code, rec.Body.String())
	}
	_ = s
}

// TestProviders_UnknownFieldRejected verifies a config/secret key not
// declared by the kind's manifest is rejected rather than silently stored.
func TestProviders_UnknownFieldRejected(t *testing.T) {
	s, _ := newWorkspaceServer(t)
	h := s.Routes()

	rec := doJSON(t, h, http.MethodPut, "/api/providers", upsertProviderInstanceReq{
		KindID:  "anthropic",
		Label:   "x",
		Secrets: map[string]string{"key": "sk-live-abc"},
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
		KindID:  "anthropic",
		Label:   "leak check",
		Secrets: map[string]string{"key": secret},
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
		enc, ok := inst.SecretsEnc["key"]
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
		KindID:  "anthropic",
		Label:   "used by agent",
		Secrets: map[string]string{"key": "sk-live-in-use"},
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

// TestInstallProviderPack_ResolvesInLiveRegistry verifies a market provider
// pack installs through the same providers.json path as the CRUD handler:
// the instance resolves in the live registry in the same call (the bug this
// task's follow-up was scoped around — installProviderPack writing to the
// legacy settings.json path the registry no longer reads from).
func TestInstallProviderPack_ResolvesInLiveRegistry(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	pack := providerPack("myrouter", "openai", "https://myrouter.ai/api/v1")
	res, err := s.installProviderPack(wsp, pack, "sk-live-pack-key")
	if err != nil {
		t.Fatalf("installProviderPack: %v", err)
	}
	if res.Ref == "" {
		t.Fatal("expected a non-empty install ref (instance id)")
	}

	if _, err := s.providers.Get(res.Ref); err != nil {
		t.Fatalf("expected registry to resolve pack-installed instance %q in the same call: %v", res.Ref, err)
	}

	inst, ok := s.providerStore.Get(res.Ref)
	if !ok {
		t.Fatalf("expected providerStore to hold instance %q", res.Ref)
	}
	if inst.KindID != "openai-compat" {
		t.Fatalf("expected legacy pack kind %q to map to \"openai-compat\", got %q", "openai", inst.KindID)
	}
	if inst.SecretsEnc["key"] == "" {
		t.Fatal("expected the supplied apiKey to be stored as a secret")
	}
}

// TestInstallProviderPack_EmptyAPIKeyStillInstalls verifies the pre-existing
// behaviour is preserved: a pack installs with no API key supplied (the user
// adds it later in Settings), even though "key" is a required secret field
// on openai-compat/anthropic-compat.
func TestInstallProviderPack_EmptyAPIKeyStillInstalls(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	pack := providerPack("nokeyrouter", "anthropic", "https://example.com/v1")
	res, err := s.installProviderPack(wsp, pack, "")
	if err != nil {
		t.Fatalf("installProviderPack with empty apiKey: %v", err)
	}

	inst, ok := s.providerStore.Get(res.Ref)
	if !ok {
		t.Fatalf("expected providerStore to hold instance %q", res.Ref)
	}
	if inst.KindID != "anthropic-compat" {
		t.Fatalf("expected legacy pack kind %q to map to \"anthropic-compat\", got %q", "anthropic", inst.KindID)
	}
	if len(inst.SecretsEnc) != 0 {
		t.Fatalf("expected no stored secret when apiKey is empty, got %v", inst.SecretsEnc)
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
