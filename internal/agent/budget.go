package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// deriveThinkingTokens estimates the hidden-reasoning share of a native
// provider response. The API bills extended thinking inside OutputTokens without
// a separate field, so thinking ≈ OutputTokens − visible(text + tool_use). The
// visible estimate uses the calibrated conversation estimator. claude-cli's
// OutputTokens is CUMULATIVE across its internal tool-loop steps (ProviderCalls
// > 1), which would make this subtraction meaningless, so it returns 0 there.
// Never negative (a visible over-estimate clamps to 0, not a spurious value).
func deriveThinkingTokens(resp *providers.Response) int {
	if resp == nil || resp.ProviderCalls > 1 {
		return 0
	}
	visible := conversation.EstimateText(resp.Text)
	for _, tc := range resp.ToolCalls {
		visible += conversation.EstimateText(tc.Name) + conversation.EstimateText(string(tc.Input))
	}
	if t := resp.Usage.OutputTokens - visible; t > 0 {
		return t
	}
	return 0
}

// ErrAutonomyPaused is returned when the workspace autonomy brake is engaged and an
// autonomous call is attempted. Manual calls are unaffected.
var ErrAutonomyPaused = errors.New("autonomy paused")

// RecordUsage adds one call and its token usage to the agent's daily counters,
// attributed to the call origin stamped on ctx (KindChat by default) and the
// provider+model that served it (for cost). Because every funnel —
// guardedComplete, recordedComplete, recordedStream — records through here,
// tagging the context at each entry point is enough to break the whole daily
// spend down by origin and model. Failures are non-fatal and only logged.
func (r *Runtime) RecordUsage(ctx context.Context, agent db.Agent, model string, u providers.Usage, providerCalls int) {
	if model == "" {
		model = agent.Model
	}
	delta := db.DeltaFromUsage(1, u)
	delta.ProviderCalls = providerCalls // CLI internal round-trips (num_turns); 0 → counted as 1 in the rollup
	kind := string(callKindFrom(ctx))
	if err := r.db.AddUsageKind(ctx, agent.ID, kind, agent.Provider, model, delta); err != nil {
		r.logger.Warn("record usage failed", "agent", agent.ID, "error", err)
	}
	// Also attribute the same call to its originating session (lifetime rollup),
	// so spend can be broken down per-conversation. A blank session id (e.g. a
	// detached auxiliary call without a session stamp) is a no-op in the DB layer.
	if sid := SessionIDFrom(ctx); sid != "" {
		if err := r.db.AddSessionUsageKind(ctx, sid, agent.ID, kind, agent.Provider, model, delta); err != nil {
			r.logger.Warn("record session usage failed", "agent", agent.ID, "session", sid, "error", err)
		}
		// Debug journal: record this provider call's per-call token spend (model +
		// input/output/cache) so the per-session debug stream can attribute where
		// tokens went, turn by turn — finer than the lifetime SessionUsage rollup.
		promptKey, promptHash := promptTraceFrom(ctx)
		r.emitDebug(ctx, db.DebugEvent{
			Type:       db.DebugLLMCall,
			AgentID:    agent.ID,
			Kind:       kind,
			Model:      model,
			PromptKey:  promptKey,
			PromptHash: promptHash,
			In:         u.InputTokens,
			Out:        u.OutputTokens,
			Think:      u.ThinkingTokens,
			CacheRead:  u.CacheReadTokens,
			CacheWrite: u.CacheWriteTokens,
			Calls:      providerCalls,
		})
	}
}

// noteResolvedModel remembers which concrete model the provider actually served
// for the requested id, so the UI can say "Opus 5" where the agent config only
// says "opus". claude-cli reports the real id on every turn; native providers
// echo what they were given and are filtered out in the store.
//
// Best-effort by design: a persistence failure is logged, never propagated — a
// missing label must not fail a turn that already produced an answer.
func (r *Runtime) noteResolvedModel(ctx context.Context, agent db.Agent, requested, resolved string) {
	if err := r.db.NoteModelResolution(ctx, agent.Provider, requested, resolved); err != nil {
		r.logger.Warn("record model resolution failed",
			"agent", agent.ID, "provider", agent.Provider, "requested", requested, "error", err)
	}
}

// guardedComplete is the single funnel for provider calls inside the runtime.
// When autonomous, it honors the global autonomy brake first; it always records
// usage afterward so the meter reflects every call.
func (r *Runtime) guardedComplete(ctx context.Context, agent db.Agent, req providers.Request, autonomous bool) (*providers.Response, error) {
	if autonomous {
		if r.Paused() {
			return nil, ErrAutonomyPaused
		}
	}
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		r.logger.Warn("provider resolve failed",
			"agent", agent.ID, "provider", agent.Provider,
			"callKind", callKindFrom(ctx), "error", err)
		return nil, err
	}
	// claude-cli auxiliary calls (title/summary/compaction/lesson reflection)
	// must use THIS workspace's config home like tool-loop turns do.
	r.PinClaudeHome(provider)
	// Fill a model-aware output cap when the caller left MaxTokens unset; the
	// explicit caps that compaction/summary/title set are respected untouched.
	req = r.withMaxOutput(agent.Provider, req)
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		// Mirror recordedComplete's logging: guardedComplete is the funnel for the
		// non-tool autonomous calls (reflect/summary/title), so a provider failure
		// here must surface in the logs too — not just propagate up silently.
		r.logger.Warn("provider complete failed",
			"agent", agent.ID, "provider", agent.Provider, "model", req.Model,
			"callKind", callKindFrom(ctx), "error", err)
		return nil, err
	}
	resp.Usage.ThinkingTokens = deriveThinkingTokens(resp)
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
	r.noteResolvedModel(ctx, agent, req.Model, resp.Model)
	return resp, nil
}

// PinClaudeHome pins THIS workspace's claude-cli config home on a claude-cli
// provider before a Complete call, so the CLI reads skills/settings/login from
// <workspace>/claude-home instead of the global default. guardedComplete applies
// this for in-loop and autonomous aux calls; out-of-loop session commands that
// call provider.Complete directly (manual /compact, /handoff) must call it
// themselves, or they fall back to the global claude-home — which may not be
// logged in even though the workspace is (authentication_failed). No-op for
// non-claude-cli providers.
func (r *Runtime) PinClaudeHome(provider providers.Provider) {
	if cli, ok := provider.(*providers.ClaudeCLI); ok {
		cli.SetConfigDir(r.claudeHomeDir())
	}
}
