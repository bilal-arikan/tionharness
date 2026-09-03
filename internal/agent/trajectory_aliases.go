package agent

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

// The Rota graph logic moved to internal/trajectory (_Docs/81, step 2). The
// names below keep the agent package's public surface — used by internal/api and
// internal/workspace — stable while callers migrate to the new package.
type (
	TrajectoryPlanPhase  = trajectory.PlanPhase
	TrajectoryTransition = trajectory.Transition
	RecipeStats          = trajectory.RecipeStats
)

const (
	TrajTransitionPhase = trajectory.TransitionPhase
	TrajTransitionEnd   = trajectory.TransitionEnd
)

// RecipeStats is trajectory.RecipeStatsFromIndex for the API layer.
func (r *Runtime) RecipeStats(rows []db.TrajectoryIndexEntry) []RecipeStats {
	return trajectory.RecipeStatsFromIndex(rows)
}
