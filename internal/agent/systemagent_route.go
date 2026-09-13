package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// auxNativeKind is the first-party API kind auxiliary calls are routed to.
const auxNativeKind = "anthropic"

// routeAuxAgent decides WHERE a tool-less auxiliary system-agent call (title,
// summary, compaction fold, lesson/insight reflection, recipe optimizer, stall
// judge) runs. The resolvers keep the calling agent's identity and credentials
// and only borrow the system agent's prompt + model; that is right for billing
// but wrong for cost when the caller is on a CLI transport: every such call is a
// fresh `claude -p` that pays Claude Code's ~36k-token base prompt (measured on
// 2.1.259, _Docs/17) to produce a ten-token answer — a title, a JSON verdict.
//
// When the AuxNativeRouting tunable is on, the caller is on a CLI kind and an
// anthropic API instance is configured, the copy is re-pointed at that instance
// with the system agent's model translated to a Messages API id
// (providers.NativeClaudeModel): the same haiku-class call then costs ~1–2k
// tokens. Everything else (native callers, no anthropic key, tunable off) is
// returned unchanged, so the historical behaviour is the fallback, never an
// error path. Billing stays on the calling agent's ID; only the provider column
// of the usage row changes, which is what actually happened.
func (r *Runtime) routeAuxAgent(agent db.Agent, systemAgent db.Agent) db.Agent {
	if r == nil || r.tun == nil || !r.tun.AuxNativeRouting() || r.providers == nil || !isCLIProviderKind(agent.Provider) {
		return nativeAuxModel(agent)
	}
	id := r.providers.FirstAvailableOfKind(auxNativeKind)
	if id == "" || r.auxRouteQuarantined(id) {
		return agent
	}
	agent.Provider = auxNativeKind
	agent.ProviderInstanceID = id
	agent.Model = providers.NativeClaudeModel(systemAgent.Model)
	return agent
}

// nativeAuxModel translates a CLI model alias ("haiku") the system agent
// suggested into a Messages API id when the calling agent is ALREADY on the
// first-party anthropic kind — the alias is a claude-cli spelling the API does
// not resolve. Concrete ids and every other provider pass through unchanged.
func nativeAuxModel(agent db.Agent) db.Agent {
	if agent.Provider == auxNativeKind && isClaudeModel(agent.Model) {
		agent.Model = providers.NativeClaudeModel(agent.Model)
	}
	return agent
}

// An anthropic instance that rejected its key is taken out of auxiliary routing
// for as long as the provider CONFIGURATION stays the same (providers.Registry
// Generation): "available" only means "has a key", and a stale key — a user who
// only ever runs claude-cli / codex-cli but still has an old API key saved —
// would otherwise break every title, summary, goal draft and canvas edit. The
// judgement is dropped the moment providers are re-saved, so a fixed key comes
// back without a restart and a wrong one never gets retried on a timer.

// noteAuxRouteFailure quarantines the instance after an authentication error on
// the auxiliary route (called from guardedComplete's error path).
func (r *Runtime) noteAuxRouteFailure(agent db.Agent, err error) {
	if r == nil || r.providers == nil || agent.Provider != auxNativeKind || agent.ProviderInstanceID == "" {
		return
	}
	if classifyProviderError(err) != errAuth {
		return
	}
	gen := r.providers.Generation()
	if _, loaded := r.auxRouteBad.LoadOrStore(agent.ProviderInstanceID, gen); !loaded && r.logger != nil {
		r.logger.Warn("auxiliary native route rejected its key; falling back to the caller's provider until providers are re-saved",
			"instance", agent.ProviderInstanceID, "error", err)
	} else {
		r.auxRouteBad.Store(agent.ProviderInstanceID, gen)
	}
}

// auxRouteFallback, after an authentication error on an aux-routed call,
// rebuilds the agent as it was BEFORE routing: the routed copy kept the
// caller's id, so the caller's own row supplies its provider, instance and
// model. Only a CLI caller qualifies (a native caller was never re-routed), and
// only when its row still resolves — otherwise there is nothing to retry on.
func (r *Runtime) auxRouteFallback(ctx context.Context, routed db.Agent, err error) (db.Agent, bool) {
	if r == nil || r.db == nil || routed.Provider != auxNativeKind || routed.SystemKey == "" {
		return db.Agent{}, false
	}
	if classifyProviderError(err) != errAuth {
		return db.Agent{}, false
	}
	// The system agent's own transport is what the call was routed away from.
	if sa, _, serr := r.ResolveSystemAgent(routed.SystemKey); serr == nil && isCLIProviderKind(sa.Provider) {
		inst := sa.ProviderInstanceID
		if inst == "" {
			inst = sa.Provider
		}
		if r.providers != nil && r.providers.Available(inst) {
			fallback := routed
			fallback.Provider = sa.Provider
			fallback.ProviderInstanceID = inst
			fallback.Model = adoptSystemAgentModel(r.logger, routed.SystemKey, sa.Provider, routed.Model, sa.Model)
			return fallback, true
		}
	}
	// Otherwise the caller's own row (the transport it lent before routing).
	row, gerr := r.db.GetAgent(ctx, routed.ID)
	if gerr != nil || !isCLIProviderKind(row.Provider) || row.Provider == auxNativeKind {
		return db.Agent{}, false
	}
	fallback := routed
	fallback.Provider = row.Provider
	fallback.ProviderInstanceID = row.ProviderInstanceID
	fallback.Model = row.Model
	return fallback, true
}

// auxRouteQuarantined reports whether instanceID is still sitting out: the
// rejection was recorded under the current provider configuration.
func (r *Runtime) auxRouteQuarantined(instanceID string) bool {
	v, ok := r.auxRouteBad.Load(instanceID)
	if !ok {
		return false
	}
	if gen, _ := v.(uint64); r.providers != nil && gen == r.providers.Generation() {
		return true
	}
	r.auxRouteBad.Delete(instanceID)
	return false
}
