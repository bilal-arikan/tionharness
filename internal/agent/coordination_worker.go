// Worker lifecycle under a coordinator: resolving the target agent, delivering follow-ups (send_to_worker), running the worker turn and its queued continuations, stopping it, listing workers, and recovering turns orphaned by a restart.
package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
)

// agentCoordinatorDefaults reports the coordinator DEFAULTS configured on a
// spawn_worker target (Agent.CoordinatorMode / CoordinatorWorkflow).
//
// A built-in profile target (explore/coder/validator…) never carries one: those
// are single-purpose leaf workers, and the agent materialized for one is created
// without the flag. An unresolvable target answers "no default" rather than an
// error — resolveWorkerTarget runs later on the same reference and is the single
// place that reports a bad target, so failing here would just duplicate (and
// reorder) that diagnosis.
func (r *Runtime) agentCoordinatorDefaults(ctx context.Context, target string) (mode bool, workflow string) {
	if _, isProfile := r.subagentProfile(strings.TrimSpace(target)); isProfile {
		return false, ""
	}
	a, err := r.resolveAgent(ctx, target)
	if err != nil {
		return false, ""
	}
	return a.CoordinatorMode, strings.TrimSpace(a.CoordinatorWorkflow)
}

// resolveWorkerTarget maps a spawn_worker target to a runnable persistent agent
// id. Existing agents pass through; built-in profiles resolve to system agents.
func (r *Runtime) resolveWorkerTarget(ctx context.Context, coordSessionID, baseAgentID, target string) (string, error) {
	target = strings.TrimSpace(target)
	prof, isProfile := r.subagentProfile(target)
	if !isProfile {
		// Ordinary target: must be an existing agent.
		a, err := r.resolveAgent(ctx, target)
		if err != nil {
			return "", err
		}
		return a.ID, nil
	}

	systemAgent, _, err := r.ResolveSystemAgent("subagent-" + prof.ID)
	if err != nil {
		return "", fmt.Errorf("cannot resolve worker profile %q system agent: %w", prof.ID, err)
	}
	if systemAgent.ID == "" {
		return "", fmt.Errorf("cannot resolve worker profile %q: system agent is not seeded", prof.ID)
	}
	allow, err := json.Marshal(prof.AllowedTools)
	if err != nil {
		return "", fmt.Errorf("cannot sync worker profile %q allowlist: %w", prof.ID, err)
	}
	if systemAgent.AllowedTools != string(allow) {
		if err := r.db.UpdateAgentAllowedTools(ctx, systemAgent.ID, string(allow)); err != nil {
			return "", fmt.Errorf("cannot sync worker profile %q allowlist: %w", prof.ID, err)
		}
	}
	return systemAgent.ID, nil
}

// SendToWorker appends a follow-up message to an existing worker session and runs
// its turn again (history-aware, so it continues with full context), notifying the
// coordinator on completion — the "continue" mechanism (Claude Code's SendMessage
// to a worker). The worker must belong to this coordinator.
func (r *Runtime) SendToWorker(ctx context.Context, coordSessionID, workerSessionID, message string) (tools.SendResult, error) {
	coordSessionID = strings.TrimSpace(coordSessionID)
	workerSessionID = strings.TrimSpace(workerSessionID)
	message = strings.TrimSpace(message)
	if workerSessionID == "" || message == "" {
		return tools.SendResult{}, fmt.Errorf("send_to_worker requires a worker session id and a message")
	}
	ws, err := r.db.GetSession(ctx, workerSessionID)
	if err != nil {
		return tools.SendResult{}, fmt.Errorf("worker session %s not found: %w", workerSessionID, err)
	}
	if ws.CoordinatorSessionID != coordSessionID {
		return tools.SendResult{}, fmt.Errorf("session %s is not a worker of this coordinator", workerSessionID)
	}
	agent, err := r.db.GetAgent(ctx, ws.AgentID)
	if err != nil {
		return tools.SendResult{}, fmt.Errorf("worker agent gone: %w", err)
	}
	// Follow-up turns re-read the agent row, so the code-side profile contract has
	// to be re-asserted here too — not only on the spawn path.
	if err := r.applyProfileAllowlist(&agent); err != nil {
		return tools.SendResult{}, err
	}

	// Recipient-side gate (size limit + inbound policy, inbound.go). Runs BEFORE
	// the queue/dispatch decision so a refused or held follow-up neither takes a
	// queue slot nor starts a turn, and the coordinator gets a durable receipt
	// either way instead of a bare error string.
	receipt, err := r.gateInbound(ctx, db.AgentMessage{
		FromName:    coordSessionID,
		ToAgentID:   agent.ID,
		ToSessionID: workerSessionID,
		Channel:     db.ChannelWorker,
		Body:        message,
	})
	if err != nil {
		return tools.SendResult{}, err
	}

	// Backpressure instead of rejection: a worker mid-turn no longer loses the
	// message. The busy-check and the enqueue are done under workerQueueMu in one
	// critical section so they stay atomic against drainWorkerQueue, which pops
	// under the same mutex once the turn ends (see runWorker's deferred drain).
	// The gate is workerTurnActive, not the generic isSessionActive: only a WORKER
	// registration is paired with that drain, so only it guarantees a message
	// accepted here is seen — a wake/user/peer turn registered on the same session
	// would otherwise keep this branch taken after the drain already ran, parking
	// the follow-up until some later worker turn.
	r.workerQueueMu.Lock()
	if r.workerTurnActive(workerSessionID) {
		queue := r.workerQueue[workerSessionID]
		if len(queue) >= maxWorkerQueueDepth {
			r.workerQueueMu.Unlock()
			return tools.SendResult{}, r.dropDelivery(ctx, receipt.ID, fmt.Errorf("worker %s already has %d queued messages (max %d); wait for them to be delivered before sending another",
				workerSessionID, len(queue), maxWorkerQueueDepth))
		}
		r.workerQueue[workerSessionID] = append(queue, message)
		r.workerQueueMu.Unlock()
		return tools.SendResult{Queued: true, ReceiptID: receipt.ID, RunningForSeconds: r.workerRunningForSeconds(workerSessionID)}, nil
	}
	r.workerQueueMu.Unlock()

	// Worker is idle: deliver immediately (same depth-aware slot reservation as a
	// fresh spawn — continuing a deep worker drains the pool just as a spawn does).
	if err := r.dispatchWorkerTurn(ctx, agent, workerSessionID, message, coordSessionID, ws.CoordinatorDepth); err != nil {
		return tools.SendResult{}, r.dropDelivery(ctx, receipt.ID, err)
	}
	return tools.SendResult{Delivered: true, ReceiptID: receipt.ID}, nil
}

// dispatchWorkerTurn reserves a background slot, records the follow-up as an
// injected user note, and launches the worker's turn goroutine. Shared by the
// immediate send_to_worker path and the queued-message drain; the busy / queue
// guard lives in the callers. The workerRunFn seam replaces the goroutine in
// tests so the queue's accept/refuse/deliver logic runs without a live provider.
func (r *Runtime) dispatchWorkerTurn(ctx context.Context, agent db.Agent, workerSessionID, message, coordSessionID string, depth int) error {
	if !r.acquireSpawnSlotAtDepth(depth) {
		return fmt.Errorf("background turn limit reached; try again once some finish")
	}
	// Credit the coordinator that wrote this follow-up so the worker transcript
	// shows it as an incoming message FROM that agent, the same way a peer inbox
	// delivery renders (TSK507). Best-effort: if the coordinator's agent cannot be
	// resolved the note still lands, just unattributed.
	if _, err := r.recordAgentAuthoredNote(ctx, workerSessionID, r.sessionAgentID(ctx, coordSessionID), agent.ID, message); err != nil {
		r.releaseSpawnSlot()
		return err
	}
	slot := r.coordSlotFor(coordSessionID)
	slot.workers.Add(1)
	slot.markHadWorkers()
	// Same fresh-transition re-arm as the spawn path: a continued worker puts the
	// fleet back to running, so the next all-idle must be reported again.
	slot.mu.Lock()
	slot.ackedIdle = false
	slot.idleFolded = false
	slot.mu.Unlock()
	if r.workerRunFn != nil {
		// Test seam: the caller counts the slots, so mirror the real path's release.
		// It runs SYNCHRONOUSLY, so there is no launch race to close here — keep the
		// bare ctl registration rather than newWorkerRun's cancellable run.
		ctl := &workerCtl{startedAt: time.Now(), done: make(chan struct{})}
		r.workerCancels.Store(workerSessionID, ctl)
		defer r.releaseSpawnSlot()
		defer slot.workers.Add(-1)
		defer r.workerCancels.CompareAndDelete(workerSessionID, ctl)
		defer close(ctl.done)
		r.workerRunFn(agent, workerSessionID, message, coordSessionID)
		return nil
	}
	// Registered before the goroutine starts: SendToWorker returns to the
	// coordinator's tool loop immediately, and a stop_worker in the next iteration
	// must be able to cancel this turn even while it is still queued.
	runCtx, cancelRun, ctl := r.newWorkerRun(workerSessionID)
	if !r.startBackgroundTurn(func() {
		r.runWorkerRegistered(runCtx, cancelRun, agent, workerSessionID, message, coordSessionID, ctl)
	}) {
		// The workspace is closing and no turn was launched: undo everything this call
		// registered, in the reverse order the turn itself would have released it.
		cancelRun()
		ctl.run.release()
		r.workerCancels.CompareAndDelete(workerSessionID, ctl)
		close(ctl.done)
		slot.workers.Add(-1)
		r.releaseSpawnSlot()
		return errSpawnQueueShutdown
	}
	return nil
}

// workerRunningForSeconds returns how long the worker's in-flight turn has been
// running, or 0 when that is unknown (no live workerCtl — e.g. a turn opened
// directly on the worker session rather than through the coordinator). Reported
// to the coordinator so a queued send conveys "busy, not stuck".
func (r *Runtime) workerRunningForSeconds(workerSessionID string) int64 {
	if v, ok := r.workerCancels.Load(workerSessionID); ok {
		if secs := int64(time.Since(v.(*workerCtl).startedAt).Seconds()); secs > 0 {
			return secs
		}
	}
	return 0
}

// hasQueuedMessage reports whether a follow-up is parked in the worker's queue
// (surfaced to the coordination UI as a "queued" badge).
func (r *Runtime) hasQueuedMessage(workerSessionID string) bool {
	r.workerQueueMu.Lock()
	queue := r.workerQueue[workerSessionID]
	r.workerQueueMu.Unlock()
	return len(queue) > 0
}

// drainWorkerQueue delivers a follow-up parked while the worker was mid-turn. It
// runs as runWorker's LAST deferred action — after every slot release and after
// the worker released its registration, so workerTurnActive is already false and
// the delivery re-runs
// the worker cleanly. The pop is done under workerQueueMu (the same mutex
// SendToWorker enqueues under) so an enqueue that raced the turn end is either
// fully visible here or already took the idle path. Runs on context.Background:
// the worker turn's ctx is cancelled by now.
func (r *Runtime) drainWorkerQueue(agent db.Agent, workerSessionID, coordSessionID string, ctl *workerCtl) {
	r.workerQueueMu.Lock()
	defer r.workerQueueMu.Unlock()
	messages, ok := r.workerQueue[workerSessionID]
	if !ok {
		return
	}
	// stop_worker is terminal for this run. Drop queued follow-ups while teardown
	// owns the session; launching one with context.Background would create a fresh
	// writer after StopWorker had already confirmed the old goroutine stopped.
	if ctl != nil && ctl.stopped.Load() {
		delete(r.workerQueue, workerSessionID)
		return
	}
	if ctl != nil && ctl.teardown.Load() {
		return
	}
	if r.workerDrainBeforeDispatch != nil {
		r.workerDrainBeforeDispatch()
	}
	delete(r.workerQueue, workerSessionID)
	message := strings.Join(messages, "\n\n---\n\n")
	ctx := context.Background()
	depth := 0
	if ws, err := r.db.GetSession(ctx, workerSessionID); err == nil {
		depth = ws.CoordinatorDepth
	}
	if err := r.dispatchWorkerTurn(ctx, agent, workerSessionID, message, coordSessionID, depth); err != nil {
		// The queued turn could not be launched (pool exhausted / DB error). Surface
		// it to the coordinator rather than dropping the message silently, so it can
		// react instead of waiting forever for a notification that will never come.
		r.NotifyCoordinator(coordSessionID, fmt.Sprintf(
			"<task-notification worker=%q status=\"failed\">Queued follow-up to worker %s could not be delivered: %v</task-notification>",
			workerSessionID, workerSessionID, err))
	}
}

// StopWorker cancels an in-flight worker turn. The worker's current turn ends and
// reports "killed" to the coordinator; the worker session survives and can be
// continued later with send_to_worker.
//
// When the target is a SUB-COORDINATOR its whole subtree is cancelled too. Left
// running, its grandchildren would keep spending budget and then notify a node
// whose task the coordinator has already written off — waking zombie turns under
// a branch nobody is waiting on.
//
// Stopping a sub-coordinator that is idle between its own turns is legitimate
// (that is exactly when it is waiting on its workers), so a target with a live
// subtree is accepted even when it has no turn of its own in flight.
func (r *Runtime) StopWorker(ctx context.Context, coordSessionID, workerSessionID string) error {
	workerSessionID = strings.TrimSpace(workerSessionID)
	if workerSessionID == "" {
		return fmt.Errorf("stop_worker requires a worker session id")
	}
	ws, err := r.db.GetSession(ctx, workerSessionID)
	if err != nil {
		return fmt.Errorf("worker session %s not found: %w", workerSessionID, err)
	}
	if coordSessionID != "" && ws.CoordinatorSessionID != coordSessionID {
		return fmt.Errorf("session %s is not a worker of this coordinator", workerSessionID)
	}
	// subtreeLive stays true when the subtree could not be read: stopping is the safe
	// direction, but claiming the worker had "already finished" over an unreadable
	// branch is not.
	subtreeLive := false
	if ws.IsCoordinator() {
		running, err := r.activeSubtreeWorkers(ctx, workerSessionID)
		if err != nil {
			r.logger.Warn("coordination: stopping worker with unknown subtree state",
				"session", workerSessionID, "error", err)
			subtreeLive = true
		} else {
			subtreeLive = running > 0
		}
		r.stopSubtree(ctx, workerSessionID, "stop_worker on their sub-coordinator")
	}
	_, wasRunning := r.workerCancels.Load(workerSessionID)
	r.workerQueueMu.Lock()
	v, ok := r.workerCancels.Load(workerSessionID)
	if !ok {
		r.workerQueueMu.Unlock()
		if wasRunning {
			return nil
		}
		if subtreeLive {
			// The sub-coordinator itself was between turns; its branch is what was
			// actually running and we just cancelled it. Report that truthfully
			// instead of the misleading "already finished?".
			return nil
		}
		// No cancellable turn. For a worker that already reached a terminal state that
		// is the expected race and reported as such below. A worker still marked
		// "running" is different: either it is unwinding right now (its own terminal
		// write is moments away) or its registration was lost — answering "already
		// finished" would be a plain false statement about a session the coordinator
		// can still see running. Say what is actually known and let it re-read.
		if ws.RunState == runStateRunning {
			return fmt.Errorf("worker %s is marked running but has no cancellable turn in this process; it is most likely finishing right now — re-read the session before retrying", workerSessionID)
		}
		return fmt.Errorf("worker %s is not running (already finished?): %w", workerSessionID, errWorkerNotRunning)
	}
	ctl := v.(*workerCtl)
	ctl.stopped.Store(true)
	ctl.cancel()
	r.workerQueueMu.Unlock()
	if ctl.done == nil {
		return nil
	}
	select {
	case <-ctl.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("worker %s did not stop: %w", workerSessionID, ctx.Err())
	}
}

// WorkerInfo is a coordinator-facing snapshot of one worker session.
type WorkerInfo struct {
	SessionID string
	AgentName string
	// AgentID and the AgentAvatar/AgentColor/AgentProvider/AgentModel/AgentDeleted
	// fields carry the worker agent's visual identity so a UI can render it with
	// the same agent-identity component it uses everywhere else (avatar, name, id,
	// resolved model label) instead of a bare name string. They stay EMPTY when
	// the agent row cannot be read (deleted and purged, or a session whose agent
	// no longer exists): that is not an error here — the roster still lists the
	// worker, and the UI falls back to the session title/id.
	AgentID       string
	AgentAvatar   string
	AgentColor    string
	AgentProvider string
	AgentModel    string
	// AgentDeleted marks a soft-deleted agent, whose sessions outlive it; the UI
	// badges the identity instead of silently showing a normal-looking agent.
	AgentDeleted bool
	Title        string
	Running      bool
	Summary      string // first line of the worker's latest reply, when finished
	// StartedAt is the unix-second stamp of when a RUNNING worker's current turn
	// began, so the UI can show live elapsed time. Zero when the worker is not
	// running, or when it is active without a workerCtl (e.g. a turn opened
	// directly on the worker session rather than through the coordinator) — the
	// start time is then genuinely unknown and the UI omits the duration.
	StartedAt int64
	// Delegating marks a SUB-COORDINATOR that is running only in the sense that its
	// own workers are: it has no turn of its own in flight, it is waiting on its
	// branch. The UI shows this differently ("delegating") because "running" would
	// suggest a live turn whose elapsed time is meaningful.
	Delegating bool
	// Queued reports that a follow-up (send_to_worker) is parked in this worker's
	// single-slot queue, waiting for its current turn to finish. Only meaningful
	// while Running: the UI shows a "queued" badge so the coordinator can see the
	// message landed and will be delivered, rather than assuming it was lost.
	Queued bool
	// CreatedAt / UpdatedAt mirror the worker session's own timestamps so a
	// listing can be sorted by them.
	CreatedAt int64
	UpdatedAt int64
	// Stuck reports whether the worker session carries the "stuck" tag (the
	// liveness signal the session-watchdog stamps on a session that stops making
	// progress). Drives the list_workers state filter.
	Stuck bool
}

// ListWorkers returns the workers spawned under a coordinator session, newest
// first, each tagged running or finished (with a one-line summary of its last
// reply).
func (r *Runtime) ListWorkers(ctx context.Context, coordSessionID string) ([]WorkerInfo, error) {
	sessions, err := r.db.ListSessions(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []WorkerInfo
	for _, s := range sessions {
		if s.CoordinatorSessionID != coordSessionID {
			continue
		}
		out = append(out, r.workerInfoFor(ctx, s))
	}
	return out, nil
}

// hasSessionTag reports whether a session's tag list contains the given tag.
func hasSessionTag(tags []string, want string) bool {
	return slices.Contains(tags, want)
}

// workerInfoFor snapshots one worker session for a coordinator-facing listing.
// Shared by ListWorkers (direct children) and ListSubtreeWorkers (every
// descendant) so both report liveness and summaries identically.
//
// A sub-coordinator counts as RUNNING while any of its own workers is running,
// even when it has no turn of its own in flight: between its turns it is waiting
// on its branch, and reporting it as finished there is exactly how a parent
// concludes on top of work that is still in progress.
//
// This live view is the ONLY channel that carries that state now — the interim
// "delegating" note the runtime used to push into the parent was removed (it woke
// the parent for a non-result), so anything reading a sub-coordinator's liveness
// must read it from here. See subCoordinatorBusy.
func (r *Runtime) workerInfoFor(ctx context.Context, s db.Session) WorkerInfo {
	info := WorkerInfo{
		SessionID: s.ID,
		AgentName: r.agentName(s.AgentID),
		AgentID:   s.AgentID,
		Title:     s.Title,
		Running:   r.isSessionActive(s.ID),
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Stuck:     hasSessionTag(s.Tags, "stuck"),
	}
	// The agent row is read once here for the identity fields. A miss is expected
	// and NOT swallowed silently: the worker keeps its session id + title, and the
	// identity fields stay empty so the UI can tell "no agent" from "no avatar".
	if a, err := r.db.GetAgent(ctx, s.AgentID); err == nil {
		info.AgentAvatar = a.Avatar
		info.AgentColor = a.Color
		info.AgentProvider = a.Provider
		info.AgentModel = a.Model
		info.AgentDeleted = a.Deleted
	}
	if !info.Running && s.IsCoordinator() && r.subCoordinatorBusy(s) {
		info.Running = true
		info.Delegating = true
	}
	if info.Running {
		if v, ok := r.workerCancels.Load(s.ID); ok {
			info.StartedAt = v.(*workerCtl).startedAt.Unix()
		}
		info.Queued = r.hasQueuedMessage(s.ID)
	} else if msgs, err := r.db.ListMessages(ctx, s.ID); err == nil {
		for _, msg := range slices.Backward(msgs) {
			if msg.Role == "assistant" {
				info.Summary = notifyLine(msg.Text, 120)
				break
			}
		}
	}
	return info
}

// newWorkerRun creates the worker turn's cancellable context and registers BOTH
// stop paths — the coordinator's workerCancels entry and the session cancel
// registry CancelSession reads — before the turn goroutine is started. The launch
// sites call this SYNCHRONOUSLY: SpawnWorker/SendToWorker return the worker session
// id to the caller's tool loop the moment they return, so a stop issued in the very
// next iteration must find something to cancel. Registering inside the goroutine
// left that window (unbounded, because the turn-slot claim can queue) uncancellable
// while the row already read "running", so stop_worker answered "already finished"
// for a worker that then went on to run.
//
// The ctl's initial cancel is cancelRun itself, so a stop landing before the first
// turn attempt registers its own cancel still tears the run down; the idle-resume
// loop re-points it per attempt (see setCancel).
func (r *Runtime) newWorkerRun(workerSessionID string) (context.Context, context.CancelFunc, *workerCtl) {
	runCtx, cancelRun := context.WithCancel(context.Background())
	ctl := &workerCtl{startedAt: time.Now(), done: make(chan struct{})}
	ctl.setCancel(cancelRun)
	ctl.run = r.trackWorkerSession(workerSessionID, cancelRun)
	r.workerCancels.Store(workerSessionID, ctl)
	return runCtx, cancelRun, ctl
}

func (r *Runtime) runWorkerRegistered(runCtx context.Context, cancelRun context.CancelFunc, agent db.Agent, workerSessionID, prompt, coordSessionID string, ctl *workerCtl) {
	defer r.workerCancels.CompareAndDelete(workerSessionID, ctl)
	defer close(ctl.done)
	r.runWorkerWithCtl(runCtx, cancelRun, agent, workerSessionID, prompt, coordSessionID, ctl)
}

func (r *Runtime) runWorkerWithCtl(runCtx context.Context, cancelRun context.CancelFunc, agent db.Agent, workerSessionID, prompt, coordSessionID string, ctl *workerCtl) {
	// Registered first so the global lifecycle slot is released last. Test/runtime
	// shutdown uses spawnActive as the definitive drain barrier; dropping it before
	// queue finalization lets cleanup race the goroutine's final store access.
	defer r.releaseSpawnSlot()
	// Drain after every per-turn slot/cancel/tracking cleanup but before releasing
	// the global lifecycle slot. A parked follow-up can then start from an idle
	// worker while shutdown still sees this goroutine as active. No-op when empty.
	defer r.drainWorkerQueue(agent, workerSessionID, coordSessionID, ctl)
	// Leak backstop, registered AFTER the drain defer so it runs BEFORE it: the
	// worker's registration must be gone by the time the drain pops, otherwise a
	// follow-up racing the drain is accepted into the queue and never delivered.
	// The explicit releases below already do this at the old untrack points; this
	// only covers the early returns. release is idempotent.
	defer ctl.run.release()
	// Released exactly once, and BEFORE the terminal notification rather than in a
	// defer: NotifyCoordinator decides whether this is the last worker (and may fold
	// the all-idle note into its message) by reading this counter, so a worker that
	// still counted itself as active would make that check permanently false. The
	// deferred call is the safety net for every early return above the explicit one.
	workerDone := releaseOnce(r.coordSlotFor(coordSessionID))
	defer workerDone()

	// Hard wall-clock ceiling (settings-driven, same as spawns) PLUS an idle
	// watchdog: a worker/coordinator turn that streams no step for SpawnIdleTimeout
	// is reclaimed fast, while a long-but-productive one runs up to SpawnTimeout.
	hardCap, idleCap := time.Duration(0), r.tun.SpawnIdleTimeout()
	// Serialize this worker turn on the WORKER session's own turn slot (keyed by
	// workerSessionID, distinct from the coordinator slot whose workers counter is
	// decremented above) so it never overlaps another turn on the same worker
	// session: a second send_to_worker that raced the isSessionActive check (that
	// check is a UI hint, not a lock), or a user/wake/peer turn opened on the worker
	// session (all of which now claim this same slot). The claim watches runCtx (the
	// worker turn's own cancellable context, created and registered by newWorkerRun
	// before this goroutine started): a stop issued while this turn waits in the
	// queue must not be outlived by it.
	releaseSlot, slotErr := r.claimSessionTurnSlotCtx(runCtx, workerSessionID, turnqueue.KindWorker, "worker görevi")
	defer releaseSlot()
	defer cancelRun()
	if slotErr != nil {
		// Stopped while waiting for the worker session's turn slot: the turn never ran,
		// so stamp the terminal state and tell the coordinator instead of starting work
		// nobody is waiting for. "killed", not "failed": a later retry_of must not be
		// refused as "still running", and this genuinely was a stop.
		r.logger.Info("worker: cancelled before its turn started",
			"session", workerSessionID, "coordinator", coordSessionID)
		// context.Background() deliberately: runCtx is already cancelled and the
		// terminal state must still be persisted.
		bg := context.Background()
		if rsErr := r.db.SetSessionRunState(bg, workerSessionID, turnStatusKilled, time.Now().Unix()); rsErr != nil {
			r.logger.Error("worker: failed to persist killed state", "session", workerSessionID, "error", rsErr)
		}
		ctl.run.release()
		r.emitWorkerEvent(agent, workerSessionID, coordSessionID, turnStatusKilled)
		// The coordinator is waiting on this worker whatever happened to it, so the
		// kill is reported like any other outcome — dropping it would freeze the
		// coordinator on a worker that will never speak.
		note := formatTaskNotification(workerSessionID, agent.ID, agent.Name, agent.Model, turnStatusKilled,
			"⏹️ Worker turu, sırası gelmeden durduruldu.", 0, 0)
		if notifyErr := r.notifyCoordinator(coordSessionID, note, workerDone(), nil, workerSessionID); notifyErr != nil {
			r.logger.Error("worker: kill notification failed",
				"session", workerSessionID, "coordinator", coordSessionID, "error", notifyErr)
		}
		return
	}

	// Raise the "thinking" indicator for the worker session (see emitTurnStart);
	// the completion "worker" event clears it.
	r.emitTurnStart(workerSessionID, "🤝 Worker turu çalışıyor")
	// Tell the coordination UI a worker is now running (covers both the initial
	// spawn and a send_to_worker continuation, since both land here).
	r.emitWorkerStartEvent(agent, workerSessionID, coordSessionID)

	// Single-shot idle-resume (FND-708844f8): an idle-cut worker turn — the exact
	// SES17 case this file was written for — gets ONE more attempt under a fresh
	// window before it reports "timeout"/unfinished up to its coordinator, which then
	// re-tasks it (the SECOND line of defence, unchanged). setCancel re-points the
	// coordinator's stop_worker at whichever attempt is in flight; the stopped
	// re-check closes the tiny gap before the first attempt registers its cancel.
	var (
		turnCtx context.Context
		meta    *turnMeta
	)
	// One stash for the whole run, shared by every idle-resume attempt: a
	// report_to_coordinator call is held here and delivered from the terminal path
	// below, so the parent is not woken while this node is still mid-turn.
	upward := &pendingUpwardReport{}
	// Backstop for the paths that return before the explicit flushes (a panic, or a
	// future early return): the claim is already spent, so the note must go out
	// whatever happens. take() makes the second call a no-op.
	defer r.flushUpwardReport(upward, workerSessionID)
	turnStart := time.Now()
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(runCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, cancel context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			ctl.setCancel(cancel)
			if ctl.stopped.Load() {
				return "", nil, context.Canceled
			}
			turnCtx = tools.WithAsyncChat(WithSessionID(WithCallKind(attemptCtx, KindSpawn), workerSessionID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			turnCtx = withPendingUpwardReport(turnCtx, upward)
			p := prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(prompt, prevOutput)
			}
			return r.runSessionTurn(turnCtx, agent, workerSessionID, p, true)
		})
	defer cancel()
	ctl.run.release()

	// Why the turn ended, independent of err: a watchdog cancellation (hard cap or
	// idle) and the loop's own terminal markers (iteration cap, guardrail halt,
	// context/output exhaustion) all yield truncated work that the provider may
	// still hand back as a nil-error "answer". Without this, such a turn was
	// reported to the coordinator as completed and the coordinator moved on.
	outcome := classifyTurnOutcome(ctx, steps, hardCap, idleCap)

	status := outcome.Status
	replyText := output
	if err != nil {
		switch {
		case ctl.stopped.Load():
			status = turnStatusKilled
			replyText = "⏹️ Worker turu koordinatör tarafından durduruldu."
		// A deadline check must precede the plain-cancel check: WithActivityTimeout
		// cancels the context, so an expired turn ALSO satisfies context.Canceled and
		// would otherwise be misreported as a clean human stop.
		case outcome.Status == turnStatusTimeout:
			status = turnStatusTimeout
			replyText = outcome.Note
			steps = appendOutcomeStep(steps, outcome)
		case errors.Is(err, context.Canceled):
			// A viewer pressed "Durdur"/"Kes": the autonomous run's cancel aborted the
			// turn (see autonomousInteraction). Report it as a clean stop, not a failure.
			status = turnStatusKilled
			replyText = "⏹️ Worker turu durduruldu."
		default:
			status = turnStatusFailed
			replyText = "⚠️ Worker turu çalıştırılamadı:\n\n" + err.Error()
		}
		r.logger.Warn("worker: turn ended", "session", workerSessionID, "coordinator", coordSessionID, "status", status, "error", err)
	} else {
		if strings.TrimSpace(replyText) == "" && !outcome.Truncated() {
			replyText = "ℹ️ Worker bu tur için boş yanıt döndürdü."
		}
		// Salvaged text from a cut-short turn: keep it (it is the only record of how
		// far the work got) but lead with the note so it is never read as a result.
		replyText = applyTurnOutcome(replyText, outcome)
		steps = appendOutcomeStep(steps, outcome)
		if outcome.Truncated() {
			r.logger.Warn("worker: turn truncated", "session", workerSessionID, "coordinator", coordSessionID,
				"status", status, "hardCap", hardCap, "idleCap", idleCap)
			// turnCtx, not ctx: only the turn context carries the session id the
			// debug journal keys on (ctx would silently drop the event).
			r.emitDebug(turnCtx, db.DebugEvent{Type: db.DebugError, AgentID: agent.ID, Detail: "worker turn " + status, Err: true})
		}
	}

	status, replyText = guardWorkerVerificationClaim(status, replyText, steps)
	if workerSession, getErr := r.db.GetSession(context.WithoutCancel(ctx), workerSessionID); getErr == nil {
		var delivery *TurnStep
		status, replyText, delivery = checkWorkerDeliverables(status, replyText, workerSession.ExpectedDeliverables)
		if delivery != nil {
			steps = append(steps, *delivery)
			if emit := r.SessionStepEmitter(turnCtx); emit != nil {
				emit(*delivery)
			}
		}
	} else {
		status = turnStatusFailed
		replyText = "Worker delivery contract could not be read; completion is not established: " + getErr.Error()
	}
	// replyText was pre-composed above (success output / failure / kill / empty note).
	if addErr := r.recordAssistantMessage(ctx, workerSessionID, agent.ID, replyText, steps, meta, time.Since(turnStart).Milliseconds()); addErr != nil {
		r.logger.Warn("worker: failed to record reply", "session", workerSessionID, "error", addErr)
		status = turnStatusFailed
		replyText = "Worker execution record could not be persisted; completion is not established: " + addErr.Error()
	}
	// Persist the terminal outcome AFTER AddMessage updates the shared lifetime
	// counters. mutateSessionLocked rewrites session.json, making the same
	// MessageCount/ToolCallCount arithmetic used by chat durable for worker turns.
	// The error is surfaced because a missing terminal write must stay observable.
	if rsErr := r.db.SetSessionRunState(ctx, workerSessionID, status, time.Now().Unix()); rsErr != nil {
		r.logger.Error("worker: failed to persist terminal session state", "session", workerSessionID, "status", status, "error", rsErr)
		status = turnStatusFailed
		replyText = "Worker terminal state could not be persisted; completion is not established: " + rsErr.Error()
	}
	r.emitWorkerEvent(agent, workerSessionID, coordSessionID, status)

	// A SUB-COORDINATOR that just fanned its work out is not finished, whatever its
	// turn returned: reporting this turn as "completed" would tell its coordinator
	// the subtask is done while the branch below has barely started. Withhold the
	// completion notification and send NOTHING at all — the parent must not burn a
	// turn on a non-result. Its live worker view keeps showing this node as
	// delegating (subCoordinatorBusy reads the pending-report flag set here), and
	// the node closes its own task later (report_to_coordinator / settle backstop).
	// See coordination_tree.go for the full contract.
	if ws, err := r.db.GetSession(ctx, workerSessionID); err == nil && r.deferWorkerReport(ctx, ws, status) {
		// This node is NOT finished (its own branch is still running), but its worker
		// turn is: release the slot so the parent's fleet count reflects reality.
		workerDone()
		// A report_to_coordinator made during THIS turn still goes out — it is the
		// node speaking for itself, which is exactly what the deferral waits for.
		r.flushUpwardReport(upward, workerSessionID)
		return
	}

	// The report the coordinator actually reads: the successful output, or the
	// failure/kill note (so the coordinator can react to failures too). buildWorkerResult
	// bounds how much of it enters the coordinator's context — an overflowing result is
	// capped and its full text offloaded to an artifact + worker-session handle, so a
	// single verbose worker can no longer fill the coordinator's window (_Docs/47, P0/P2).
	// A report_to_coordinator made during this turn folds into the terminal note
	// (its status may override a clean turn's); sending both woke the parent twice
	// with the same result. See pendingUpwardReport.foldIntoTerminal.
	status = upward.foldIntoTerminal(status)
	notifyResult := r.buildWorkerResult(ctx, workerSessionID, agent.ID, status, replyText)
	note := formatTaskNotification(workerSessionID, agent.ID, agent.Name, agent.Model, status, notifyResult, countToolSteps(steps), time.Since(turnStart).Milliseconds())
	// Release BEFORE notifying: this worker is done, and whether it took the fleet to
	// zero decides if the all-idle note folds into this very message (saving the
	// coordinator a separate reconcile turn).
	// The coordinator's copy of the trace is DIGESTED to the file changes its
	// notification card renders. The worker's own session keeps the full trace and
	// the notification names it, so nothing is lost — see digestWorkerSteps.
	if notifyErr := r.notifyCoordinator(coordSessionID, note, workerDone(), digestWorkerSteps(steps), workerSessionID); notifyErr != nil {
		r.logger.Error("worker: terminal notification failed", "session", workerSessionID, "coordinator", coordSessionID, "error", notifyErr)
	}
	// The stash was consumed by the fold above; this flush is a no-op unless a
	// report landed between the fold and here (a late tool call on a cut-short
	// attempt) — then it still goes out rather than being lost.
	r.flushUpwardReport(upward, workerSessionID)
}

// RecoverOrphanedTurns reclaims autonomous background turns (worker / plain spawn /
// coordinator) that were mid-flight when the process died. Those turns run as
// fire-and-forget goroutines with no crash sidecar, so a restart kills them
// silently: the session is left with a trailing USER message and no reply, and —
// for a worker — the coordinator is never notified and waits forever (the SES28
// freeze). At boot this, per orphaned session:
//
//   - records an INTERRUPTED assistant reply, so the session no longer looks frozen
//     mid-turn (and a second boot skips it — the last message is now an assistant);
//   - for a worker, injects a synthetic <task-notification status="killed"> into its
//     coordinator, which persists it + enqueues a coordinator turn → the coordinator
//     stops waiting and can react (re-dispatch or conclude);
//   - for a coordinator orphaned mid-turn (trailing user notification, no reply),
//     re-enqueues one coordinator turn so it resumes from full history.
//
// Best-effort and idempotent. Detection is "an autonomous session whose LAST message
// is a user turn" — at boot nothing is running, so that is exactly a killed mid-turn.
func (r *Runtime) RecoverOrphanedTurns(ctx context.Context) {
	sessions, err := r.db.ListSessions(ctx, "")
	if err != nil {
		// Nothing is reclaimed on this boot: every session killed mid-turn stays
		// frozen and each orphaned worker's coordinator waits forever. That must not
		// be a silent early return.
		r.logger.Error("coordination: orphan recovery skipped; session list failed", "error", err)
		r.emitDebug(ctx, db.DebugEvent{
			Type:   db.DebugError,
			Name:   "orphan_recovery_failed",
			Detail: "orphaned-turn recovery skipped: session list failed",
			Error:  err.Error(),
			Err:    true,
		})
		return
	}
	// Shallowest first, so a coordinator TREE is reclaimed from the root down. The
	// order is load-bearing: a mid-level node is both a worker and a coordinator, so
	// when we reach a child its parent has already been reclaimed and recorded in
	// `reclaimed` below — and we can skip notifying a node that is itself dead
	// instead of waking a zombie turn on it.
	slices.SortStableFunc(sessions, func(a, b db.Session) int {
		return cmp.Compare(a.CoordinatorDepth, b.CoordinatorDepth)
	})
	reclaimed := map[string]bool{}
	for _, sess := range sessions {
		if sess.State == "archived" {
			continue // do not resurrect work the user has archived
		}
		isWorker := sess.IsWorker()
		// A flow's coordinator node owns its coordinator session (kind
		// "flow-coordinator"). Re-enqueueing a turn on it would race the flow
		// runner, which resumes the owning run and re-executes the node against a
		// FRESH coordinator session — so the orphan is left alone here.
		isCoordinator := sess.IsCoordinator() && sess.Kind != SessionKindFlowCoordinator
		// A spawn_session child is a "chat" by kind (TSK1005), so the spawn test also
		// reads the lineage: a spawn-origin chat still ran a detached background turn.
		isSpawn := sess.Kind == "spawned" || sess.Kind == "worker" || sess.Lineage().Kind == db.OriginSpawn
		if !isWorker && !isCoordinator && !isSpawn {
			continue // ordinary interactive/inbox session — not an autonomous turn
		}
		// At boot the in-memory active set is empty; this only guards a late call.
		if r.isSessionActive(sess.ID) {
			continue
		}
		msgs, err := r.db.ListMessages(ctx, sess.ID)
		if err != nil || len(msgs) == 0 {
			continue
		}
		if last := msgs[len(msgs)-1]; last.Role != "user" || last.Origin == noteOriginGate {
			// Completed normally (last message is an assistant reply), or the
			// transcript ends with a record that owes no reply (a resolved human
			// gate, see noteOriginGate) — either way there is no turn to reclaim.
			continue
		}
		switch {
		case isWorker:
			r.recordInterruptedReply(ctx, sess, "⏹️ Worker turu süreç yeniden başlarken yarıda kaldı (kurtarıldı).")
			reclaimed[sess.ID] = true
			// Tell the coordinator so it stops waiting and can re-dispatch or conclude.
			// Skipped in two cases, both because the target cannot act on it:
			//   - the coordinator belongs to a flow's coordinator node, which is
			//     abandoned on restart (the node re-runs with a fresh session);
			//   - the coordinator is a mid-level node THIS sweep already reclaimed and
			//     reported dead upward — notifying it would wake a zombie turn on a
			//     branch its own coordinator has already written off.
			switch {
			case reclaimed[sess.CoordinatorSessionID]:
				r.logger.Info("recover: skipping notify, coordinator was reclaimed too",
					"session", sess.ID, "coordinator", sess.CoordinatorSessionID)
			case r.isFlowCoordinatorSession(ctx, sess.CoordinatorSessionID):
			default:
				note := formatTaskNotification(sess.ID, sess.AgentID, r.agentName(sess.AgentID), sess.Model, "killed",
					"Worker turu süreç yeniden başlatılırken (crash/restart) yarıda kaldı; sonuç üretilemedi. Gerekirse yeniden görevlendir.", 0, 0)
				r.NotifyCoordinator(sess.CoordinatorSessionID, note)
			}
			r.logger.Info("recover: orphaned worker reclaimed", "session", sess.ID, "coordinator", sess.CoordinatorSessionID)
		case isCoordinator:
			// Coordinator itself died mid-turn: re-run once so it resumes from history.
			r.enqueueCoordinatorTurn(sess.ID)
			r.logger.Info("recover: orphaned coordinator re-enqueued", "session", sess.ID)
		default: // plain spawn
			r.recordInterruptedReply(ctx, sess, "⚠️ Spawn turu süreç yeniden başlarken yarıda kaldı.")
			r.logger.Info("recover: orphaned spawn reclaimed", "session", sess.ID)
		}
	}
}

// recordInterruptedReply persists a crash-recovered assistant reply on an orphaned
// autonomous session so it reads as finished (Interrupted) instead of hanging on a
// trailing user message. Best-effort; a write failure is logged, not fatal.
func (r *Runtime) recordInterruptedReply(ctx context.Context, sess db.Session, text string) {
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID:   sess.ID,
		AgentID:     sess.AgentID,
		Role:        "assistant",
		Text:        text,
		Interrupted: true,
	}); err != nil {
		r.logger.Warn("recover: failed to record interrupted reply", "session", sess.ID, "error", err)
	}
}
