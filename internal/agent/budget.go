package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// ErrAutonomyPaused is returned when the global autonomy brake is engaged and an
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
		r.emitDebug(ctx, db.DebugEvent{
			Type:       db.DebugLLMCall,
			AgentID:    agent.ID,
			Kind:       kind,
			Model:      model,
			In:         u.InputTokens,
			Out:        u.OutputTokens,
			CacheRead:  u.CacheReadTokens,
			CacheWrite: u.CacheWriteTokens,
			Calls:      providerCalls,
		})
	}
}

// guardedComplete is the single funnel for provider calls inside the runtime.
// When autonomous, it honors the global autonomy brake first; it always records
// usage afterward so the meter reflects every call.
func (r *Runtime) guardedComplete(ctx context.Context, agent db.Agent, req providers.Request, autonomous bool) (*providers.Response, error) {
	if autonomous {
		if r.tun.AutonomyPaused() || r.Paused() {
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
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage, resp.ProviderCalls)
	return resp, nil
}
