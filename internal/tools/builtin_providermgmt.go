package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Provider tools are READ-ONLY by design. Provider instances carry credentials
// (API keys, per-instance CLI config dirs) and are created/edited/deleted only
// through the Settings screen and its HTTP API (PUT/DELETE /api/providers).
// What an agent needs is the ability to SEE which instances exist so it can
// bind an agent to one via create_agent/update_agent's `provider` field, which
// takes a provider INSTANCE id (_Docs/71 §5).

// ---- list_providers ----

// ListProvidersTool lists the configured provider instances, credential-free.
type ListProvidersTool struct {
	list func() []providers.InstanceSummary
}

// NewListProvidersTool constructs list_providers over the given lister
// (Registry.ListInstances in production).
func NewListProvidersTool(list func() []providers.InstanceSummary) ListProvidersTool {
	return ListProvidersTool{list: list}
}

func (ListProvidersTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "list_providers",
		Description: "List the provider instances configured in this app (the entries on the Settings → Providers screen). " +
			"Returns id, kindId (anthropic | openai-compat | claude-cli | codex-cli | ...), label, enabled, defaultModel, " +
			"the instance's model list and whether it is currently usable (available: kind registered and credentials/binary " +
			"present). API keys are never returned. Use the `id` here as the `provider` value of create_agent / update_agent " +
			"to bind an agent to that instance. Provider instances themselves are created, edited and deleted only from the " +
			"Settings screen — no tool can change them.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t ListProvidersTool) Call(_ context.Context, _ json.RawMessage) (string, error) {
	if t.list == nil {
		return "", fmt.Errorf("provider registry not configured")
	}
	list := t.list()
	b, err := json.Marshal(map[string]any{"providers": list, "total": len(list)})
	if err != nil {
		return "", err
	}
	return string(b), nil
}
