package agent

import (
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// runWorker uses the same registration and execution path as real launches.
func (r *Runtime) runWorker(agent db.Agent, workerSessionID, prompt, coordSessionID string) {
	runCtx, cancelRun, ctl := r.newWorkerRun(workerSessionID)
	r.runWorkerRegistered(runCtx, cancelRun, agent, workerSessionID, prompt, coordSessionID, ctl)
}

// enqueueCoordinatorWorkerTurn supplies a fresh persistence time for fixtures.
func (r *Runtime) enqueueCoordinatorWorkerTurn(coordSessionID string, keepIdleAck bool) {
	r.enqueueCoordinatorWorkerTurnAt(coordSessionID, keepIdleAck, time.Now())
}
