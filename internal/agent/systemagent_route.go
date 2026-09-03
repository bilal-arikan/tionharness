package agent

import (
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
	if id == "" {
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
