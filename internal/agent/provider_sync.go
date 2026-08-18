package agent

import "github.com/bilal-arikan/tionswarm/internal/providers"

// SyncProviderFields resolves the kind id for a provider instance id and
// returns the (provider, providerInstanceID) pair every agent-writing call
// site must persist together (_Docs/71 §2.5, K3): Agent.ProviderInstanceID is
// the single source of truth, Agent.Provider is its DERIVED kind id, kept in
// sync so the many kind-keyed readers (billing, context-window sizing, usage
// rollups — _Docs/71 §4.1) keep working unchanged.
//
// instanceID == "" is treated as the keyless claude-cli default, matching
// Registry.Get's historical empty-provider behaviour — it resolves to the
// "claude-cli" kind rather than erroring, so creating an agent without
// picking a provider still works. Any other unresolvable instance id
// (deleted/never existed) is a hard error: silently falling back to
// claude-cli would misattribute that agent's billing/context-window
// resolution to the wrong provider (_Docs/71 §4.3 risk table).
func SyncProviderFields(registry *providers.Registry, instanceID string) (provider, providerInstanceID string, err error) {
	kind := registry.KindOf(instanceID)
	if kind == "" {
		resolved := instanceID
		if resolved == "" {
			resolved = "claude-cli"
		}
		return "", "", &UnknownProviderInstanceError{InstanceID: resolved}
	}
	resolved := instanceID
	if resolved == "" {
		resolved = "claude-cli"
	}
	return kind, resolved, nil
}

// UnknownProviderInstanceError reports that an agent write referenced a
// provider instance id the registry does not recognize (not configured, or
// deleted after the agent was created).
type UnknownProviderInstanceError struct {
	InstanceID string
}

func (e *UnknownProviderInstanceError) Error() string {
	return "unknown provider instance: " + e.InstanceID
}
