package agent

import (
	"context"
	"errors"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
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

// RecordUsage adds one call and its token usage to the agent's daily counters.
// Failures are non-fatal and only logged.
func (r *Runtime) RecordUsage(ctx context.Context, agentID string, u providers.Usage) {
	if err := r.db.AddUsage(ctx, agentID, 1, u.InputTokens, u.OutputTokens); err != nil {
		r.logger.Warn("record usage failed", "agent", agentID, "error", err)
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
		return nil, err
	}
	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	r.RecordUsage(ctx, agent.ID, resp.Usage)
	return resp, nil
}
