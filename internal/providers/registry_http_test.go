package providers

import (
	"net/http"
	"strings"
	"testing"
)

func TestRegistryHTTPAccess(t *testing.T) {
	r := NewRegistry()
	r.SetInstances([]Instance{
		{ID: "PRV1", KindID: "openrouter", Enabled: true, Values: map[string]string{FieldKeyAPIKey: "sk-or-test", FieldKeyBaseURL: "https://openrouter.ai/api/v1"}},
		{ID: "PRV2", KindID: "openrouter", Enabled: false, Values: map[string]string{FieldKeyAPIKey: "sk-or-test"}},
		{ID: "PRV3", KindID: "openrouter", Enabled: true, Values: map[string]string{}},
		{ID: "claude-cli", KindID: "claude-cli", Enabled: true, Values: map[string]string{}},
	})

	acc, err := r.HTTPAccess("PRV1")
	if err != nil {
		t.Fatal(err)
	}
	if acc.Kind != "openrouter" || acc.BaseURL != "https://openrouter.ai/api/v1" || acc.InstanceID != "PRV1" {
		t.Errorf("access = %+v", acc)
	}
	h := http.Header{}
	acc.Authorize(h)
	if h.Get("Authorization") != "Bearer sk-or-test" {
		t.Errorf("authorization = %q", h.Get("Authorization"))
	}
	if r.InstanceBaseURL("PRV1") != "https://openrouter.ai/api/v1" || r.InstanceBaseURL("nope") != "" {
		t.Error("InstanceBaseURL wrong")
	}

	for id, want := range map[string]string{
		"PRV2":       "disabled",
		"PRV3":       "not configured",
		"claude-cli": "not an API instance",
		"missing":    "unknown provider instance",
	} {
		if _, err := r.HTTPAccess(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("HTTPAccess(%q) err = %v, want it to mention %q", id, err, want)
		}
	}
}

func TestJevPricedInputOnly(t *testing.T) {
	for _, model := range []string{"typesafe/jev-1.13", "~typesafe/jev-latest"} {
		p, ok := PriceFor("openrouter", model)
		if !ok || p.InputPerMTok != 0.042 || p.OutputPerMTok != 0 {
			t.Errorf("PriceFor(openrouter, %s) = %+v, %v", model, p, ok)
		}
	}
}
