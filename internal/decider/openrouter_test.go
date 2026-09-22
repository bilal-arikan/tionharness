package decider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// noRetryPause makes the one-retry path instant for the duration of a test.
func noRetryPause(t *testing.T) {
	t.Helper()
	prev := retryPause
	retryPause = 0
	t.Cleanup(func() { retryPause = prev })
}

func bearer(key string) func(http.Header) {
	return func(h http.Header) { h.Set("Authorization", "Bearer "+key) }
}

// liveShapedResponse is the body OpenRouter returned for a four-question probe
// on 2026-09-21 (typesafe/jev-1.13), trimmed to the fields the client reads.
const liveShapedResponse = `{
  "model": "typesafe/jev-1.13-20260917",
  "answers": {
    "needs_approval": {"type": "noul", "noul": 0.96},
    "risk": {"type": "score", "score": 2, "legend": {"0": "Read-only", "1": "Local", "2": "Destructive"},
             "probabilities": {"0": 0, "1": 0, "2": 1}, "confidence": 1},
    "model_tier": {"type": "choice", "choice": "balanced",
                   "probabilities": {"frontier": 0, "balanced": 0.77, "fast": 0.23}, "confidence": 0.66}
  },
  "usage": {"input_tokens": 606, "output_tokens": 92, "cost": 2.5452e-05},
  "id": "gen-dec-1790022255-abc",
  "provider": "TypeSafe"
}`

func TestOpenRouterDecideRequestAndResponseShape(t *testing.T) {
	var got map[string]any
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, liveShapedResponse)
	}))
	defer srv.Close()

	d, err := openRouterBackend{}.New(Endpoint{InstanceID: "PRV1", Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Authorize: bearer("k1")}, ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := d.Decide(context.Background(), Request{
		State: map[string]any{"command": "git push --force"},
		Questions: map[string]Question{
			"needs_approval": Noul("approve?", "destructive", "harmless"),
			"risk":           Score("how risky?", "Read-only", "Local", "Destructive"),
			"model_tier":     Choice("tier?", map[string]string{"fast": "f", "balanced": "b", "frontier": "x"}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/api/alpha/decisions" {
		t.Errorf("path = %q, want /api/alpha/decisions", path)
	}
	if auth != "Bearer k1" {
		t.Errorf("authorization = %q", auth)
	}
	if got["model"] != JevModel {
		t.Errorf("model = %v, want the pinned default %q", got["model"], JevModel)
	}
	qs := got["questions"].(map[string]any)
	noul := qs["needs_approval"].(map[string]any)
	if c := noul["criteria"].(map[string]any); c["true"] != "destructive" || c["false"] != "harmless" {
		t.Errorf("noul criteria = %v", c)
	}
	if lv := qs["risk"].(map[string]any)["criteria"].([]any); len(lv) != 3 || lv[2] != "Destructive" {
		t.Errorf("score criteria = %v", lv)
	}
	if opts := qs["model_tier"].(map[string]any)["criteria"].(map[string]any); opts["balanced"] != "b" {
		t.Errorf("choice criteria = %v", opts)
	}

	if a := resp.Answers["needs_approval"]; a.Type != QuestionNoul || a.Probability != 0.96 {
		t.Errorf("noul answer = %+v", a)
	}
	if a := resp.Answers["risk"]; a.Level() != 2 || a.Confidence != 1 {
		t.Errorf("score answer = %+v", a)
	}
	if a := resp.Answers["model_tier"]; a.Choice != "balanced" || a.Strength() != 0.77 {
		t.Errorf("choice answer = %+v", a)
	}
	if resp.Model != JevModel || resp.ServedModel != "typesafe/jev-1.13-20260917" {
		t.Errorf("model = %q served = %q; usage must be billed under the requested id", resp.Model, resp.ServedModel)
	}
	if resp.Usage.InputTokens != 606 || resp.Usage.CostUSD != 2.5452e-05 || resp.BillingProvider != "openrouter" {
		t.Errorf("usage = %+v billing = %q", resp.Usage, resp.BillingProvider)
	}
}

func TestOpenRouterErrorEnvelopeAndRetry(t *testing.T) {
	noRetryPause(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		switch {
		case strings.Contains(r.Header.Get("X-Case"), "flaky") && n == 1:
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"overloaded","code":503}}`)
		case strings.Contains(r.Header.Get("X-Case"), "flaky"):
			_, _ = io.WriteString(w, `{"answers":{"q":{"type":"noul","noul":0.2}},"usage":{"input_tokens":10}}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"[\n  {\n \"path\": [\"questions\",\"q\",\"criteria\",\"false\"],\n \"message\": \"Invalid input\"\n  }\n]","code":400}}`)
		}
	}))
	defer srv.Close()
	ask := func(caseName string) error {
		ep := Endpoint{Kind: "openrouter", BaseURL: srv.URL + "/api/v1", Authorize: func(h http.Header) { h.Set("X-Case", caseName) }}
		d, err := openRouterBackend{}.New(ep, ClientOptions{})
		if err != nil {
			t.Fatal(err)
		}
		_, err = d.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{"q": Noul("?", "", "")}})
		return err
	}

	if err := ask("flaky"); err != nil {
		t.Fatalf("a 503 followed by a success must succeed after one retry: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2 (one retry)", calls.Load())
	}

	calls.Store(0)
	err := ask("bad")
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 400 {
		t.Fatalf("err = %v, want an HTTPError 400", err)
	}
	if calls.Load() != 1 {
		t.Errorf("a 400 was retried (%d calls)", calls.Load())
	}
	if !strings.HasPrefix(err.Error(), "openrouter-decisions HTTP 400: ") || strings.Contains(err.Error(), "\n") {
		t.Errorf("error text = %q, want the one-line classified shape", err.Error())
	}
}

func TestOpenRouterRejectsIncompleteAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"answers":{"a":{"type":"choice","choice":"x"}}}`)
	}))
	defer srv.Close()
	d, _ := openRouterBackend{}.New(Endpoint{Kind: "openrouter", BaseURL: srv.URL + "/v1"}, ClientOptions{})
	_, err := d.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{
		"a": Noul("?", "", ""),
		"b": Noul("?", "", ""),
	}})
	if err == nil {
		t.Fatal("expected an error for a type mismatch / missing answer")
	}
}

func TestOpenRouterAttemptTimeout(t *testing.T) {
	noRetryPause(t)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	d, _ := openRouterBackend{}.New(Endpoint{Kind: "openrouter", BaseURL: srv.URL + "/v1"}, ClientOptions{Timeout: 50 * time.Millisecond})
	start := time.Now()
	_, err := d.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{"q": Noul("?", "", "")}})
	if err == nil || !isTimeout(err) {
		t.Fatalf("err = %v, want a timeout", err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("a hung endpoint held the caller for %v", el)
	}
}

func TestDecisionsURL(t *testing.T) {
	cases := map[string]string{
		"":                               "https://openrouter.ai/api/alpha/decisions",
		"https://openrouter.ai/api/v1":   "https://openrouter.ai/api/alpha/decisions",
		"https://openrouter.ai/api/v1/ ": "https://openrouter.ai/api/alpha/decisions",
		"http://127.0.0.1:9/proxy/v1":    "http://127.0.0.1:9/proxy/alpha/decisions",
	}
	for in, want := range cases {
		got, err := DecisionsURL(in)
		if err != nil || got != want {
			t.Errorf("DecisionsURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := DecisionsURL("https://proxy.corp/openrouter"); err == nil {
		t.Error("a base without /v1 must be refused, not guessed")
	}
}

func TestOpenRouterAccepts(t *testing.T) {
	b := openRouterBackend{}
	if !b.Accepts("openrouter", "") {
		t.Error("openrouter kind refused")
	}
	if !b.Accepts("openai-compat", "https://openrouter.ai/api/v1") {
		t.Error("openai-compat pointing at openrouter.ai refused")
	}
	if b.Accepts("openai-compat", "https://api.example.com/v1") || b.Accepts("anthropic", "") {
		t.Error("an instance that cannot reach OpenRouter was accepted")
	}
}

func TestIsDecisionModel(t *testing.T) {
	for _, m := range []string{JevModel, JevLatestModel, "TypeSafe/jev-2", " typesafe/jev-1.13 "} {
		if !IsDecisionModel(m) {
			t.Errorf("IsDecisionModel(%q) = false", m)
		}
	}
	for _, m := range []string{"", "anthropic/claude-sonnet-5", "openai/gpt-5.5"} {
		if IsDecisionModel(m) {
			t.Errorf("IsDecisionModel(%q) = true", m)
		}
	}
}
