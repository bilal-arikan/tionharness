package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/billing"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

// Trajectory summary persistence (Rota F3): the pure digest lives in
// internal/trajectory (Summarize); this file prices sessions and stores the
// result on the graph.

func (r *Runtime) sessionCost(ctx context.Context) trajectory.SessionCostFn {
	return func(sessionID string) (int64, float64, bool) {
		u, err := r.db.GetSessionUsage(ctx, sessionID)
		if err != nil {
			return 0, 0, true // no usage recorded: nothing to price
		}
		roll := billing.RollupOf(u.ByModel)
		return u.TotalTokens(), roll.CostUSD, roll.Priced
	}
}

// SummarizeTrajectory computes and stores the summary of a trajectory (by id)
// and returns the updated graph.
func (r *Runtime) SummarizeTrajectory(ctx context.Context, id string) (db.Trajectory, error) {
	if strings.TrimSpace(id) == "" {
		return db.Trajectory{}, errors.New("trajectory: empty id")
	}
	cost := r.sessionCost(ctx)
	nowSec := time.Now().Unix()
	return r.db.UpdateTrajectory(ctx, id, 0, func(t *db.Trajectory) error {
		s := trajectory.Summarize(*t, cost, nowSec)
		t.Summary = &s
		return nil
	})
}

// summarizeOnEnd is queued by observeTrajectoryTransitions ahead of the
// trajectory_end automations, so a rule's prompt and the curator see the final
// numbers.
func (r *Runtime) summarizeOnEnd(id string) {
	if _, err := r.SummarizeTrajectory(context.Background(), id); err != nil {
		r.logger.Warn("trajectory summary failed", "trajectory", id, "error", err)
	}
}
