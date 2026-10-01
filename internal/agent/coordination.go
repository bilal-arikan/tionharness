package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Keep the queue bounded: an uncontrolled coordinator loop could otherwise keep
// a worker running follow-up turns forever.
const maxWorkerQueueDepth = 4

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

// coordSlot holds one coordinator session's DRAIN POLICY: should another auto-turn
// run, has this batch been reconciled, is the notify loop capped, is the model
// wedged. Mutual exclusion is NOT here — every turn (coordinator or not) is ordered
// by the per-session admission queue in internal/turnqueue, which the drain loop
// re-enters once per iteration exactly like any other caller. That is what keeps a
// waiting user message from starving behind a coordinator's own auto-turns
// (_Docs/58). Guarded by mu except workers (atomic, touched from the spawn path
// without the turn lock).
type coordSlot struct {
	mu sync.Mutex
	// admission serializes worker-note persistence/arming with the drain's final
	// turn-start gate. Lock order is admission, then mu; neither durable writes nor
	// coordinator turns run while mu is held.
	admission chan struct{}
	// driving marks that a drainCoordinator goroutine owns this coordinator's
	// auto-turn loop. It is NOT "a turn is running" (ask the queue for that): it
	// exists so concurrent notifications coalesce into the ONE loop instead of
	// starting a second one.
	driving bool
	// pending is an immediate/generic wake. workerPending has its own fixed
	// first-arrival deadline so flow starts, recovery and stall nudges never wait.
	pending        bool
	workerPending  bool
	workerDeadline time.Time
	wake           chan struct{}
	ackedIdle      bool // claimed the all-idle signal for this worker wave
	hadWorkers     bool // at least one worker was ever spawned (gates the idle sweep)
	// idleFolded marks that the CURRENT all-idle transition was already reported by
	// piggybacking <coordination-status> onto the last worker's own notification, so
	// the coordinator learns the result and "everyone is done" in a single turn.
	// It keeps later notifications for that same transition (already-finished
	// siblings landing after the zero-crossing) from reopening the claim. Cleared by the next
	// spawn/continuation, which opens a genuinely new transition.
	idleFolded bool
	// coordinatorMode records that this slot belongs to a session that actually has
	// coordinator mode on (set by guardCoordinatorStall, which only ever runs for a
	// coordinator turn). The stall sweeper uses it to relax the hadWorkers gate: a
	// coordinator that NARRATED a spawn and never made the call has no workers by
	// definition, which is exactly the phantom-spawn freeze the sweeper must catch.
	coordinatorMode bool
	turns           int          // auto-triggered coordinator turns so far (notify-loop cap)
	capWarn         bool         // whether the "cap reached" warning has been posted
	workers         atomic.Int64 // active workers under this coordinator
	// spawnHallucStreak counts consecutive coordinator turns judged to have CLAIMED a
	// spawn while making NO coordination tool call — the long-context degradation
	// freeze. Bounds the corrective nudges so a wedged model cannot burn the notify
	// loop; reset to 0 by any turn that actually calls a coordination tool. See
	// guardCoordinatorStall (coordination_stall.go).
	spawnHallucStreak int
	// stallHalted is the hard-halt escalation flag: set once the nudge budget is spent
	// AND the coordinator is STILL judged to be phantom-spawning. It stops the drain
	// loop from re-arming so a wedged coordinator
	// no longer burns auto-turns, and gates the one-shot user-facing halt notice. Reset
	// to false by any turn that actually calls a coordination tool (genuine recovery).
	// See guardCoordinatorStall / escalateCoordinatorStallHalt (coordination_stall.go).
	stallHalted bool
	// stopRequested records that this coordinator's turn ended on a human Stop (plain
	// context.Canceled, distinct from a watchdog cut). Set in two places:
	// runCoordinatorTurn, when the LIVE turn was cancelled, and drainCoordinator's bail,
	// when the stop landed while the turn was still queued for the session's turn slot.
	// Both are followed by the drain loop returning immediately — the flag itself has no
	// reader today; it is kept so the slot's state still says WHY the loop stopped, and
	// so the three clears below (`stallHalted` exit, pending re-arm, idle exit) keep a
	// consistent meaning. The exit is what suppresses the idle-reconcile turn: without
	// it a manual Stop on a coordinator whose workers had all finished would inject a
	// fresh <coordination-status> note and run once more, so the session looked like it
	// "kept going" after the user stopped it.
	// One-shot: a later worker notification re-arms the loop via enqueueCoordinatorTurn.
	stopRequested bool
	// lastTurnUnix is the wall-clock (unix seconds) at which this coordinator's last
	// real turn finished. 0 until the first real turn ran (a stubbed test never sets
	// it). The stall sweeper reads it to find coordinators gone silent past the
	// staleness window.
	lastTurnUnix int64
}

// releaseOnce returns a func that decrements slot's active-worker counter the first
// time it is called and does nothing afterwards, reporting whether THIS call was the
// one that brought the fleet to zero. runWorker needs the release at a precise point
// (before the terminal notification, so the last-worker check sees an accurate count)
// while still keeping a deferred release for its early returns; an idempotent releaser
// lets both exist without ever double-decrementing — which would under-count the fleet
// and make a still-running worker look idle. A nil slot yields a no-op releaser.
//
// The decrement happens under slot.mu, not as a bare atomic, and that is what makes
// "was I last?" trustworthy: N workers finishing concurrently all reach zero in some
// order, but exactly ONE observes the zero-crossing. Reading the counter separately
// after an unlocked decrement let several finishers each see 0 and each claim to be
// last, which duplicated the all-idle note (observed as 3 copies under load).
func releaseOnce(slot *coordSlot) func() bool {
	if slot == nil {
		return func() bool { return false }
	}
	var (
		once bool
		last bool
		mu   sync.Mutex
	)
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		if once {
			return last
		}
		once = true
		slot.mu.Lock()
		last = slot.workers.Add(-1) == 0
		slot.mu.Unlock()
		return last
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
	mu        sync.Mutex         // guards cancelFn (re-pointed each idle-resume attempt)
	cancelFn  context.CancelFunc // the turn attempt currently in flight
	stopped   atomic.Bool
	teardown  atomic.Bool // reversible delete preparation; drain preserves queued work
	startedAt time.Time
	done      chan struct{}
	// run is THIS worker turn's activeSessions registration (newWorkerRun). It rides
	// on the ctl because the ctl already threads through every frame that has to
	// release it (runWorkerRegistered → runWorkerWithCtl → drainWorkerQueue), so no
	// signature grows. Nil for the workerRunFn test seam, which registers a bare ctl
	// and never tracks the session; release() is nil-safe.
	run *sessionRun
}

var errWorkerNotRunning = errors.New("worker is not running")

// setCancel installs the cancel of the turn attempt now running. The idle-resume
// loop calls it once per attempt so a coordinator stop_worker always aborts the
// attempt actually in flight, not a stale (already-cancelled) one from before a
// resume.
func (c *workerCtl) setCancel(fn context.CancelFunc) {
	c.mu.Lock()
	c.cancelFn = fn
	c.mu.Unlock()
}

// cancel aborts the attempt currently in flight. Nil-safe in the brief window
// before the first attempt registers its cancel (a stop that lands there still sets
// stopped, which the invoke closure re-checks before running).
func (c *workerCtl) cancel() {
	c.mu.Lock()
	fn := c.cancelFn
	c.mu.Unlock()
	if fn != nil {
		fn()
	}
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
			return tools.SpawnResult{
				SessionID:       res.SessionID,
				AgentName:       res.AgentName,
				Queued:          res.Queued,
				QueuePosition:   res.QueuePosition,
				TreeBudgetUsed:  res.TreeBudgetUsed,
				TreeBudgetTotal: res.TreeBudgetTotal,
			}, nil
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
		f.ListRows = func(c context.Context, subtree bool) ([]tools.WorkerRow, error) {
			var ws []WorkerInfo
			if subtree {
				sub, err := r.ListSubtreeWorkers(c, coordID)
				if err != nil {
					return nil, err
				}
				for _, sw := range sub {
					ws = append(ws, sw.WorkerInfo)
				}
			} else {
				direct, err := r.ListWorkers(c, coordID)
				if err != nil {
					return nil, err
				}
				ws = direct
			}
			rows := make([]tools.WorkerRow, 0, len(ws))
			for _, w := range ws {
				rows = append(rows, tools.WorkerRow{
					SessionID:  w.SessionID,
					AgentName:  w.AgentName,
					Title:      w.Title,
					Running:    w.Running,
					Delegating: w.Delegating,
					Queued:     w.Queued,
					Stuck:      w.Stuck,
					Summary:    w.Summary,
					CreatedAt:  w.CreatedAt,
					UpdatedAt:  w.UpdatedAt,
				})
			}
			return rows, nil
		}
	}
	if sess.CoordinatorSessionID != "" {
		f.Report = func(c context.Context, status, summary string) error {
			return r.ReportToCoordinator(c, coordID, status, summary)
		}
	}
	f.Trajectory = r.trajectoryFuncsFor(sess)
	return f
}

// workerSpecFor turns the tool-layer spawn options into the runtime spec,
// resolving a requested coordinator recipe through the same gate the UI and the
// flow node use. An unknown or wrong-kind slug fails the spawn rather than
// falling back to free coordination — a sub-coordinator silently running a
// different plan than it was given is worse than a refused spawn.
func (r *Runtime) workerSpecFor(spec tools.WorkerSpawnSpec) (WorkerSpec, error) {
	out := WorkerSpec{
		ExpectedDeliverables: append([]string(nil), spec.ExpectedDeliverables...),
		ModelOverride:        spec.ModelOverride,
		Coordinator:          spec.Coordinator,
		Workflow:             strings.TrimSpace(spec.Workflow),
		WorkingDir:           strings.TrimSpace(spec.WorkingDir),
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
	if f.Trajectory != nil {
		defs = append(defs, tools.NewTrajectoryTool().Def())
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
	case tools.TrajectoryToolName:
		t = tools.NewTrajectoryTool()
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
		switch {
		case w.Delegating:
			// Not a turn of its own: it is waiting on its branch or owes a report.
			// "finished" here would read as "its result is in".
			status = "delegating"
		case w.Running:
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
	created := &coordSlot{
		admission: make(chan struct{}, 1),
		wake:      make(chan struct{}, 1),
	}
	created.admission <- struct{}{}
	v, _ := r.coordSlots.LoadOrStore(coordSessionID, created)
	return v.(*coordSlot)
}

// acquireCoordinatorAdmission blocks until this caller owns the session's coordinator
// admission token and returns its release. Worker-note persistence and the drain's
// turn-start gate share the token, so a note that lands before the gate is either
// consumed by the turn about to run or stays armed for a later one.
func acquireCoordinatorAdmission(ctx context.Context, slot *coordSlot) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-slot.admission:
		return func() { slot.admission <- struct{}{} }, nil
	}
}

// isSessionActive reports whether a session holds at least one autonomous-turn
// registration — running OR queued (used by ListWorkers to distinguish running
// from finished workers).
func (r *Runtime) isSessionActive(id string) bool {
	r.activeMu.Lock()
	defer r.activeMu.Unlock()
	return len(r.activeSessions[id]) > 0
}

// IsSessionActive is isSessionActive for callers outside the package (the
// coordinator-tree endpoint, which marks live nodes in the tree view).
func (r *Runtime) IsSessionActive(id string) bool { return r.isSessionActive(id) }

// HasQueuedMessage is the exported view of hasQueuedMessage for the api layer (the
// coordinator tree endpoint), so a node with a parked send_to_worker follow-up can
// show the same "queued" badge the flat roster does.
func (r *Runtime) HasQueuedMessage(id string) bool { return r.hasQueuedMessage(id) }

// WorkerSpec describes one spawn_worker request beyond the plain target/task
// pair: whether the new worker is itself a coordinator (the nesting switch) and,
// if so, which recipe it runs under.
type WorkerSpec struct {
	ExpectedDeliverables []string
	ModelOverride        string
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
	// WorkingDir pins the worker's cwd. Empty inherits the COORDINATOR's cwd (see
	// SpawnWorker), not the workspace default: a coordinator working in repo A must
	// not fan out workers that land in repo B.
	WorkingDir string
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
	if r.spawnBlockedByTreeTeardown(ctx, rootID, coordSessionID) {
		return SpawnResult{}, fmt.Errorf("coordinator branch %s is being torn down; cannot spawn a new worker", coordSessionID)
	}
	// Fold in the target agent's own coordinator DEFAULT (Agent.CoordinatorMode).
	// The agent default can only ADD the capability, never remove one the caller
	// explicitly asked for — so a team whose CTO is configured as a coordinator
	// nests correctly even when the spawning prompt forgot `coordinator: true`.
	// Resolved BEFORE the budget check because the depth guard's answer differs for
	// a coordinator vs a plain worker.
	coordinator, workflow, maxTurns := spec.Coordinator, spec.Workflow, spec.WorkflowMaxTurns
	if !coordinator {
		if mode, wf := r.agentCoordinatorDefaults(ctx, agentRef); mode {
			// Degrade SILENTLY at the depth ceiling instead of failing the spawn: an
			// explicit `coordinator: true` is a request that must be answered (see
			// checkCoordinatorTreeBudget), but a default is only a preference — the
			// caller asked for a worker and a working plain worker is the right answer.
			if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth <= 0 || depth < maxDepth {
				coordinator = true
				if workflow == "" && wf != "" {
					// An unresolvable default recipe drops to free coordination rather
					// than failing a spawn nobody asked to be recipe-driven.
					if mt, err := skills.ResolveCoordinatorWorkflow(r.Skills(), wf); err == nil {
						workflow, maxTurns = wf, mt
					} else {
						r.logger.Warn("coordination: ignoring agent default recipe",
							"agent", agentRef, "workflow", wf, "error", err)
					}
				}
			} else {
				r.logger.Info("coordination: agent coordinator default degraded to plain worker at depth limit",
					"agent", agentRef, "depth", depth)
			}
		}
	}
	budget, err := r.checkCoordinatorTreeBudget(ctx, parent, rootID, depth, coordinator)
	if err != nil {
		return SpawnResult{}, err
	}
	// A profile target resolves to its persistent system agent. An ordinary target
	// passes through unchanged (existing agent name/id). Resolved BEFORE the
	// worker-count reservation so a bad target never leaks a slot.
	agentRef, err = r.resolveWorkerTarget(ctx, coordSessionID, createdBy, agentRef)
	if err != nil {
		return SpawnResult{}, err
	}
	// Capability gate: refuse a brief that plainly needs to write when the resolved
	// agent has no write/exec tool. Checked AFTER target resolution (the profile's
	// allowlist is only known then) and BEFORE the worker-slot reservation, so a
	// rejected spawn leaks nothing.
	if target, terr := r.resolveAgent(ctx, agentRef); terr == nil {
		if cerr := r.checkWorkerCapability(target, task); cerr != nil {
			return SpawnResult{}, cerr
		}
	} else {
		// resolveWorkerTarget already returned an id, so a failure here is a real
		// store problem, not a bad target: surface it instead of spawning blind.
		return SpawnResult{}, fmt.Errorf("cannot read worker agent %s: %w", agentRef, terr)
	}
	slot := r.coordSlotFor(coordSessionID)
	max := int64(r.tun.CoordinatorMaxWorkers())
	if slot.workers.Add(1) > max {
		slot.workers.Add(-1)
		return SpawnResult{}, fmt.Errorf("coordinator worker limit reached (%d active); wait for some to finish before spawning more", max)
	}
	// A newly spawned worker opens a FRESH all-idle transition: re-arm the sweep and
	// drop the fold claim, so this wave's completion is reported on its own merits.
	slot.mu.Lock()
	slot.ackedIdle = false
	slot.idleFolded = false
	slot.mu.Unlock()
	// Pin the worker's cwd: an explicit request wins, otherwise inherit the
	// coordinator's own directory. Falling through to the workspace default (what
	// SpawnSession does on an empty value) silently dropped fan-out workers into an
	// unrelated repository whenever the coordinator itself had been moved.
	cwd := strings.TrimSpace(spec.WorkingDir)
	if cwd == "" {
		cwd = strings.TrimSpace(parent.WorkingDir)
	}
	// The reservation above is released exactly once, by whichever path ends this
	// worker before it ever reaches runWorkerWithCtl: a shutdown drop of the queued
	// item, or SpawnSession failing outright. Both must report the zero-crossing —
	// only the observer of the transition may fold the all-idle note.
	dropped := releaseOnce(slot)
	contractCwd := cwd
	if contractCwd == "" {
		contractCwd = r.WorkspaceDefaultDir()
	}
	expected, deliverableErr := normalizeWorkerDeliverables(spec.ExpectedDeliverables, contractCwd)
	if deliverableErr != nil {
		dropped()
		return SpawnResult{}, deliverableErr
	}
	if len(expected) > 0 {
		task += "\n\nRequired delivery files (presence checked before completion; independent acceptance remains separate):\n" + strings.Join(expected, "\n") + "\nWrite and verify these files before reporting completion."
	}
	res, err := r.SpawnSession(ctx, agentRef, task, SpawnOptions{
		ExpectedDeliverables:     expected,
		ModelOverride:            spec.ModelOverride,
		WorkingDir:               cwd,
		CreatedBy:                createdBy,
		RuntimeBaseAgentID:       createdBy,
		CoordinatorSessionID:     coordSessionID,
		Role:                     db.SessionRoleWorker,
		RootCoordinatorSessionID: rootID,
		CoordinatorDepth:         depth,
		CoordinatorMode:          coordinator,
		// Versioned ref ("slug@version") when the recipe declares one, so the
		// sub-coordinator's trajectory records the revision it followed (R6).
		CoordinatorWorkflow: skills.RecipeRefFor(r.Skills(), workflow),
		CoordinatorMaxTurns: maxTurns,
		onDrop:              func(error) bool { return dropped() },
	})
	if err != nil {
		// SpawnSession never launched runWorker, so release the reservation here.
		// No notification is sent on this path — the tool call hands the error back to
		// the coordinator's live turn — so the zero-crossing needs no further routing.
		dropped()
		return SpawnResult{}, err
	}
	slot.markHadWorkers()
	r.observeSpawn(SpawnEvent{
		CoordinatorID: coordSessionID, WorkerID: res.SessionID, RootID: rootID, Depth: depth,
		AgentRef: agentRef, AgentName: res.AgentName, SubCoordinator: coordinator,
		Workflow: workflow, Queued: res.Queued,
	})
	// Surface the tree's live-worker occupancy so the coordinator sees remaining
	// quota on every spawn. budget.used counted the LIVE workers before this call;
	// the worker just created occupies one more slot now. Total 0 (unlimited) leaves
	// both fields zero, which the tool reads as "no budget line to show".
	res.TreeBudgetTotal = budget.total
	if budget.total > 0 {
		res.TreeBudgetUsed = budget.used + 1
	}
	return res, nil
}

// coordinatorTreeLocks holds one spawn lock per coordinator TREE, keyed by root id.
// Entries are reference-counted and removed when the last holder releases, so a
// long-lived process does not accumulate one mutex per coordinator tree it ever ran.
var (
	coordinatorTreeLocksMu sync.Mutex
)

// Reversible worker teardown for session deletion (StopWorkerForTeardown /
// FinishWorkerTeardown and the tree-teardown spawn gate) lives in
// coordination_teardown.go.

// The per-session turn lock itself lives in turnslot.go / internal/turnqueue; this
// file only decides WHETHER the coordinator wants another turn.
