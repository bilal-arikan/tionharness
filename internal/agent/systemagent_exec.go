package agent

import "github.com/bilal-arikan/tionharness/internal/db"

// systemAgentExecutor builds the agent copy a system role's call runs as. The
// rule (2026-09-11, user decision): the system agent's OWN provider, instance
// and model are what runs — what the Settings ▸ System agents card shows is the
// rule, not a suggestion. The caller only lends its id (billing) and, when the
// system agent names no usable provider, its transport.
//
//   - provider/instance: the system agent's, when it has one that is available
//     (a codex-only install whose built-in titler says claude-cli keeps running
//     titles on codex rather than failing on a missing CLI);
//   - model: the system agent's, checked against the provider it will run on
//     (adoptSystemAgentModel keeps the caller's model for an alias the provider
//     cannot resolve);
//   - auxiliary native routing (CLI → anthropic, systemagent_route.go) only
//     applies when the user did NOT pin the provider on the system agent.
func (r *Runtime) systemAgentExecutor(key string, caller db.Agent, systemAgent db.Agent) db.Agent {
	exec := caller
	exec.System = true
	exec.SystemKey = systemAgent.SystemKey
	if systemAgent.Provider != "" {
		inst := systemAgent.ProviderInstanceID
		if inst == "" {
			inst = systemAgent.Provider
		}
		// The caller's empty provider IS the keyless claude-cli default: switching
		// it to the named instance would only swap provider objects mid-fold for
		// the same transport, so that case is a no-op.
		same := exec.Provider == systemAgent.Provider ||
			(exec.Provider == "" && systemAgent.Provider == "claude-cli" && inst == "claude-cli")
		// A pinned provider is the user's explicit choice and always wins (an
		// unavailable one fails visibly, which is right); the built-in default is
		// only taken when the registry can actually serve it.
		if !same && (pinsProvider(systemAgent) || (r.providers != nil && r.providers.Available(inst))) {
			exec.Provider = systemAgent.Provider
			exec.ProviderInstanceID = inst
		}
	}
	if systemAgent.Model != "" {
		exec.Model = adoptSystemAgentModel(r.logger, key, exec.Provider, exec.Model, systemAgent.Model)
	}
	if pinsProvider(systemAgent) {
		return nativeAuxModel(exec)
	}
	return r.routeAuxAgent(exec, systemAgent)
}
