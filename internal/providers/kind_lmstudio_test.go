package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLMStudioKindRegistered pins the manifest choices that make the kind usable
// without an API key: a local server has no credential to require, and local
// inference needs a far longer request budget than a hosted endpoint.
func TestLMStudioKindRegistered(t *testing.T) {
	k, ok := lookupKind("lmstudio")
	if !ok {
		t.Fatal("lmstudio kind not registered")
	}
	man := k.Manifest()
	if man.NeedsKey {
		t.Error("NeedsKey = true; a local server has no credential to supply")
	}
	if !man.NeedsBaseURL {
		t.Error("NeedsBaseURL = false; the user must be able to point at their own port")
	}
	if !man.AllowCustomModel {
		t.Error("AllowCustomModel = false; LM Studio serves whatever the user downloaded")
	}
	if man.Transport != TransportAPI {
		t.Errorf("Transport = %q, want %q", man.Transport, TransportAPI)
	}
	if man.RequestTimeoutSecs <= 120 {
		t.Errorf("RequestTimeoutSecs = %d; local inference needs more than the hosted default", man.RequestTimeoutSecs)
	}
	f, ok := man.FieldByKey(FieldKeyAPIKey)
	if !ok {
		t.Fatal("no API key field; a reverse-proxied server still needs the option")
	}
	if f.Required {
		t.Error("API key field is Required; it must be optional for a bare local server")
	}
}

// TestLMStudioAvailableWithoutKey covers the actual blocker for local models:
// every hosted kind gates availability on a non-empty key, which would leave a
// correctly configured local instance permanently unavailable.
func TestLMStudioAvailableWithoutKey(t *testing.T) {
	k, ok := lookupKind("lmstudio")
	if !ok {
		t.Fatal("lmstudio kind not registered")
	}
	cfg := ResolvedConfig{InstanceID: "lmstudio", BaseURL: lmstudioDefaultBaseURL}
	if !k.Available(cfg) {
		t.Fatal("Available = false with no key; a local instance must be usable as-is")
	}
	p, err := k.Build(cfg)
	if err != nil {
		t.Fatalf("Build with no key: %v", err)
	}
	if p == nil {
		t.Fatal("Build returned a nil provider")
	}
}

// TestLMStudioBuildDefaultsBaseURL checks that an empty base URL falls back to
// the LM Studio port rather than the MiniMax default baked into OpenAICompat,
// which would silently send local traffic to a hosted endpoint.
func TestLMStudioBuildDefaultsBaseURL(t *testing.T) {
	k, _ := lookupKind("lmstudio")
	p, err := k.Build(ResolvedConfig{InstanceID: "lmstudio"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	oc, ok := p.(*OpenAICompat)
	if !ok {
		t.Fatalf("Build returned %T, want *OpenAICompat", p)
	}
	if oc.baseURL != strings.TrimRight(lmstudioDefaultBaseURL, "/") {
		t.Errorf("baseURL = %q, want %q", oc.baseURL, lmstudioDefaultBaseURL)
	}
}

// TestLMStudioPriceIsKnownZero pins that local usage is priced (at zero) rather
// than reported unpriced, so the budget screens do not flag it as unknown spend
// or offer a subscription-equivalent estimate for hardware the user owns.
func TestLMStudioPriceIsKnownZero(t *testing.T) {
	p, ok := PriceFor("lmstudio", "qwen3-coder-30b-a3b-instruct")
	if !ok {
		t.Fatal("PriceFor(lmstudio) ok = false; local usage must be a known zero, not unknown")
	}
	if got := p.Cost(1_000_000, 1_000_000); got != 0 {
		t.Errorf("Cost = %v, want 0", got)
	}
}

// TestLMStudioLocalWindows pins the conservative local sizing. A local server
// serves the context it was loaded with, so claiming a hosted family window
// (1M for a "deepseek" distill slug) would delay compaction past the point where
// the server rejects the request.
func TestLMStudioLocalWindows(t *testing.T) {
	cases := []struct {
		model      string
		wantWindow int
	}{
		{"qwen3-coder-30b-a3b-instruct", windowLocalQwen},
		{"qwen3-8b", windowLocalQwen},
		{"meta-llama-3.1-8b-instruct", windowLocalLlama},
		{"mistral-nemo-instruct-2407", windowLocalMistral},
		{"gemma-2-9b-it", windowLocalGemma},
		// A DeepSeek R1 distill is a Qwen model locally, NOT the hosted DeepSeek V4
		// with its 1M window: the local branch must win.
		{"deepseek-r1-distill-qwen-32b", windowLocalQwen},
	}
	for _, tc := range cases {
		if got := ContextWindowFor("lmstudio", tc.model); got != tc.wantWindow {
			t.Errorf("ContextWindowFor(lmstudio, %q) = %d, want %d", tc.model, got, tc.wantWindow)
		}
		if got := MaxOutputFor("lmstudio", tc.model); got <= 0 || got >= tc.wantWindow {
			t.Errorf("MaxOutputFor(lmstudio, %q) = %d, want >0 and < window %d", tc.model, got, tc.wantWindow)
		}
		if got := AdaptiveBudgetFraction("lmstudio", tc.model); got != 0.50 {
			t.Errorf("AdaptiveBudgetFraction(lmstudio, %q) = %v, want 0.50", tc.model, got)
		}
	}
}

// TestHostedSizingUnaffectedByLocalTable guards the blast radius: the local
// family table must only apply to local kinds, so a hosted provider serving the
// same open weights keeps its own (larger) window.
func TestHostedSizingUnaffectedByLocalTable(t *testing.T) {
	if got := ContextWindowFor("deepseek", "deepseek-v4-flash"); got != windowDeepSeek {
		t.Errorf("ContextWindowFor(deepseek, deepseek-v4-flash) = %d, want %d", got, windowDeepSeek)
	}
	// OpenRouter routing to a Qwen model is not KV-cache bound on the user machine.
	if got := ContextWindowFor("openrouter", "qwen/qwen3-coder"); got == windowLocalQwen {
		t.Error("hosted openrouter qwen picked up the local window")
	}
}

// TestLocalEndpointDetection pins that the keyless path is decided by the host,
// not the kind name: a local instance pointed at a remote authenticated proxy
// must still demand a key, and a generic openai-compat instance on loopback must
// not.
func TestLocalEndpointDetection(t *testing.T) {
	cases := []struct {
		base string
		want bool
	}{
		{"http://localhost:1234/v1", true},
		{"http://127.0.0.1:1234/v1", true},
		{"http://[::1]:1234/v1", true},
		{"http://192.168.1.50:1234/v1", true},
		{"http://10.0.0.4:1234/v1", true},
		{"https://api.openai.com/v1", false},
		{"https://openrouter.ai/api/v1", false},
		{"https://8.8.8.8/v1", false},
	}
	for _, tc := range cases {
		m := NewOpenAICompat("x", "", tc.base, "m")
		if got := m.localEndpoint(); got != tc.want {
			t.Errorf("localEndpoint(%q) = %v, want %v", tc.base, got, tc.want)
		}
	}
}

// TestRemoteStillRequiresKey guards the regression risk of relaxing the key
// guard: a hosted endpoint with no key must keep failing fast with the same
// error instead of sending an unauthenticated request.
func TestRemoteStillRequiresKey(t *testing.T) {
	m := NewOpenAICompat("openrouter", "", "https://openrouter.ai/api/v1", "some-model")
	_, err := m.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Text: "hi"}}})
	if err == nil {
		t.Fatal("Complete with no key against a hosted endpoint: want error, got nil")
	}
	if !strings.Contains(err.Error(), "missing API key") {
		t.Errorf("error = %v, want it to mention the missing API key", err)
	}
}

// oaiStubHandler serves one minimal OpenAI-compatible completion and records the
// Authorization header the client sent, including whether it sent one at all.
func oaiStubHandler(sawAuth *bool, gotAuth *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, ok := r.Header["Authorization"]
		*sawAuth = ok
		*gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "qwen3-8b",
			"choices": []map[string]any{{
				"message":       map[string]any{"role": "assistant", "content": "ok"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{"prompt_tokens": 3, "completion_tokens": 1},
		})
	}
}

// TestLocalCompleteSendsNoAuthHeader is the end-to-end shape of a keyless local
// turn: the request must carry no Authorization header at all. An empty
// "Bearer " value is rejected outright by some local servers.
func TestLocalCompleteSendsNoAuthHeader(t *testing.T) {
	var sawAuth bool
	var gotAuth string
	srv := httptest.NewServer(oaiStubHandler(&sawAuth, &gotAuth))
	defer srv.Close()

	// httptest binds 127.0.0.1, so this exercises the real local detection path.
	m := NewOpenAICompat("lmstudio", "", srv.URL, "qwen3-8b")
	resp, err := m.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Text: "hi"}}})
	if err != nil {
		t.Fatalf("Complete against a keyless local endpoint: %v", err)
	}
	if sawAuth {
		t.Errorf("request carried Authorization: %q; want the header omitted entirely", gotAuth)
	}
	if resp.Text != "ok" {
		t.Errorf("Text = %q, want %q", resp.Text, "ok")
	}
}

// TestLocalWithKeyStillSendsAuthHeader covers the reverse-proxy case the
// optional key field exists for.
func TestLocalWithKeyStillSendsAuthHeader(t *testing.T) {
	var sawAuth bool
	var gotAuth string
	srv := httptest.NewServer(oaiStubHandler(&sawAuth, &gotAuth))
	defer srv.Close()

	m := NewOpenAICompat("lmstudio", "secret", srv.URL, "qwen3-8b")
	if _, err := m.Complete(context.Background(), Request{Messages: []Message{{Role: "user", Text: "hi"}}}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret")
	}
}

// TestLMStudioInCatalog checks the kind reaches the model picker with its window
// metadata filled from the local table. Catalog() enriches by provider KIND, so
// this also proves the local sizing survives the manifest-to-DTO path.
func TestLMStudioInCatalog(t *testing.T) {
	var entry *CatalogEntry
	for i, e := range Catalog() {
		if e.ID == "lmstudio" {
			entry = &Catalog()[i]
			break
		}
	}
	if entry == nil {
		t.Fatal("lmstudio missing from Catalog(); the model picker cannot offer it")
	}
	if entry.NeedsKey {
		t.Error("catalog entry NeedsKey = true; a local server needs none")
	}
	if !entry.AllowCustomModel {
		t.Error("catalog entry AllowCustomModel = false; the user must be able to type a downloaded model id")
	}
	if len(entry.Models) == 0 {
		t.Fatal("catalog entry has no suggested models")
	}
	for _, m := range entry.Models {
		if m.ContextWindow != windowLocalQwen {
			t.Errorf("model %q ContextWindow = %d, want the local Qwen window %d", m.ID, m.ContextWindow, windowLocalQwen)
		}
		if m.MaxOutput <= 0 || m.MaxOutput >= m.ContextWindow {
			t.Errorf("model %q MaxOutput = %d, want >0 and < window %d", m.ID, m.MaxOutput, m.ContextWindow)
		}
	}
}
