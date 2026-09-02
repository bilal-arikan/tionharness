package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// flow_coordinator.go backs the orchestration `coordinator` node: the flow's
// deterministic engine stays in charge of the GRAPH, while one node delegates a
// sub-goal to a live coordinator session that decides at runtime how many workers
// to spawn (see coordination.go / _Docs/47). This is the piece parallel/spawn
// cannot express — their fan-out width is fixed when the flow is drawn.
//
// The node is atomic from the engine's point of view: it blocks until the
// coordinator settles, then returns one output. A crash mid-node therefore
// re-runs the whole node on resume (a FRESH coordinator session), which keeps the
// restart-safe state model intact — see SessionKindFlowCoordinator.

// SessionKindFlowCoordinator marks a coordinator session owned by a flow's
// coordinator node. It is deliberately distinct from an ordinary "coordinator"
// role session opened by a user: crash recovery must NOT resurrect it, because
// ResumeRunningFlows already re-runs the owning node with a fresh session.
const SessionKindFlowCoordinator = "flow-coordinator"

// coordinatorSettleTimeout is the default wall-clock bound for a coordinator
// node when the node sets no TimeoutSec. Generous on purpose: a coordinator
// dispatching several workers legitimately runs for many minutes.
const coordinatorSettleTimeout = 30 * time.Minute

// coordinatorSettlePoll is how often the node re-checks whether its coordinator
// has settled. Matches the join node's poll cadence (joinPollInterval).
const coordinatorSettlePoll = 500 * time.Millisecond

// RunCoordinatorNode implements orchestration.CoordinatorRunner.
func (f flowRunner) RunCoordinatorNode(ctx context.Context, spec orchestration.CoordinatorSpec) (string, error) {
	return f.rt.RunCoordinatorNode(ctx, spec)
}

// RunCoordinatorNode opens a dedicated coordinator session for one flow node,
// seeds it with the rendered prompt, drives it through the normal coordinator
// notify-loop (the agent spawns/steers workers itself), waits until every worker
// has finished and no further coordinator turn is pending, and returns the
// coordinator's final reply as the node's output.
func (r *Runtime) RunCoordinatorNode(ctx context.Context, spec orchestration.CoordinatorSpec) (string, error) {
	prompt := strings.TrimSpace(spec.Prompt)
	if prompt == "" {
		return "", fmt.Errorf("coordinator node has an empty prompt")
	}
	agent, err := r.db.GetAgent(ctx, spec.AgentID)
	if err != nil {
		return "", fmt.Errorf("coordinator agent unavailable: %w", err)
	}
	// Same gate the session panel uses when a user picks a recipe: an unknown or
	// non-coordinator skill fails the node instead of running free-form under a
	// name the flow author believed was in effect.
	recipeTurns, err := skills.ResolveCoordinatorWorkflow(r.skills, spec.Workflow)
	if err != nil {
		return "", fmt.Errorf("coordinator workflow: %w", err)
	}
	// The node's own cap wins; otherwise the recipe's max_turns applies (0 = the
	// workspace default, resolved per turn in drainCoordinator).
	maxTurns := spec.MaxTurns
	if maxTurns == 0 {
		maxTurns = recipeTurns
	}

	title := "🧭 " + agent.Name
	if nodeID := orchestration.NodeIDFromContext(ctx); nodeID != "" {
		title += " · " + nodeID
	}
	runID := flowRunIDFromContext(ctx)
	origin := &db.SessionOrigin{
		Kind:   db.OriginFlow,
		RunID:  runID,
		NodeID: orchestration.NodeIDFromContext(ctx),
		// The run's own transcript session is the one this coordinator hangs
		// under in the lineage graph; the flow id names the entity.
		TriggerSessionID: flowTranscriptSessionFromContext(ctx),
	}
	if run, rerr := r.db.GetFlowRun(ctx, runID); rerr == nil {
		origin.EntityID = run.FlowID
	}
	sess, err := r.db.CreateSession(ctx, db.Session{
		AgentID: agent.ID,
		Kind:    SessionKindFlowCoordinator,
		Origin:  origin,
		// Same directory the flow run itself works in; its workers then inherit from
		// here (SpawnWorker), so a whole coordinator tree stays in one repository.
		WorkingDir: r.effectiveWorkDir(ctx),
		// SourceID points at the owning flow run so the executions feed and the run
		// viewer can resolve this session back to the run that produced it.
		SourceID: flowRunIDFromContext(ctx),
		Title:    title,
		// Coordinator CAPABILITY, not lineage: the flow owns this session, it has no
		// parent coordinator to report to, so Role stays empty and it is the root of
		// its own tree.
		CoordinatorMode: true,
		// Persisting the slug is all the recipe needs: the turn's static prefix
		// resolves and injects its body from the session (coordinatorRecipeBlock).
		// Versioned ref ("slug@version") when the recipe declares a version, so the
		// session — and the trajectory seeded from it — record the revision (R6).
		CoordinatorWorkflow: skills.RecipeRefFor(r.skills, spec.Workflow),
		CoordinatorMaxTurns: maxTurns,
	})
	if err != nil {
		return "", fmt.Errorf("coordinator session create failed: %w", err)
	}
	// Own cancelable context for the coordinator node so a human "Durdur"
	// (CancelSession) can stop the wait/drain — it never enters the api chatRuns.
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	ctx = runCtx
	run := r.trackSession(sess.ID, cancelRun)
	defer run.release()
	// Announce it up front so the sidebar/executions feed shows the coordinator
	// (and, through it, its workers) while the node is still running.
	r.publish(events.Event{
		Type:   events.TypeSession,
		Level:  "info",
		Target: map[string]string{"sessionId": sess.ID, "op": "create"},
	})

	if _, err := r.recordInjectedUserNote(ctx, sess.ID, "", prompt); err != nil {
		return "", fmt.Errorf("coordinator prompt record failed: %w", err)
	}

	// enqueueCoordinatorTurn claims the slot SYNCHRONOUSLY (running=true) before
	// starting its drain goroutine, so the settle wait below can never observe a
	// premature "idle" between this call and the first turn.
	r.enqueueCoordinatorTurn(sess.ID)
	r.logger.Info("flow coordinator node started",
		"session", sess.ID, "agent", agent.ID, "workflow", spec.Workflow, "maxTurns", maxTurns)

	waitErr := r.waitCoordinatorIdle(ctx, sess.ID, spec.TimeoutSec)
	// The slot is per-session bookkeeping for a session nothing will reuse; drop it
	// so a long-lived workspace does not accumulate one entry per coordinator node.
	// A late straggler notification simply re-creates a fresh slot.
	r.coordSlots.Delete(sess.ID)
	r.turns.Forget(sess.ID)
	if waitErr != nil {
		r.stopCoordinatorWorkers(ctx, sess.ID)
		return "", waitErr
	}

	out := r.lastAssistantText(ctx, sess.ID)
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("coordinator produced no reply (session %s)", sess.ID)
	}
	r.logger.Info("flow coordinator node finished", "session", sess.ID)
	return out, nil
}

// waitCoordinatorIdle blocks until the coordinator session has settled: no turn
// running, none pending, and no active worker. Returns an error when ctx is
// cancelled or the deadline (timeoutSec, or coordinatorSettleTimeout when 0)
// elapses first.
//
// Polling is safe against the completion race because runWorker calls
// NotifyCoordinator — which claims the slot synchronously — BEFORE its deferred
// worker-counter decrement runs. There is therefore no instant where the last
// worker has been counted out while its coordinator turn is not yet claimed.
func (r *Runtime) waitCoordinatorIdle(ctx context.Context, coordSessionID string, timeoutSec int) error {
	limit := coordinatorSettleTimeout
	if timeoutSec > 0 {
		limit = time.Duration(timeoutSec) * time.Second
	}
	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	tick := time.NewTicker(coordinatorSettlePoll)
	defer tick.Stop()

	slot := r.coordSlotFor(coordSessionID)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("coordinator did not settle within %s", limit)
		case <-tick.C:
			// The slot's own counter tracks DIRECT workers only. A sub-coordinator
			// decrements it the moment its own turn ends — while its branch keeps
			// working — so the slot alone would read as settled with grandchildren
			// still running, and the node would take a half-finished result.
			if !r.coordSlotIdle(coordSessionID, slot) {
				continue
			}
			// An unreadable subtree is not an idle one: keep polling and let the
			// deadline above surface it as a failure to settle.
			if active, err := r.activeSubtreeWorkers(ctx, coordSessionID); err == nil && active == 0 {
				return nil
			}
		}
	}
}

// coordSlotIdle reports whether a coordinator is fully quiescent: no turn holds (or
// is queued for) its admission slot, no notification is pending, no drain loop is
// active, and no worker is running.
func (r *Runtime) coordSlotIdle(coordSessionID string, slot *coordSlot) bool {
	slot.mu.Lock()
	busy := slot.driving || slot.pending || slot.workerPending
	slot.mu.Unlock()
	if busy || slot.workers.Load() > 0 {
		return false
	}
	return !r.sessionTurnBusy(coordSessionID) && r.turns.Waiting(coordSessionID) == 0
}

// stopCoordinatorWorkers cancels every still-running worker of a coordinator
// whose node gave up (timeout / cancellation), so a failed node leaves no
// detached worker turns burning budget behind it. Best-effort.
func (r *Runtime) stopCoordinatorWorkers(ctx context.Context, coordSessionID string) {
	// Whole subtree, not just the direct workers: a failed node that leaves its
	// sub-coordinators' workers running keeps burning budget for a flow run that has
	// already given up.
	r.stopSubtree(ctx, coordSessionID, "coordinator node gave up")
}

// isFlowCoordinatorSession reports whether a session id names a coordinator
// session owned by a flow's coordinator node. An unreadable/missing session
// reads as false, so an ordinary coordinator is never silently skipped.
func (r *Runtime) isFlowCoordinatorSession(ctx context.Context, sessionID string) bool {
	if strings.TrimSpace(sessionID) == "" {
		return false
	}
	s, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return false
	}
	return s.Kind == SessionKindFlowCoordinator
}

// lastAssistantText returns the text of the session's most recent assistant
// message ("" when there is none).
func (r *Runtime) lastAssistantText(ctx context.Context, sessionID string) string {
	msgs, err := r.db.ListMessages(ctx, sessionID)
	if err != nil {
		r.logger.Warn("coordinator node: cannot read reply", "session", sessionID, "error", err)
		return ""
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			return msgs[i].Text
		}
	}
	return ""
}
