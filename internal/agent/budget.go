package agent

import (
	"context"
	"errors"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// ErrBudgetExceeded is returned when an autonomous call would exceed an agent's
// daily spend caps. Manual (user-initiated) calls are never subject to this.
var ErrBudgetExceeded = errors.New("daily budget exceeded")

// ErrAutonomyPaused is returned when the global autonomy brake is engaged and an
// autonomous call is attempted. Manual calls are unaffected.
var ErrAutonomyPaused = errors.New("autonomy paused")

// ensureBudget verifies an agent is under its daily caps before an autonomous
// call. Zero limits mean unlimited.
func (r *Runtime) ensureBudget(ctx context.Context, agent db.Agent) error {
	if agent.DailyCallLimit <= 0 && agent.DailyTokenLimit <= 0 {
		return nil
	}
	u, err := r.db.GetUsageToday(ctx, agent.ID)
	if err != nil {
		// A usage-lookup failure here silently blocks the autonomous call (it
		// propagates up as the call's error). Log it so a budget-gate failure is
		// distinguishable from a real ErrBudgetExceeded in the logs view.
		r.logger.Warn("budget check failed",
			"agent", agent.ID, "callKind", callKindFrom(ctx), "error", err)
		return err
	}
	if agent.DailyCallLimit > 0 && u.Calls >= agent.DailyCallLimit {
		return ErrBudgetExceeded
	}
	if agent.DailyTokenLimit > 0 && u.InputTokens+u.OutputTokens >= agent.DailyTokenLimit {
		return ErrBudgetExceeded
	}
	return nil
}

// RecordUsage adds one call and its token usage to the agent's daily counters,
// attributed to the call origin stamped on ctx (KindChat by default) and the
// provider+model that served it (for cost). Because every funnel —
// guardedComplete, recordedComplete, recordedStream — records through here,
// tagging the context at each entry point is enough to break the whole daily
// spend down by origin and model. Failures are non-fatal and only logged.
func (r *Runtime) RecordUsage(ctx context.Context, agent db.Agent, model string, u providers.Usage) {
	if model == "" {
		model = agent.Model
	}
	delta := db.DeltaFromUsage(1, u)
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
	}
}

// guardedComplete is the single funnel for provider calls inside the runtime.
// When autonomous, it enforces the agent's daily budget first; it always
// records usage afterward so the meter reflects every call.
func (r *Runtime) guardedComplete(ctx context.Context, agent db.Agent, req providers.Request, autonomous bool) (*providers.Response, error) {
	if autonomous {
		if r.tun.AutonomyPaused() || r.Paused() {
			return nil, ErrAutonomyPaused
		}
		if err := r.ensureBudget(ctx, agent); err != nil {
			return nil, err
		}
	}
	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		r.logger.Warn("provider resolve failed",
			"agent", agent.ID, "provider", agent.Provider,
			"callKind", callKindFrom(ctx), "error", err)
		return nil, err
	}
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
	r.RecordUsage(ctx, agent, resp.Model, resp.Usage)
	return resp, nil
}
