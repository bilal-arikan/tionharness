package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/skills"
	"github.com/bilal-arikan/tionswarm/internal/tools"
	"github.com/bilal-arikan/tionswarm/internal/view"
)

// coordination.go implements the M2 coordinator/worker method (see _Docs/47).
//
// A coordinator session spawns WORKER sessions (spawn_worker) that run detached,
// history-aware turns. When a worker's turn finishes (success, failure, or a
// stop_worker cancellation) it does NOT return into the caller's turn; instead it
// injects a <task-notification> user message into the coordinator session and
// asks the per-session turn queue to run one coordinator turn. Concurrent worker
// completions therefore serialize into single coordinator turns, and any that
// pile up while the coordinator is mid-turn coalesce into the next one (their
// notifications are already in history) — this is the whole point of coordSlot.

// coordSlot serializes coordinator turns for one coordinator session and counts
// its active workers. Guarded by mu except workers (atomic, touched from the
// spawn path without the turn lock).
type coordSlot struct {
	mu         sync.Mutex
	free       *sync.Cond   // lazily created; broadcast whenever running flips false
	running    bool         // a turn (auto OR interactive) is currently executing
	pending    bool         // >=1 notification arrived while running; run once more after
	ackedIdle  bool         // ran the "all workers idle" reconcile turn for this batch
	hadWorkers bool         // at least one worker was ever spawned (gates the idle sweep)
	turns      int          // auto-triggered coordinator turns so far (notify-loop cap)
	capWarn    bool         // whether the "cap reached" warning has been posted
	workers    atomic.Int64 // active workers under this coordinator
	// spawnHallucStreak counts consecutive coordinator turns judged to have CLAIMED a
	// spawn while making NO coordination tool call — the long-context degradation
	// freeze. Bounds the corrective nudges so a wedged model cannot burn the notify
	// loop; reset to 0 by any turn that actually calls a coordination tool. See
	// guardCoordinatorStall (coordination_stall.go).
	spawnHallucStreak int
	// lastTurnUnix is the wall-clock (unix seconds) at which this coordinator's last
	// real turn finished. 0 until the first real turn ran (a stubbed test never sets
	// it). The stall sweeper reads it to find coordinators gone silent past the
	// staleness window.
	lastTurnUnix int64
}

// signalFree wakes turns blocked in claimCoordinatorSlot. Callers must hold mu.
func (s *coordSlot) signalFree() {
	if s.free != nil {
		s.free.Broadcast()
	}
}

// markHadWorkers records that this coordinator has spawned at least one worker, so
// the idle-reconcile sweep only ever fires for a coordinator that actually has
// workers to reconcile (never for a plain no-worker session).
func (s *coordSlot) markHadWorkers() {
	s.mu.Lock()
	s.hadWorkers = true
	s.mu.Unlock()
}

// workerCtl lets stop_worker cancel an in-flight worker turn and mark it stopped
// so it reports "killed" rather than "failed". startedAt stamps when the turn
// began so ListWorkers can report a running worker's elapsed time.
type workerCtl struct {
	cancel    context.CancelFunc
	stopped   atomic.Bool
	startedAt time.Time
}

// sessionIsCoordinator reports whether the session stamped on ctx may drive
// workers (db.Session.IsCoordinator). False when no session is stamped or it
// can't be loaded. Note this is TRUE for a mid-level node too: since the
// unlimited-depth rework a worker session that has coordinator mode on drives its
// own workers while still reporting up to its parent.
func (r *Runtime) sessionIsCoordinator(ctx context.Context) bool {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return false
	}
	s, err := r.db.GetSession(ctx, sid)
	if err != nil {
		return false
	}
	return s.IsCoordinator()
}

// withCoordination wires the coordination runner for one turn. It always installs
// the runner (when a session is stamped on ctx) but populates only the functions
// this session may actually use; buildRegistry then registers each tool from the
// presence of its function. Three independent surfaces:
//
//   - Spawn/Send/Stop/List — coordinator mode is on. This is the gate that used to
//     cap a tree at one level: it tested Role=="coordinator", which a worker could
//     never hold, so a worker could not nest. It now tests the CAPABILITY, so a
//     worker spawned with coordinator:true (or one that turned the mode on itself)
//     drives its own workers. Recursion is bounded by the depth/subtree budgets in
//     SpawnWorker instead of by hiding the tools.
//   - Report — this session has a coordinator above it (it owes a result upward).
//   - SetMode — always, otherwise a plain session could never become a coordinator.
func (r *Runtime) withCoordination(ctx context.Context, caller db.Agent) context.Context {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return ctx
	}
	sess, err := r.db.GetSession(ctx, sid)
	if err != nil {
		return ctx
	}
	return tools.WithCoordination(ctx, r.coordinationFuncsFor(sess, caller.ID))
}

// coordinationFuncsFor builds the coordination runner bound to a session + caller.
// Shared by the native path (withCoordination) and the CLI bridge (BridgeTools),
// so both dispatch the coordination tools identically. Functions the session is
// not entitled to are left nil — that IS the per-tool gate.
func (r *Runtime) coordinationFuncsFor(sess db.Session, callerID string) *tools.CoordinationFuncs {
	coordID := sess.ID
	f := &tools.CoordinationFuncs{
		SetMode: func(c context.Context, enabled bool) (string, error) {
			return r.SetSessionCoordinatorMode(c, coordID, enabled)
		},
	}
	if sess.IsCoordinator() {
		f.Spawn = func(c context.Context, agentRef, task string, spec tools.WorkerSpawnSpec) (tools.SpawnResult, error) {
			ws, err := r.workerSpecFor(spec)
			if err != nil {
				return tools.SpawnResult{}, err
			}
			res, err := r.SpawnWorker(c, coordID, agentRef, task, callerID, ws)
			if err != nil {
				return tools.SpawnResult{}, err
			}
			return tools.SpawnResult{SessionID: res.SessionID, AgentName: res.AgentName}, nil
		}
		f.Send = func(c context.Context, workerSessionID, message string) (tools.SendResult, error) {
			return r.SendToWorker(c, coordID, workerSessionID, message)
		}
		f.Stop = func(c context.Context, workerSessionID string) error {
			return r.StopWorker(c, coordID, workerSessionID)
		}
		f.List = func(c context.Context, subtree bool) (string, error) {
			if subtree {
				ws, err := r.ListSubtreeWorkers(c, coordID)
				if err != nil {
					return "", err
				}
				return formatWorkerTree(ws), nil
			}
			ws, err := r.ListWorkers(c, coordID)
			if err != nil {
				return "", err
			}
			return formatWorkerList(ws), nil
		}
	}
	if sess.CoordinatorSessionID != "" {
		f.Report = func(c context.Context, status, summary string) error {
			return r.ReportToCoordinator(c, coordID, status, summary)
		}
	}
	return f
}

// workerSpecFor turns the tool-layer spawn options into the runtime spec,
// resolving a requested coordinator recipe through the same gate the UI and the
// flow node use. An unknown or wrong-kind slug fails the spawn rather than
// falling back to free coordination — a sub-coordinator silently running a
// different plan than it was given is worse than a refused spawn.
func (r *Runtime) workerSpecFor(spec tools.WorkerSpawnSpec) (WorkerSpec, error) {
	out := WorkerSpec{
		ModelOverride: spec.ModelOverride,
		Coordinator:   spec.Coordinator,
		Workflow:      strings.TrimSpace(spec.Workflow),
	}
	if out.Workflow != "" {
		maxTurns, err := skills.ResolveCoordinatorWorkflow(r.Skills(), out.Workflow)
		if err != nil {
			return WorkerSpec{}, err
		}
		out.WorkflowMaxTurns = maxTurns
	}
	return out, nil
}

// coordinationBridgeDefs returns the coordination tool defs for the CLI bridge,
// filtered to what this session is entitled to (mirroring buildRegistry's
// per-function gating, so the CLI advertises exactly the native tool set).
func coordinationBridgeDefs(f *tools.CoordinationFuncs) []providers.ToolDef {
	var defs []providers.ToolDef
	if f.Spawn != nil {
		defs = append(defs,
			tools.NewSpawnWorkerTool().Def(),
			tools.NewSendToWorkerTool().Def(),
			tools.NewStopWorkerTool().Def(),
			tools.NewListWorkersTool().Def(),
		)
	}
	if f.Report != nil {
		defs = append(defs, tools.NewReportToCoordinatorTool().Def())
	}
	if f.SetMode != nil {
		defs = append(defs, tools.NewSetCoordinatorModeTool().Def())
	}
	return defs
}

// dispatchCoordinationBridge handles a coordinator tool call on the CLI bridge:
// it wires the coordination runner into ctx and invokes the matching tool. The
// bool reports whether name was a coordination tool (so the caller can fall
// through to the normal registry dispatch otherwise).
func dispatchCoordinationBridge(ctx context.Context, f *tools.CoordinationFuncs, name string, args json.RawMessage) (string, bool, error) {
	ctx = tools.WithCoordination(ctx, f)
	type caller interface {
		Call(context.Context, json.RawMessage) (string, error)
	}
	var t caller
	switch name {
	case "spawn_worker":
		t = tools.NewSpawnWorkerTool()
	case "send_to_worker":
		t = tools.NewSendToWorkerTool()
	case "stop_worker":
		t = tools.NewStopWorkerTool()
	case "list_workers":
		t = tools.NewListWorkersTool()
	case "report_to_coordinator":
		t = tools.NewReportToCoordinatorTool()
	case "set_coordinator_mode":
		t = tools.NewSetCoordinatorModeTool()
	default:
		return "", false, nil
	}
	out, err := t.Call(ctx, args)
	return out, true, err
}

// formatWorkerList renders a coordinator's workers as a compact status list for
// the list_workers tool.
func formatWorkerList(ws []WorkerInfo) string {
	if len(ws) == 0 {
		return "No workers have been spawned under this coordinator yet."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d worker(s):\n", len(ws))
	for _, w := range ws {
		status := "finished"
		if w.Running {
			status = "running"
		}
		fmt.Fprintf(&b, "- %s [%s] (%s)", w.AgentName, status, w.SessionID)
		if w.Summary != "" {
			fmt.Fprintf(&b, " — %s", w.Summary)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// coordSlotFor returns (creating if needed) the slot for a coordinator session.
func (r *Runtime) coordSlotFor(coordSessionID string) *coordSlot {
	v, _ := r.coordSlots.LoadOrStore(coordSessionID, &coordSlot{})
	return v.(*coordSlot)
}

// isSessionActive reports whether a session is currently running an autonomous
// invoke (used by ListWorkers to distinguish running from finished workers).
func (r *Runtime) isSessionActive(id string) bool {
	_, ok := r.activeSessions.Load(id)
	return ok
}

// IsSessionActive is isSessionActive for callers outside the package (the
// coordinator-tree endpoint, which marks live nodes in the tree view).
func (r *Runtime) IsSessionActive(id string) bool { return r.isSessionActive(id) }

// WorkerSpec describes one spawn_worker request beyond the plain target/task
// pair: whether the new worker is itself a coordinator (the nesting switch) and,
// if so, which recipe it runs under.
type WorkerSpec struct {
	ModelOverride string
	// Coordinator makes the spawned worker a sub-coordinator: it gets the
	// coordination tools and may nest another level. Rejected past
	// CoordinatorMaxDepth rather than silently downgraded to a plain worker — a
	// caller that asked for delegation must not get a worker that cannot delegate
	// and never says so.
	Coordinator bool
	// Workflow optionally pins a coordinator recipe on the sub-coordinator. Only
	// meaningful with Coordinator; never inherited from the parent.
	Workflow string
	// WorkflowMaxTurns is the recipe-resolved notify-loop cap for the child (0 =
	// workspace default). Resolved by the caller, which owns the skills store.
	WorkflowMaxTurns int
}

// SpawnWorker launches a background worker under a coordinator session. The
// target must be an EXISTING agent (name or id) — a worker needs a persistent
// session, so ephemeral run_subagent profiles (explore/coder/reviewer) are not
// valid worker targets; use run_subagent (the sync M1 method) for those. Returns
// the worker session id immediately; the turn runs detached and notifies the
// coordinator on completion.
//
// Three guards stack here, and they are not redundant:
//   - CoordinatorMaxWorkers bounds the workers of THIS coordinator;
//   - CoordinatorMaxDepth bounds how far below the root a sub-coordinator may sit;
//   - CoordinatorMaxSubtreeSessions bounds the whole TREE, which is the only one
//     that actually stops exponential fan-out (per-node caps multiply with depth).
//
// The global SpawnMaxConcurrent cap applies on top, inside SpawnSession.
func (r *Runtime) SpawnWorker(ctx context.Context, coordSessionID, agentRef, task, createdBy string, spec WorkerSpec) (SpawnResult, error) {
	coordSessionID = strings.TrimSpace(coordSessionID)
	if coordSessionID == "" {
		return SpawnResult{}, fmt.Errorf("spawn_worker requires a coordinator session")
	}
	parent, err := r.db.GetSession(ctx, coordSessionID)
	if err != nil {
		return SpawnResult{}, fmt.Errorf("coordinator session %s not found: %w", coordSessionID, err)
	}
	depth := parent.CoordinatorDepth + 1
	rootID := parent.RootCoordinator()
	if rootID == "" {
		rootID = parent.ID // parent is a plain session being used as a root coordinator
	}
	// Hold the tree's spawn lock across the budget check AND the session creation.
	// The budget counts sessions on disk, so a bare check-then-create lets N
	// concurrent spawn_worker calls — which is precisely how a coordinator fans out —
	// all read the same "one slot left" and every one of them create. The
	// per-coordinator worker cap is safe because it reserves with an atomic add; the
	// tree budget has nothing to add to until the session exists. Creation is cheap
	// and the turn itself runs detached, so this serializes bookkeeping, not work.
	unlockTree := lockCoordinatorTree(rootID)
	defer unlockTree()
	if err := r.checkCoordinatorTreeBudget(ctx, parent, rootID, depth, spec.Coordinator); err != nil {
		return SpawnResult{}, err
	}
	// A profile target (explore/coder/reviewer) is materialized into a persisted,
	// reusable worker agent so the worker has a real session to run in. An ordinary
	// target passes through unchanged (existing agent name/id). Resolved BEFORE the
	// worker-count reservation so a bad target never leaks a slot.
	agentRef, err = r.resolveWorkerTarget(ctx, coordSessionID, createdBy, agentRef)
	if err != nil {
		return SpawnResult{}, err
	}
	slot := r.coordSlotFor(coordSessionID)
	max := int64(r.tun.CoordinatorMaxWorkers())
	if slot.workers.Add(1) > max {
		slot.workers.Add(-1)
		return SpawnResult{}, fmt.Errorf("coordinator worker limit reached (%d active); wait for some to finish before spawning more", max)
	}
	res, err := r.SpawnSession(ctx, agentRef, task, SpawnOptions{
		ModelOverride:            spec.ModelOverride,
		CreatedBy:                createdBy,
		CoordinatorSessionID:     coordSessionID,
		Role:                     db.SessionRoleWorker,
		RootCoordinatorSessionID: rootID,
		CoordinatorDepth:         depth,
		CoordinatorMode:          spec.Coordinator,
		CoordinatorWorkflow:      spec.Workflow,
		CoordinatorMaxTurns:      spec.WorkflowMaxTurns,
	})
	if err != nil {
		// SpawnSession never launched runWorker, so release the reservation here.
		slot.workers.Add(-1)
		return SpawnResult{}, err
	}
	slot.markHadWorkers()
	return res, nil
}

// coordinatorTreeLocks holds one spawn lock per coordinator TREE, keyed by root id.
var coordinatorTreeLocks sync.Map

// lockCoordinatorTree serializes budget-check-plus-create for one coordinator tree
// and returns the unlock func.
func lockCoordinatorTree(rootID string) func() {
	v, _ := coordinatorTreeLocks.LoadOrStore(rootID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// checkCoordinatorTreeBudget enforces the TREE-WIDE guards before a worker session
// is created: nesting depth and the total session count of the whole tree. Both
// fail loudly — a caller that hits a ceiling gets an error naming the limit, never
// a quietly downgraded worker, because a coordinator that believes it delegated
// work it did not delegate stalls waiting for a report that will never come.
func (r *Runtime) checkCoordinatorTreeBudget(ctx context.Context, parent db.Session, rootID string, depth int, wantCoordinator bool) error {
	if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && depth > maxDepth {
		return fmt.Errorf("coordinator depth limit reached (max %d levels; this worker would sit at depth %d). Do this work in the current session, or ask your own coordinator to restructure the plan", maxDepth, depth)
	}
	// Spawning a NON-coordinator leaf at the last allowed level is fine; only the
	// sub-coordinator itself needs room for a level below it.
	if wantCoordinator {
		if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && depth >= maxDepth {
			return fmt.Errorf("cannot spawn a sub-coordinator at depth %d: its own workers would exceed the coordinator depth limit (max %d). Spawn a plain worker here instead", depth, maxDepth)
		}
	}
	maxSubtree := r.tun.CoordinatorMaxSubtreeSessions()
	if maxSubtree <= 0 {
		return nil
	}
	tree, err := r.db.ListCoordinatorTree(ctx, rootID)
	if err != nil {
		// The root is gone (deleted mid-run). Fall back to the parent's own subtree
		// so the budget still bites rather than silently disappearing.
		if tree, err = r.db.ListCoordinatorTree(ctx, parent.ID); err != nil {
			return fmt.Errorf("cannot verify coordinator tree budget: %w", err)
		}
	}
	// The root itself is not a worker; everything below it is.
	if workers := len(tree) - 1; workers >= maxSubtree {
		return fmt.Errorf("coordinator tree budget exhausted (%d/%d worker sessions across the whole tree); stop or conclude existing workers before spawning more", workers, maxSubtree)
	}
	return nil
}

// resolveWorkerTarget maps a spawn_worker target to a runnable persistent agent
// id. An existing agent (name or id) passes through. A built-in profile
// (explore/coder/reviewer) is materialized once into a reusable persisted worker
// agent — "worker:<profile>" — cloned from the base agent (the coordinator's own
// agent: provider/model/permission) but reshaped with the profile's soul and tool
// allowlist. Reused on subsequent spawns (find-or-create, serialized).
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

	name := "worker:" + prof.ID
	r.profileWorkerMu.Lock()
	defer r.profileWorkerMu.Unlock()
	// Already materialized? Reuse it.
	if a, err := r.resolveAgent(ctx, name); err == nil {
		return a.ID, nil
	}
	// Clone provider/model/permission from the base agent (coordinator's agent), or
	// the coordinator session's agent when no base id was supplied.
	if strings.TrimSpace(baseAgentID) == "" {
		if sess, err := r.db.GetSession(ctx, coordSessionID); err == nil {
			baseAgentID = sess.AgentID
		}
	}
	base, err := r.db.GetAgent(ctx, baseAgentID)
	if err != nil {
		return "", fmt.Errorf("cannot materialize worker profile %q: base agent unavailable: %w", prof.ID, err)
	}
	allow, _ := json.Marshal(prof.AllowedTools)
	created, err := r.db.CreateAgent(ctx, db.Agent{
		Name:           name,
		Soul:           prof.SystemPrompt,
		Provider:       base.Provider,
		Model:          base.Model,
		PermissionMode: base.PermissionMode,
		MCPEnabled:     true,
		AllowedTools:   string(allow),
		CreatedBy:      baseAgentID,
	})
	if err != nil {
		return "", fmt.Errorf("cannot materialize worker profile %q: %w", prof.ID, err)
	}
	r.logger.Info("coordination: materialized profile worker", "profile", prof.ID, "agent", created.ID)
	return created.ID, nil
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

	// Backpressure instead of rejection: a worker mid-turn no longer loses the
	// message. The busy-check and the enqueue are done under workerQueueMu in one
	// critical section so they stay atomic against drainWorkerQueue, which pops
	// under the same mutex once the turn ends (see runWorker's deferred drain).
	// isSessionActive flips false BEFORE that drain runs, so any message accepted
	// here (active == true) is guaranteed to be seen by the drain — no lost update.
	r.workerQueueMu.Lock()
	if r.isSessionActive(workerSessionID) {
		if _, exists := r.workerQueue[workerSessionID]; exists {
			r.workerQueueMu.Unlock()
			return tools.SendResult{}, fmt.Errorf("worker %s already has a queued message waiting for its current turn to finish; "+
				"wait for that to be delivered before sending another (only one may be queued per worker)", workerSessionID)
		}
		r.workerQueue[workerSessionID] = message
		r.workerQueueMu.Unlock()
		return tools.SendResult{Queued: true, RunningForSeconds: r.workerRunningForSeconds(workerSessionID)}, nil
	}
	r.workerQueueMu.Unlock()

	// Worker is idle: deliver immediately (same depth-aware slot reservation as a
	// fresh spawn — continuing a deep worker drains the pool just as a spawn does).
	if err := r.dispatchWorkerTurn(ctx, agent, workerSessionID, message, coordSessionID, ws.CoordinatorDepth); err != nil {
		return tools.SendResult{}, err
	}
	return tools.SendResult{Delivered: true}, nil
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
	if _, err := r.recordInjectedUserNote(ctx, workerSessionID, "", message); err != nil {
		r.releaseSpawnSlot()
		return err
	}
	slot := r.coordSlotFor(coordSessionID)
	slot.workers.Add(1)
	slot.markHadWorkers()
	if r.workerRunFn != nil {
		// Test seam: the caller counts the slots, so mirror the real path's release.
		defer r.releaseSpawnSlot()
		defer slot.workers.Add(-1)
		r.workerRunFn(agent, workerSessionID, message, coordSessionID)
		return nil
	}
	go r.runWorker(agent, workerSessionID, message, coordSessionID)
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

// drainWorkerQueue delivers a follow-up parked while the worker was mid-turn. It
// runs as runWorker's LAST deferred action — after every slot release and after
// untrackSession, so isSessionActive is already false and the delivery re-runs
// the worker cleanly. The pop is done under workerQueueMu (the same mutex
// SendToWorker enqueues under) so an enqueue that raced the turn end is either
// fully visible here or already took the idle path. Runs on context.Background:
// the worker turn's ctx is cancelled by now.
func (r *Runtime) drainWorkerQueue(agent db.Agent, workerSessionID, coordSessionID string) {
	r.workerQueueMu.Lock()
	message, ok := r.workerQueue[workerSessionID]
	if ok {
		delete(r.workerQueue, workerSessionID)
	}
	r.workerQueueMu.Unlock()
	if !ok {
		return
	}
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
	subtreeRunning := 0
	if ws.IsCoordinator() {
		subtreeRunning = r.activeSubtreeWorkers(ctx, workerSessionID)
		r.stopSubtree(ctx, workerSessionID, "stop_worker on their sub-coordinator")
	}
	v, ok := r.workerCancels.Load(workerSessionID)
	if !ok {
		if subtreeRunning > 0 {
			// The sub-coordinator itself was between turns; its branch is what was
			// actually running and we just cancelled it. Report that truthfully
			// instead of the misleading "already finished?".
			return nil
		}
		return fmt.Errorf("worker %s is not running (already finished?)", workerSessionID)
	}
	ctl := v.(*workerCtl)
	ctl.stopped.Store(true)
	ctl.cancel()
	return nil
}

// WorkerInfo is a coordinator-facing snapshot of one worker session.
type WorkerInfo struct {
	SessionID string
	AgentName string
	Title     string
	Running   bool
	Summary   string // first line of the worker's latest reply, when finished
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

// workerInfoFor snapshots one worker session for a coordinator-facing listing.
// Shared by ListWorkers (direct children) and ListSubtreeWorkers (every
// descendant) so both report liveness and summaries identically.
//
// A sub-coordinator counts as RUNNING while any of its own workers is running,
// even when it has no turn of its own in flight: between its turns it is waiting
// on its branch, and reporting it as finished there is exactly how a parent
// concludes on top of work that is still in progress.
func (r *Runtime) workerInfoFor(ctx context.Context, s db.Session) WorkerInfo {
	info := WorkerInfo{
		SessionID: s.ID,
		AgentName: r.agentName(s.AgentID),
		Title:     s.Title,
		Running:   r.isSessionActive(s.ID),
	}
	if !info.Running && s.IsCoordinator() && r.coordSlotFor(s.ID).workers.Load() > 0 {
		info.Running = true
		info.Delegating = true
	}
	if info.Running {
		if v, ok := r.workerCancels.Load(s.ID); ok {
			info.StartedAt = v.(*workerCtl).startedAt.Unix()
		}
	} else if msgs, err := r.db.ListMessages(ctx, s.ID); err == nil {
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == "assistant" {
				info.Summary = notifyLine(msgs[i].Text, 120)
				break
			}
		}
	}
	return info
}

// runWorker executes a worker's background turn (initial spawn or a send_to_worker
// continuation): it runs a history-aware turn, records the reply (or the failure)
// as an assistant turn, releases the concurrency slots, and — crucially — injects a
// <task-notification> into the coordinator session and triggers a coordinator turn.
// It mirrors runSpawn but is coordinator-aware and notifies on EVERY outcome
// (completed / failed / killed), unlike a plain spawn.
func (r *Runtime) runWorker(agent db.Agent, workerSessionID, prompt, coordSessionID string) {
	// Registered FIRST so it runs LAST — after every slot release, workerCancels
	// delete, and untrackSession below. Only then is the worker idle enough for a
	// parked follow-up (send_to_worker while this turn was busy) to be delivered as
	// the next turn. No-op when nothing was queued.
	defer r.drainWorkerQueue(agent, workerSessionID, coordSessionID)
	defer r.releaseSpawnSlot()
	if slot := r.coordSlotFor(coordSessionID); slot != nil {
		defer slot.workers.Add(-1)
	}

	// Hard wall-clock ceiling (settings-driven, same as spawns) PLUS an idle
	// watchdog: a worker/coordinator turn that streams no step for SpawnIdleTimeout
	// is reclaimed fast, while a long-but-productive one runs up to SpawnTimeout.
	hardCap, idleCap := r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout()
	ctx, cancel := withActivityTimeout(context.Background(), hardCap, idleCap)
	defer cancel()
	ctl := &workerCtl{cancel: cancel, startedAt: time.Now()}
	r.workerCancels.Store(workerSessionID, ctl)
	defer r.workerCancels.Delete(workerSessionID)

	// Serialize this worker turn on the WORKER session's own turn slot (keyed by
	// workerSessionID, distinct from the coordinator slot whose workers counter is
	// decremented above) so it never overlaps another turn on the same worker
	// session: a second send_to_worker that raced the isSessionActive check (that
	// check is a UI hint, not a lock), or a user/wake/peer turn opened on the worker
	// session (all of which now claim this same slot).
	releaseSlot := r.claimSessionTurnSlot(workerSessionID)
	defer releaseSlot()

	turnCtx := tools.WithAsyncChat(WithSessionID(WithCallKind(ctx, KindSpawn), workerSessionID))
	turnCtx, meta := WithTurnMeta(turnCtx)
	turnStart := time.Now()

	r.trackSession(workerSessionID)
	// Raise the "thinking" indicator for the worker session (see emitTurnStart);
	// the completion "worker" event clears it.
	r.emitTurnStart(workerSessionID, "🤝 Worker turu çalışıyor")
	// Tell the coordination UI a worker is now running (covers both the initial
	// spawn and a send_to_worker continuation, since both land here).
	r.emitWorkerStartEvent(agent, workerSessionID, coordSessionID)
	output, steps, err := r.runSessionTurn(turnCtx, agent, workerSessionID, prompt, true)
	r.untrackSession(workerSessionID)

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
		// A deadline check must precede the plain-cancel check: withActivityTimeout
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

	// replyText was pre-composed above (success output / failure / kill / empty note).
	if addErr := r.recordAssistantMessage(ctx, workerSessionID, agent.ID, replyText, steps, meta, time.Since(turnStart).Milliseconds()); addErr != nil {
		r.logger.Warn("worker: failed to record reply", "session", workerSessionID, "error", addErr)
	}
	r.emitWorkerEvent(agent, workerSessionID, coordSessionID, status)

	// A SUB-COORDINATOR that just fanned its work out is not finished, whatever its
	// turn returned: reporting this turn as "completed" would tell its coordinator
	// the subtask is done while the branch below has barely started. Withhold the
	// completion notification, send an interim "delegating" note instead, and let
	// the node close its own task later (report_to_coordinator / settle backstop).
	// See coordination_tree.go for the full contract.
	if ws, err := r.db.GetSession(ctx, workerSessionID); err == nil && r.deferWorkerReport(ctx, ws, status) {
		r.notifyDelegating(coordSessionID, workerSessionID, agent.Name, int(r.coordSlotFor(workerSessionID).workers.Load()))
		return
	}

	// The report the coordinator actually reads: the successful output verbatim, or
	// the failure/kill note (so the coordinator can react to failures too).
	note := formatTaskNotification(workerSessionID, agent.Name, status, replyText, countToolSteps(steps), time.Since(turnStart).Milliseconds())
	r.NotifyCoordinator(coordSessionID, note)
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
		return
	}
	// Shallowest first, so a coordinator TREE is reclaimed from the root down. The
	// order is load-bearing: a mid-level node is both a worker and a coordinator, so
	// when we reach a child its parent has already been reclaimed and recorded in
	// `reclaimed` below — and we can skip notifying a node that is itself dead
	// instead of waking a zombie turn on it.
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].CoordinatorDepth < sessions[j].CoordinatorDepth
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
		isSpawn := sess.Kind == "spawned" || sess.Kind == "worker"
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
		if msgs[len(msgs)-1].Role != "user" {
			continue // completed normally (last message is an assistant reply)
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
				note := formatTaskNotification(sess.ID, r.agentName(sess.AgentID), "killed",
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

// NotifyCoordinator persists a <task-notification> as a user message in the
// coordinator session, then asks the per-session turn queue to run a coordinator
// turn. Safe to call from many workers concurrently: the queue serializes turns
// and coalesces pile-ups. No-op when coordSessionID is empty.
func (r *Runtime) NotifyCoordinator(coordSessionID, note string) {
	coordSessionID = strings.TrimSpace(coordSessionID)
	if coordSessionID == "" || strings.TrimSpace(note) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if _, err := r.recordInjectedUserNote(ctx, coordSessionID, "worker-note", note); err != nil {
		cancel()
		r.logger.Warn("coordination: failed to record task-notification", "coordinator", coordSessionID, "error", err)
		return
	}
	cancel()
	r.enqueueCoordinatorTurn(coordSessionID)
}

// recordInjectedUserNote persists a runtime-injected user-role note to a
// coordination session (a worker task-notification, a send_to_worker prompt, a
// coordination status/guard note) AND bridges it live to the session hub via the
// bus. Interactive user messages already reach the hub from the send-queue worker
// (chat_stream publishHub KindUserMessage); these injected ones bypassed it, so a
// window watching the coordinator/worker rendered the assistant reply that
// followed WITHOUT the message it answered — it looked like a duplicate reply
// appearing out of nowhere, and only a page reload restored the real order
// (_Docs/58, _Docs/47). Returns the persisted message so callers can chain.
func (r *Runtime) recordInjectedUserNote(ctx context.Context, sessionID, origin, text string) (db.Message, error) {
	return r.recordInjectedUserMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      "user",
		Origin:    origin,
		Text:      text,
	})
}

// recordInjectedUserMessage is the general form of recordInjectedUserNote for
// injected user turns that carry extra participant fields (a peer inbox delivery
// stamps AuthorKind/AuthorID/RecipientID; a spawn/flow opening prompt is a plain
// bubble). It persists m (Role should be "user") AND bridges it live to the hub.
// Returns the persisted message.
func (r *Runtime) recordInjectedUserMessage(ctx context.Context, m db.Message) (db.Message, error) {
	msg, err := r.db.AddMessage(ctx, m)
	if err != nil {
		return db.Message{}, err
	}
	r.emitInjectedUserNote(msg.SessionID, msg)
	return msg, nil
}

// emitInjectedUserNote broadcasts a just-persisted injected user message so the
// bus→hub bridge (bridgeBusToHub) can render it live on every window watching the
// session, mirroring how emitTurnStart / publishAutonomousReply cover the rest of
// an autonomous turn. No-op on an empty session id or a marshal error (best-effort
// live signal; the durable transcript backstops it on reload).
func (r *Runtime) emitInjectedUserNote(sessionID string, msg db.Message) {
	if sessionID == "" {
		return
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeSessionUserMessage,
		Target: map[string]string{"view": "chat", "sessionId": sessionID},
		Msg:    b,
	})
}

// BeginSessionUserTurn claims the session's turn slot for an interactive
// (user-initiated) turn so it never overlaps ANY other turn on the same session —
// a concurrent direct /chat/stream call, a queued inbox turn, an auto-triggered
// coordinator turn, a scheduler wake, or a peer inbox delivery. This is the single
// per-session turn lock: EVERY turn-entry path claims it, not just coordinator
// sessions (that coordinator-only gate was the source of the concurrent-turn race
// on plain sessions — see _Docs/58). Turns arriving meanwhile block until this
// call's release runs; a coordinator's auto turns instead fall into pending
// (enqueueCoordinatorTurn sees running=true) and coalesce into one turn on release.
// The slot's coordinator-only fields (pending/turns) stay unused on a plain
// session, so release is a clean unlock there. A user turn also resets the
// auto-turn cap (a human is back in the loop); harmless on a plain session where
// the cap is never consulted. The returned release func MUST be deferred.
func (r *Runtime) BeginSessionUserTurn(sessionID string) (release func()) {
	return r.claimCoordinatorSlot(sessionID, true)
}

// claimSessionTurnSlot claims the session's turn slot for an AUTONOMOUS turn
// (scheduler wake, scheduled prompt, peer inbox delivery) so it serializes with
// every other turn on the same session — exactly like an interactive chat turn.
// It always claims (no coordinator gate): a plain session gets real mutual
// exclusion too, closing the wake-vs-user / peer-vs-user race. Unlike a user turn
// it does NOT reset the auto-turn cap (no human re-entered the loop). Returns the
// release func — defer it.
func (r *Runtime) claimSessionTurnSlot(sessionID string) (release func()) {
	return r.claimCoordinatorSlot(sessionID, false)
}

// claimCoordinatorSlot is the low-level per-session turn lock: it blocks until the
// session's turn slot is free, claims it, and returns the release func (see
// BeginSessionUserTurn / claimSessionTurnSlot for semantics). Despite the name it
// backs EVERY session's turn serialization, not only coordinators — a plain
// session simply never touches the coordinator-only pending/turns fields. resetCap
// additionally zeroes the auto-turn budget (human back in the loop).
func (r *Runtime) claimCoordinatorSlot(coordSessionID string, resetCap bool) func() {
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	if slot.free == nil {
		slot.free = sync.NewCond(&slot.mu)
	}
	for slot.running {
		slot.free.Wait()
	}
	slot.running = true
	if resetCap {
		slot.turns = 0
		slot.capWarn = false
	}
	slot.mu.Unlock()
	return func() {
		slot.mu.Lock()
		slot.running = false
		pending := slot.pending
		slot.pending = false
		slot.signalFree()
		slot.mu.Unlock()
		if pending {
			r.enqueueCoordinatorTurn(coordSessionID)
		}
	}
}

// enqueueCoordinatorTurn schedules one coordinator turn. If a turn is already
// running it just flags pending (the running turn will loop once more and see the
// freshly-persisted notification in history). Otherwise it starts the drain loop.
func (r *Runtime) enqueueCoordinatorTurn(coordSessionID string) {
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	// A fresh notification (a worker just finished or continued) re-arms the
	// idle-reconcile sweep: this batch is no longer "acknowledged idle".
	slot.ackedIdle = false
	if slot.running {
		slot.pending = true
		slot.mu.Unlock()
		return
	}
	slot.running = true
	slot.mu.Unlock()
	go r.drainCoordinator(coordSessionID, slot)
}

// drainCoordinator runs coordinator turns until no more notifications are pending,
// bounded by CoordinatorMaxTurns (the notify-loop guard). Each iteration runs one
// history-aware turn that sees every notification persisted so far.
func (r *Runtime) drainCoordinator(coordSessionID string, slot *coordSlot) {
	for {
		slot.mu.Lock()
		// A selected recipe (M5) may lower/raise the notify-loop cap for just this
		// coordinator session; fall back to the workspace default when unset (0).
		maxTurns := r.tun.CoordinatorMaxTurns()
		if sess, err := r.db.GetSession(context.Background(), coordSessionID); err == nil && sess.CoordinatorMaxTurns > 0 {
			maxTurns = sess.CoordinatorMaxTurns
		}
		if slot.turns >= maxTurns {
			warn := !slot.capWarn
			slot.capWarn = true
			slot.running = false
			slot.pending = false
			slot.signalFree()
			slot.mu.Unlock()
			if warn {
				r.warnCoordinatorCap(coordSessionID, slot.turns)
			}
			return
		}
		slot.turns++
		slot.mu.Unlock()

		if r.coordRunFn != nil {
			r.coordRunFn(coordSessionID)
		} else {
			r.runCoordinatorTurn(coordSessionID)
		}

		slot.mu.Lock()
		if slot.pending {
			slot.pending = false
			slot.mu.Unlock()
			continue
		}
		// Idle reconciliation (liveness backstop): if EVERY worker is now finished
		// and we have not yet run a reconcile turn for this all-idle transition,
		// inject an authoritative "all workers finished" note and loop ONCE more.
		// This guarantees the coordinator gets a final, unambiguous turn even when
		// it overlooked one notification in a coalesced batch — breaking the "waits
		// forever on an already-finished worker" stall. One-shot per all-idle
		// transition (ackedIdle, re-armed by the next notification) and bounded by
		// CoordinatorMaxTurns (checked at the loop top), so it can never loop.
		if !slot.ackedIdle && slot.hadWorkers && slot.workers.Load() == 0 {
			slot.ackedIdle = true
			slot.mu.Unlock()
			r.appendCoordinationStatus(coordSessionID)
			continue
		}
		slot.running = false
		slot.signalFree()
		slot.mu.Unlock()
		// This coordinator may itself be a worker that owes its own coordinator a
		// result (a mid-level node). It has now had its reconcile turn with every
		// worker finished; if it still has not called report_to_coordinator, the
		// branch above it would wait forever. The backstop reports for it — see
		// settleReportBackstop for why it never claims "completed".
		r.scheduleSettleBackstop(coordSessionID)
		return
	}
}

// scheduleSettleBackstop arms the upward-report backstop for a mid-level node
// whose drain loop just went idle. Delayed rather than immediate: a notification
// racing the drain exit re-enters the loop and the node still gets its chance to
// report itself, which is always preferable to the runtime guessing on its behalf.
// A no-op for a node that owes nothing (the overwhelmingly common case: a root
// coordinator).
func (r *Runtime) scheduleSettleBackstop(coordSessionID string) {
	probe, cancelProbe := context.WithTimeout(context.Background(), 10*time.Second)
	owes := r.owesReportNow(probe, coordSessionID)
	cancelProbe()
	if !owes {
		return
	}
	go func() {
		time.Sleep(r.tun.CoordinatorSettleGrace())
		slot := r.coordSlotFor(coordSessionID)
		slot.mu.Lock()
		busy := slot.running || slot.pending
		slot.mu.Unlock()
		if busy {
			return // it woke up again; let it report for itself
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r.settleReportBackstop(ctx, coordSessionID)
	}()
}

// appendCoordinationStatus persists a one-shot <coordination-status> note into the
// coordinator session so the idle-reconcile turn opens on an explicit, authoritative
// signal that every worker has finished. Purely a history append (no enqueue): the
// caller is already inside the drain loop and continues to the next turn.
func (r *Runtime) appendCoordinationStatus(coordSessionID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const note = "<coordination-status>All workers under this coordinator have finished. " +
		"Act on any results you have not handled yet, spawn the next steps if the plan has more, " +
		"or conclude the project. Do NOT wait for a worker that has already finished.</coordination-status>"
	if _, err := r.recordInjectedUserNote(ctx, coordSessionID, "worker-note", note); err != nil {
		r.logger.Warn("coordination: failed to record idle status note", "coordinator", coordSessionID, "error", err)
	}
}

// coordinatorWorkerStatusBlock renders an authoritative, always-fresh snapshot of
// every worker under coordSessionID for the coordinator's dynamic system suffix.
// Unlike the prose <task-notification>s in history — which a coalesced batch can
// let the model overlook — this block is regenerated every turn from live session
// state, so the coordinator can never believe a finished worker is still running.
// Empty when the session has no workers.
//
// The rendering itself lives in internal/view (ProjectWorkers): this is a PUSH
// projection into every coordinator turn, so it inherits that package's discipline
// — a cap on how many entries a wide fleet may inject, with the dropped ones still
// counted in the summary line the coordinator reasons over.
func (r *Runtime) coordinatorWorkerStatusBlock(ctx context.Context, coordSessionID string) string {
	ws, err := r.ListWorkers(ctx, coordSessionID)
	if err != nil || len(ws) == 0 {
		return ""
	}
	v, err := view.ProjectWorkers(view.WorkersInput{Workers: toViewWorkers(ws)}, view.LevelCard, view.LensHealth)
	if err != nil {
		// The block is an optional prompt enrichment; losing it must not fail the
		// turn. It IS worth a log line — a coordinator silently running without its
		// authoritative fleet state is the exact condition this block prevents.
		r.logger.Warn("coordination: worker status block render failed",
			"coordinator", coordSessionID, "error", err)
		return ""
	}
	return v.Text()
}

// toViewWorkers maps the runtime's live fleet onto the renderer's input. The
// view package cannot import this one (agent → tools → view), so the mapping
// lives here.
func toViewWorkers(ws []WorkerInfo) []view.Worker {
	out := make([]view.Worker, 0, len(ws))
	for _, w := range ws {
		out = append(out, view.Worker{
			SessionID:  w.SessionID,
			AgentName:  w.AgentName,
			Running:    w.Running,
			Delegating: w.Delegating,
			Summary:    w.Summary,
			StartedAt:  w.StartedAt,
		})
	}
	return out
}

// runCoordinatorTurn runs one history-aware turn for the coordinator session so it
// synthesizes the worker notifications now sitting in its history, then records the
// reply and fires the turn-finished hook (for tags/automations on the coordinator).
func (r *Runtime) runCoordinatorTurn(coordSessionID string) {
	// Hard wall-clock ceiling (settings-driven, same as spawns) PLUS an idle
	// watchdog: a worker/coordinator turn that streams no step for SpawnIdleTimeout
	// is reclaimed fast, while a long-but-productive one runs up to SpawnTimeout.
	hardCap, idleCap := r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout()
	ctx, cancel := withActivityTimeout(context.Background(), hardCap, idleCap)
	defer cancel()

	sess, err := r.db.GetSession(ctx, coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: coordinator session gone", "coordinator", coordSessionID, "error", err)
		return
	}
	agent, err := r.db.GetAgent(ctx, sess.AgentID)
	if err != nil {
		r.logger.Warn("coordination: coordinator agent gone", "coordinator", coordSessionID, "error", err)
		return
	}

	turnCtx := tools.WithAsyncChat(WithSessionID(WithCallKind(ctx, KindSpawn), coordSessionID))
	turnCtx, meta := WithTurnMeta(turnCtx)
	turnStart := time.Now()

	r.trackSession(coordSessionID)
	output, steps, err := r.runSessionTurn(turnCtx, agent, coordSessionID, "", true)
	r.untrackSession(coordSessionID)

	// A watchdog cut (hard/idle) or a self-truncated loop hands back salvaged text;
	// lead it with the outcome note (nil error) so the recorded reply reads as a
	// fragment, not a clean result, and the success-only follow-ups below are skipped.
	output, steps, err, truncated := r.reconcileTurnOutcome(ctx, output, steps, err, hardCap, idleCap)
	text := output
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// A viewer pressed "Durdur"/"Kes": the autonomous run's cancel aborted the
			// turn (see autonomousInteraction). Report a clean stop, not a failure.
			text = "⏹️ Koordinatör turu durduruldu."
			r.logger.Info("coordination: coordinator turn stopped", "coordinator", coordSessionID)
		} else {
			text = "⚠️ Koordinatör turu çalıştırılamadı:\n\n" + err.Error()
			r.logger.Error("coordination: coordinator turn failed", "coordinator", coordSessionID, "error", err)
		}
	} else if strings.TrimSpace(text) == "" {
		text = "ℹ️ Koordinatör bu tur için boş yanıt döndürdü."
	}
	// text was pre-composed above (success output / failure / stop / empty note).
	if addErr := r.recordAssistantMessage(ctx, coordSessionID, agent.ID, text, steps, meta, time.Since(turnStart).Milliseconds()); addErr != nil {
		r.logger.Warn("coordination: failed to record coordinator reply", "coordinator", coordSessionID, "error", addErr)
	}
	r.publish(events.Event{
		Type:   events.TypeChat,
		Level:  "info",
		Title:  "🧭 Koordinatör turu tamamlandı — " + agent.Name,
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
	// Coordinator turns are ordinary turns for the rest of the system: fire the hook
	// so tags/automations on the coordinator session still work. The coordinator has
	// no CoordinatorSessionID, so this never re-enters the coordination loop.
	// Stamp turn-completion time for the stall sweeper's staleness check, regardless
	// of success (a failed/stopped turn still counts as activity).
	if slot := r.coordSlotFor(coordSessionID); slot != nil {
		slot.mu.Lock()
		slot.lastTurnUnix = time.Now().Unix()
		slot.mu.Unlock()
	}
	// A truncated turn only produced a fragment (already led with a "not done" note):
	// withhold completion automations AND skip the spawn-narration stall check, which
	// must not judge a turn the watchdog cut short.
	if err == nil && !truncated {
		r.FireTurnFinished(coordSessionID, agent.ID, output)
		// Catch the "narrated a spawn but never called the tool" degradation before the
		// drain loop's post-turn pending check, so a corrective re-prompt runs THIS batch.
		r.guardCoordinatorStall(coordSessionID, agent.ID, agent, output, steps)
	}
}

// turnCalledCoordinationTool reports whether any (possibly nested) step in a turn
// invoked a coordination tool. Matches the bare Tool name and the namespaced CallName
// alike (mcp__tionswarm_interaction__spawn_worker on the claude-cli path).
func turnCalledCoordinationTool(steps []TurnStep) bool {
	for _, s := range steps {
		if s.Kind == StepTool {
			n := strings.ToLower(s.Tool + " " + s.CallName)
			if strings.Contains(n, "spawn_worker") || strings.Contains(n, "list_workers") ||
				strings.Contains(n, "send_to_worker") || strings.Contains(n, "stop_worker") {
				return true
			}
		}
		if len(s.SubSteps) > 0 && turnCalledCoordinationTool(s.SubSteps) {
			return true
		}
	}
	return false
}

// warnCoordinatorCap posts a one-time notice that a coordinator hit its auto-turn
// cap; further worker notifications are still persisted but stop auto-triggering
// turns, so the user can inspect and continue manually.
func (r *Runtime) warnCoordinatorCap(coordSessionID string, turns int) {
	r.logger.Warn("coordination: coordinator auto-turn cap reached", "coordinator", coordSessionID, "turns", turns)
	r.publish(events.Event{
		Type:   events.TypeCoordination,
		Level:  "info",
		Title:  "🧭 Koordinatör tur limiti",
		Body:   fmt.Sprintf("Otomatik koordinatör turları limiti (%d) aşıldı; yeni worker bildirimleri kaydediliyor ama otomatik tur tetiklenmiyor. Devam etmek için oturuma manuel mesaj gönderin.", turns),
		Target: map[string]string{"view": "executions", "sessionId": coordSessionID},
	})
}

// emitWorkerStartEvent publishes a worker turn START so the coordination UI (the
// sidebar roster and the chat's running-worker banner) learns about a new worker
// the moment it begins, instead of polling for it.
//
// It carries phase="start", which the frontend MUST use to tell it apart from the
// completion event below: the completion branch drops the session's live ghost
// bubble, reloads the transcript and raises a desktop toast — all wrong for a turn
// that is only just beginning.
func (r *Runtime) emitWorkerStartEvent(agent db.Agent, workerSessionID, coordSessionID string) {
	r.publish(events.Event{
		Type:  events.TypeWorker,
		Level: "info",
		Title: "🤖 Worker başladı — " + agent.Name,
		Target: map[string]string{
			"view":          "executions",
			"sessionId":     workerSessionID,
			"coordinatorId": coordSessionID,
			"phase":         "start",
		},
	})
}

// emitWorkerEvent publishes a worker status transition so the coordination UI can
// live-update its worker cards.
func (r *Runtime) emitWorkerEvent(agent db.Agent, workerSessionID, coordSessionID, status string) {
	level := "success"
	if status == "failed" {
		level = "error"
	} else if status == "killed" {
		level = "info"
	}
	r.publish(events.Event{
		Type:  events.TypeWorker,
		Level: level,
		Title: "🤖 Worker " + status + " — " + agent.Name,
		Target: map[string]string{
			"view":          "executions",
			"sessionId":     workerSessionID,
			"coordinatorId": coordSessionID,
		},
	})
}

// formatTaskNotification renders a worker outcome as the <task-notification> XML
// the coordinator reads (mirrors Claude Code's coordinator format). status is
// completed | timeout | incomplete | failed | killed; result is the worker's final
// text (for the truncated statuses, prefixed with a note saying so — see
// turnoutcome.go). Only "completed" means the worker finished its assignment.
func formatTaskNotification(workerSessionID, agentName, status, result string, toolUses int, durationMs int64) string {
	var b strings.Builder
	b.WriteString("<task-notification>\n")
	fmt.Fprintf(&b, "<task-id>%s</task-id>\n", workerSessionID)
	fmt.Fprintf(&b, "<agent>%s</agent>\n", agentName)
	fmt.Fprintf(&b, "<status>%s</status>\n", status)
	fmt.Fprintf(&b, "<summary>Worker %q %s</summary>\n", agentName, status)
	if strings.TrimSpace(result) != "" {
		fmt.Fprintf(&b, "<result>%s</result>\n", strings.TrimSpace(result))
	}
	b.WriteString("<usage>")
	fmt.Fprintf(&b, "<tool_uses>%d</tool_uses><duration_ms>%d</duration_ms>", toolUses, durationMs)
	b.WriteString("</usage>\n")
	b.WriteString("</task-notification>")
	return b.String()
}

// countToolSteps counts tool invocations in a turn trace (for the usage section).
func countToolSteps(steps []TurnStep) int {
	n := 0
	for _, s := range steps {
		if s.Kind == StepTool {
			n++
		}
	}
	return n
}
