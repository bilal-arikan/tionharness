package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// FoldContext stamps onto ctx everything a compaction / handoff fold needs and
// that the shared conversation.Manager cannot know on its own: the workspace's
// editable compaction prompt and the fold TARGET — the provider + agent copy the
// fold's direct provider.Complete should run on.
//
// Historically the fold ran on the SESSION agent's provider and model: on a
// claude-cli opus session that is a fresh `claude -p` at opus price carrying the
// ~36k-token Claude Code base prompt, for a call whose only input is the rendered
// transcript. The "compaction" system agent already names a cheaper model; this
// is where it takes effect, and routeAuxAgent moves the call to a configured
// anthropic API instance when the caller is on a CLI (_Docs/74, _Docs/17). When
// nothing changes (native caller, no anthropic key, resolution failure) no
// target is stamped and the fold runs exactly as before.
func (r *Runtime) FoldContext(ctx context.Context, agent db.Agent) context.Context {
	ctx = conversation.WithCompactPrompt(ctx, r.CompactPromptTemplate())
	target, ok := r.resolveFoldAgent(agent)
	if !ok {
		return ctx
	}
	if target.ProviderRef() == agent.ProviderRef() {
		if target.Model == agent.Model {
			return ctx
		}
		// Same provider, cheaper model: the fold keeps the provider object it was
		// handed (nil override) and only the agent copy changes.
		return conversation.WithFoldTarget(ctx, nil, target)
	}
	if r.providers == nil {
		return ctx
	}
	provider, err := r.providers.Get(target.ProviderRef())
	if err != nil {
		r.logger.Warn("fold target provider unavailable; folding on the session agent",
			"provider", target.ProviderRef(), "error", err)
		return ctx
	}
	return conversation.WithFoldTarget(ctx, provider, target)
}

// resolveFoldAgent derives the fold's agent copy from the "compaction" system
// agent: the caller's identity and credentials, the system agent's model
// (adoptSystemAgentModel keeps the caller's model where the two providers are
// incompatible) and, when applicable, the native routing of routeAuxAgent.
func (r *Runtime) resolveFoldAgent(agent db.Agent) (db.Agent, bool) {
	compactor, _, err := r.ResolveSystemAgent("compaction")
	if err != nil {
		return agent, false
	}
	agent.Model = adoptSystemAgentModel(r.logger, "compaction", agent.Provider, agent.Model, compactor.Model)
	agent.System = true
	agent.SystemKey = compactor.SystemKey
	return r.routeAuxAgent(agent, compactor), true
}
