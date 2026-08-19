package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
)

func TestListProvidersReturnsSummaries(t *testing.T) {
	tool := NewListProvidersTool(func() []providers.InstanceSummary {
		return []providers.InstanceSummary{
			{ID: "anthropic", KindID: "anthropic", Label: "Anthropic", Enabled: true, DefaultModel: "claude-opus-5", Available: true},
			{ID: "PRV3", KindID: "openai-compat", Label: "Router", Enabled: false},
		}
	})
	out, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("list_providers: %v", err)
	}
	var got struct {
		Providers []providers.InstanceSummary `json:"providers"`
		Total     int                         `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, out)
	}
	if got.Total != 2 || len(got.Providers) != 2 {
		t.Fatalf("want 2 instances, got %d (%s)", got.Total, out)
	}
	if got.Providers[1].ID != "PRV3" || got.Providers[1].Enabled {
		t.Fatalf("disabled instance must still be listed as disabled: %+v", got.Providers[1])
	}
	// Credentials must never appear in the payload.
	if strings.Contains(strings.ToLower(out), "secret") || strings.Contains(strings.ToLower(out), "apikey") {
		t.Fatalf("credential-looking field leaked: %s", out)
	}
}

func TestListProvidersWithoutRegistryErrors(t *testing.T) {
	if _, err := (ListProvidersTool{}).Call(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("want error when the registry is not configured")
	}
}

func TestRegistryListInstancesIsSortedAndCredentialFree(t *testing.T) {
	r := providers.NewRegistry()
	r.SetInstances([]providers.Instance{
		{ID: "zeta", KindID: "openai-compat", Label: "Z", Enabled: true, Values: map[string]string{"apiKey": "sk-secret"}},
		{ID: "alpha", KindID: "anthropic", Label: "A", Enabled: false},
	})
	list := r.ListInstances()
	if len(list) != 2 || list[0].ID != "alpha" || list[1].ID != "zeta" {
		t.Fatalf("want id-sorted list, got %+v", list)
	}
	b, _ := json.Marshal(list)
	if strings.Contains(string(b), "sk-secret") {
		t.Fatalf("secret leaked into summary: %s", b)
	}
}
