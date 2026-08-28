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

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/turnqueue"
	"github.com/bilal-arikan/tionharness/internal/view"
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
	// driving marks that a drainCoordinator goroutine owns this coordinator's
	// auto-turn loop. It is NOT "a turn is running" (ask the queue for that): it
	// exists so concurrent notifications coalesce into the ONE loop instead of
	// starting a second one.
	driving    bool
	pending    bool         // >=1 notification arrived mid-turn; run once more after
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
	// stallHalted is the hard-halt escalation flag: set once the nudge budget is spent
	// AND the coordinator is STILL judged to be phantom-spawning. It stops the drain
	// loop from re-arming (or running the idle-reconcile turn) so a wedged coordinator
	// no longer burns auto-turns, and gates the one-shot user-facing halt notice. Reset
	// to false by any turn that actually calls a coordination tool (genuine recovery).
	// See guardCoordinatorStall / escalateCoordinatorStallHalt (coordination_stall.go).
	stallHalted bool
	// stopRequested is set by runCoordinatorTurn when the live coordinator turn ended
	// on a human Stop (plain context.Canceled, distinct from a watchdog cut). The drain
	// loop consumes it right after the turn returns and EXITS without running the
	// idle-reconcile turn — otherwise a manual Stop on a coordinator whose workers had
	// all finished would inject a fresh <coordination-status> note and run one more
	// turn, so the session looked like it "kept going" after the user stopped it.
	// One-shot: a later worker notification re-arms the loop via enqueueCoordinatorTurn.
	stopRequested bool
	// lastTurnUnix is the wall-clock (unix seconds) at which this coordinator's last
	// real turn finished. 0 until the first real turn ran (a stubbed test never sets
	// it). The stall sweeper reads it to find coordinators gone silent past the
	// staleness window.
	lastTurnUnix int64
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
	startedAt time.Time
}

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
		WorkingDir:    strings.TrimSpace(spec.WorkingDir),
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

// HasQueuedMessage is the exported view of hasQueuedMessage for the api layer (the
// coordinator tree endpoint), so a node with a parked send_to_worker follow-up can
// show the same "queued" badge the flat roster does.
func (r *Runtime) HasQueuedMessage(id string) bool { return r.hasQueuedMessage(id) }

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
	slot := r.coordSlotFor(coordSessionID)
	max := int64(r.tun.CoordinatorMaxWorkers())
	if slot.workers.Add(1) > max {
		slot.workers.Add(-1)
		return SpawnResult{}, fmt.Errorf("coordinator worker limit reached (%d active); wait for some to finish before spawning more", max)
	}
	// Pin the worker's cwd: an explicit request wins, otherwise inherit the
	// coordinator's own directory. Falling through to the workspace default (what
	// SpawnSession does on an empty value) silently dropped fan-out workers into an
	// unrelated repository whenever the coordinator itself had been moved.
	cwd := strings.TrimSpace(spec.WorkingDir)
	if cwd == "" {
		cwd = strings.TrimSpace(parent.WorkingDir)
	}
	res, err := r.SpawnSession(ctx, agentRef, task, SpawnOptions{
		ModelOverride:            spec.ModelOverride,
		WorkingDir:               cwd,
		CreatedBy:                createdBy,
		RuntimeBaseAgentID:       createdBy,
		CoordinatorSessionID:     coordSessionID,
		Role:                     db.SessionRoleWorker,
		RootCoordinatorSessionID: rootID,
		CoordinatorDepth:         depth,
		CoordinatorMode:          coordinator,
		CoordinatorWorkflow:      workflow,
		CoordinatorMaxTurns:      maxTurns,
		onDrop: func(error) {
			slot.workers.Add(-1)
		},
	})
	if err != nil {
		// SpawnSession never launched runWorker, so release the reservation here.
		slot.workers.Add(-1)
		return SpawnResult{}, err
	}
	slot.markHadWorkers()
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
var coordinatorTreeLocks sync.Map

// lockCoordinatorTree serializes budget-check-plus-create for one coordinator tree
// and returns the unlock func.
func lockCoordinatorTree(rootID string) func() {
	v, _ := coordinatorTreeLocks.LoadOrStore(rootID, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// liveWorkerRef names one worker that still occupies a tree-budget slot, for the
// exhaustion error's "still active" listing.
type liveWorkerRef struct {
	SessionID  string
	AgentName  string
	Delegating bool // live only via its own running branch, not a turn of its own
}

// coordTreeBudget is a snapshot of a coordinator tree's LIVE-worker occupancy
// against its ceiling. A worker counts while it can still do or spawn work; a
// worker that has concluded, failed, or been stopped is reclaimed and no longer
// counts — which is exactly what the exhaustion error has always promised
// ("conclude existing workers before spawning more"). total <= 0 means unlimited.
type coordTreeBudget struct {
	used  int
	total int
	live  []liveWorkerRef
}

// exhausted reports whether a further worker would exceed the ceiling.
func (b coordTreeBudget) exhausted() bool { return b.total > 0 && b.used >= b.total }

// activeList renders the still-counted workers for the exhaustion error, so a
// coordinator learns WHICH workers hold the budget rather than only that it is
// full.
func (b coordTreeBudget) activeList() string {
	if len(b.live) == 0 {
		return "No workers are currently counted as active."
	}
	var sb strings.Builder
	sb.WriteString("Still counted as active: ")
	for i, w := range b.live {
		if i > 0 {
			sb.WriteString(", ")
		}
		role := ""
		if w.Delegating {
			role = " (delegating sub-coordinator)"
		}
		fmt.Fprintf(&sb, "%s%s [%s]", w.AgentName, role, w.SessionID)
	}
	sb.WriteString(".")
	return sb.String()
}

// countsAgainstTreeBudget reports whether a worker session still occupies a slot
// in its tree's budget. Mirrors workerInfoFor's liveness (a running turn, or a
// sub-coordinator whose own branch is still live) without the message fetch, so
// it is cheap to call for every node while holding the tree lock. A
// concluded/failed/stopped worker returns false and is reclaimed.
func (r *Runtime) countsAgainstTreeBudget(s db.Session) bool {
	if r.isSessionActive(s.ID) {
		return true
	}
	if s.IsCoordinator() && r.coordSlotFor(s.ID).workers.Load() > 0 {
		return true
	}
	return false
}

// evalCoordinatorTreeBudget walks the whole tree and counts the workers that are
// still LIVE (see countsAgainstTreeBudget). Finished workers are reclaimed rather
// than held forever, so a long-running coordinator is not permanently bricked by
// the sessions of work it already completed. total <= 0 short-circuits (no walk).
func (r *Runtime) evalCoordinatorTreeBudget(ctx context.Context, parent db.Session, rootID string) (coordTreeBudget, error) {
	b := coordTreeBudget{total: r.tun.CoordinatorMaxSubtreeSessions()}
	if b.total <= 0 {
		return b, nil
	}
	tree, err := r.db.ListCoordinatorTree(ctx, rootID)
	if err != nil {
		// The root is gone (deleted mid-run). Fall back to the parent's own subtree
		// so the budget still bites rather than silently disappearing.
		if tree, err = r.db.ListCoordinatorTree(ctx, parent.ID); err != nil {
			return coordTreeBudget{}, fmt.Errorf("cannot verify coordinator tree budget: %w", err)
		}
	}
	for _, s := range tree {
		// The root has no coordinator parent; everything below it is a worker. (A
		// worker always carries CoordinatorSessionID, so this cleanly skips the root
		// regardless of which node the walk normalized to.)
		if s.CoordinatorSessionID == "" {
			continue
		}
		if !r.countsAgainstTreeBudget(s) {
			continue // concluded/failed/stopped: reclaimed, no longer holds a slot
		}
		b.used++
		b.live = append(b.live, liveWorkerRef{
			SessionID:  s.ID,
			AgentName:  r.agentName(s.AgentID),
			Delegating: !r.isSessionActive(s.ID), // live only through its branch
		})
	}
	return b, nil
}

// checkCoordinatorTreeBudget enforces the TREE-WIDE guards before a worker session
// is created: nesting depth and the live-worker count of the whole tree. Both fail
// loudly — a caller that hits a ceiling gets an error naming the limit, never a
// quietly downgraded worker, because a coordinator that believes it delegated work
// it did not delegate stalls waiting for a report that will never come. On success
// it returns the budget snapshot so the caller can report remaining quota.
func (r *Runtime) checkCoordinatorTreeBudget(ctx context.Context, parent db.Session, rootID string, depth int, wantCoordinator bool) (coordTreeBudget, error) {
	if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && depth > maxDepth {
		return coordTreeBudget{}, fmt.Errorf("coordinator depth limit reached (max %d levels; this worker would sit at depth %d). Do this work in the current session, or ask your own coordinator to restructure the plan", maxDepth, depth)
	}
	// Spawning a NON-coordinator leaf at the last allowed level is fine; only the
	// sub-coordinator itself needs room for a level below it.
	if wantCoordinator {
		if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && depth >= maxDepth {
			return coordTreeBudget{}, fmt.Errorf("cannot spawn a sub-coordinator at depth %d: its own workers would exceed the coordinator depth limit (max %d). Spawn a plain worker here instead", depth, maxDepth)
		}
	}
	budget, err := r.evalCoordinatorTreeBudget(ctx, parent, rootID)
	if err != nil {
		return coordTreeBudget{}, err
	}
	if budget.exhausted() {
		return budget, fmt.Errorf("coordinator tree budget exhausted (%d/%d LIVE worker sessions across the whole tree). %s Stop or conclude a still-running worker before spawning more (finished workers are already reclaimed)", budget.used, budget.total, budget.activeList())
	}
	return budget, nil
}

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
	if receipt.Status == db.DeliveryHeld {
		return tools.SendResult{Held: true, ReceiptID: receipt.ID}, nil
	}

	// Backpressure instead of rejection: a worker mid-turn no longer loses the
	// message. The busy-check and the enqueue are done under workerQueueMu in one
	// critical section so they stay atomic against drainWorkerQueue, which pops
	// under the same mutex once the turn ends (see runWorker's deferred drain).
	// isSessionActive flips false BEFORE that drain runs, so any message accepted
	// here (active == true) is guaranteed to be seen by the drain — no lost update.
	r.workerQueueMu.Lock()
	if r.isSessionActive(workerSessionID) {
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

// deliverToWorker re-dispatches a previously HELD worker follow-up once it has
// been approved (see inbound.go). It re-reads the worker session for its current
// coordinator/depth rather than trusting stale values captured at hold time.
func (r *Runtime) deliverToWorker(ctx context.Context, workerSessionID, message string) error {
	ws, err := r.db.GetSession(ctx, workerSessionID)
	if err != nil {
		return fmt.Errorf("worker session %s not found: %w", workerSessionID, err)
	}
	agent, err := r.db.GetAgent(ctx, ws.AgentID)
	if err != nil {
		return fmt.Errorf("worker agent gone: %w", err)
	}
	if err := r.applyProfileAllowlist(&agent); err != nil {
		return err
	}
	r.workerQueueMu.Lock()
	if r.isSessionActive(workerSessionID) {
		queue := r.workerQueue[workerSessionID]
		if len(queue) >= maxWorkerQueueDepth {
			r.workerQueueMu.Unlock()
			return fmt.Errorf("worker %s queue is full (max %d)", workerSessionID, maxWorkerQueueDepth)
		}
		r.workerQueue[workerSessionID] = append(queue, message)
		r.workerQueueMu.Unlock()
		return nil
	}
	r.workerQueueMu.Unlock()
	return r.dispatchWorkerTurn(ctx, agent, workerSessionID, message, ws.CoordinatorSessionID, ws.CoordinatorDepth)
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
// untrackSession, so isSessionActive is already false and the delivery re-runs
// the worker cleanly. The pop is done under workerQueueMu (the same mutex
// SendToWorker enqueues under) so an enqueue that raced the turn end is either
// fully visible here or already took the idle path. Runs on context.Background:
// the worker turn's ctx is cancelled by now.
func (r *Runtime) drainWorkerQueue(agent db.Agent, workerSessionID, coordSessionID string) {
	r.workerQueueMu.Lock()
	messages, ok := r.workerQueue[workerSessionID]
	if ok {
		delete(r.workerQueue, workerSessionID)
	}
	r.workerQueueMu.Unlock()
	if !ok {
		return
	}
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
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
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
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Stuck:     hasSessionTag(s.Tags, "stuck"),
	}
	if !info.Running && s.IsCoordinator() && r.coordSlotFor(s.ID).workers.Load() > 0 {
		info.Running = true
		info.Delegating = true
	}
	if info.Running {
		if v, ok := r.workerCancels.Load(s.ID); ok {
			info.StartedAt = v.(*workerCtl).startedAt.Unix()
		}
		info.Queued = r.hasQueuedMessage(s.ID)
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
	// Registered first so the global lifecycle slot is released last. Test/runtime
	// shutdown uses spawnActive as the definitive drain barrier; dropping it before
	// queue finalization lets cleanup race the goroutine's final store access.
	defer r.releaseSpawnSlot()
	// Drain after every per-turn slot/cancel/tracking cleanup but before releasing
	// the global lifecycle slot. A parked follow-up can then start from an idle
	// worker while shutdown still sees this goroutine as active. No-op when empty.
	defer r.drainWorkerQueue(agent, workerSessionID, coordSessionID)
	if slot := r.coordSlotFor(coordSessionID); slot != nil {
		defer slot.workers.Add(-1)
	}

	// Hard wall-clock ceiling (settings-driven, same as spawns) PLUS an idle
	// watchdog: a worker/coordinator turn that streams no step for SpawnIdleTimeout
	// is reclaimed fast, while a long-but-productive one runs up to SpawnTimeout.
	hardCap, idleCap := r.tun.SpawnTimeout(), r.tun.SpawnIdleTimeout()
	ctl := &workerCtl{startedAt: time.Now()}
	r.workerCancels.Store(workerSessionID, ctl)
	defer r.workerCancels.Delete(workerSessionID)

	// Serialize this worker turn on the WORKER session's own turn slot (keyed by
	// workerSessionID, distinct from the coordinator slot whose workers counter is
	// decremented above) so it never overlaps another turn on the same worker
	// session: a second send_to_worker that raced the isSessionActive check (that
	// check is a UI hint, not a lock), or a user/wake/peer turn opened on the worker
	// session (all of which now claim this same slot).
	releaseSlot := r.claimSessionTurnSlot(workerSessionID, turnqueue.KindWorker, "worker görevi")
	defer releaseSlot()

	// Own cancelable context for this worker turn so a human "Durdur"
	// (CancelSession) can stop it, alongside the coordinator's own stop_worker.
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	r.trackSession(workerSessionID, cancelRun)
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
	turnStart := time.Now()
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(runCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, cancel context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			ctl.setCancel(cancel)
			if ctl.stopped.Load() {
				return "", nil, context.Canceled
			}
			turnCtx = tools.WithAsyncChat(WithSessionID(WithCallKind(attemptCtx, KindSpawn), workerSessionID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := prompt
			if attempt > 1 {
				p = resumeContinuationPrompt(prompt, prevOutput)
			}
			return r.runSessionTurn(turnCtx, agent, workerSessionID, p, true)
		})
	defer cancel()
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
	// Persist the terminal outcome AFTER AddMessage updates the shared lifetime
	// counters. mutateSessionLocked rewrites session.json, making the same
	// MessageCount/ToolCallCount arithmetic used by chat durable for worker turns.
	// The error is surfaced because a missing terminal write must stay observable.
	if rsErr := r.db.SetSessionRunState(ctx, workerSessionID, status, time.Now().Unix()); rsErr != nil {
		r.logger.Error("worker: failed to persist terminal session state", "session", workerSessionID, "status", status, "error", rsErr)
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

	// The report the coordinator actually reads: the successful output, or the
	// failure/kill note (so the coordinator can react to failures too). buildWorkerResult
	// bounds how much of it enters the coordinator's context — an overflowing result is
	// capped and its full text offloaded to an artifact + worker-session handle, so a
	// single verbose worker can no longer fill the coordinator's window (_Docs/47, P0/P2).
	notifyResult := r.buildWorkerResult(ctx, workerSessionID, agent.ID, status, replyText)
	note := formatTaskNotification(workerSessionID, agent.Name, status, notifyResult, countToolSteps(steps), time.Since(turnStart).Milliseconds())
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
	// Same byte limit as send_message / send_to_worker, applied differently: this
	// path is ONE-WAY (the worker turn that produced the note is already over), so
	// there is nobody to hand a message_too_large error back to. Refusing here
	// would leave the coordinator waiting forever for a worker that has finished,
	// so an oversized note is cut and the cut is stated explicitly instead.
	if capped, cut := capNotification(note, r.tun.AgentMessageMaxBytes()); cut {
		r.logger.Warn("coordination: task-notification capped",
			"coordinator", coordSessionID, "bytes", len(note), "max", r.tun.AgentMessageMaxBytes())
		note = capped
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

// The per-session turn lock itself lives in turnslot.go / internal/turnqueue; this
// file only decides WHETHER the coordinator wants another turn.

// enqueueCoordinatorTurn schedules one coordinator turn. If a drain loop already
// owns this coordinator it just flags pending — that loop will run once more and
// see the freshly-persisted notification in history (coalescing). Otherwise it
// starts the loop, which queues for the session's turn slot like any other caller.
func (r *Runtime) enqueueCoordinatorTurn(coordSessionID string) {
	// Archive is a HARD stop on automatic turns, and it has to be enforced here --
	// the wake entry point -- not only in RecoverOrphanedTurns. WHY: on 2026-08-27
	// archiving the three runaway coordinators in WS5 was not enough. Their orphaned
	// workers are still reclaimed at boot, and each reclaim calls NotifyCoordinator,
	// which lands right here and starts a drain on a session the user had explicitly
	// archived. The whole subtree had to be archived by hand to break the loop.
	// The note itself is already durably recorded by the caller, so nothing is lost:
	// un-archiving and resuming replays it.
	if sess, err := r.db.GetSession(context.Background(), coordSessionID); err == nil && sess.State == "archived" {
		r.logger.Info("coordination: skipping turn, coordinator session is archived", "session", coordSessionID)
		return
	}
	slot := r.coordSlotFor(coordSessionID)
	slot.mu.Lock()
	// A hard stall halt must gate the wake entry point, not only a drain already in
	// progress. Worker notes remain durably recorded, but cannot silently start a
	// fresh automatic drain while the UI says auto-turns are stopped. Resume clears
	// the flag and explicitly enqueues the next turn, preserving all queued notes.
	if slot.stallHalted {
		slot.mu.Unlock()
		return
	}
	// A fresh notification (a worker just finished or continued) re-arms the
	// idle-reconcile sweep: this batch is no longer "acknowledged idle".
	slot.ackedIdle = false
	if slot.driving {
		slot.pending = true
		slot.mu.Unlock()
		return
	}
	slot.driving = true
	slot.mu.Unlock()
	go r.drainCoordinator(coordSessionID, slot)
}

// drainCoordinator runs coordinator turns until no more notifications are pending,
// bounded by CoordinatorMaxTurns (the notify-loop guard). Each iteration runs one
// history-aware turn that sees every notification persisted so far.
//
// The loop re-enters the session's admission queue EVERY iteration rather than
// holding the slot across the drain. That is the fairness property: a message the
// user queued mid-drain is already in the FIFO, so it runs after the current turn —
// not after the whole drain. Nothing is lost by yielding; the notification that
// re-armed us is persisted in history and slot.pending carries the intent.
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
			slot.pending = false
			slot.driving = false
			slot.mu.Unlock()
			if warn {
				r.warnCoordinatorCap(coordSessionID, slot.turns)
			}
			return
		}
		slot.mu.Unlock()

		// Queue for the slot like everyone else. Whatever is ahead of us — a user
		// message, a /compact, a peer delivery — runs first.
		release := r.claimSessionTurnSlot(coordSessionID, turnqueue.KindCoordinator, "worker bildirimi")
		slot.mu.Lock()
		// Count the auto-turn HERE, not before the wait: while we were queued a user
		// turn may have reset the cap (a human is back in the loop), and a turn that
		// never ran must not spend the budget.
		slot.turns++
		// Consume the notification(s) that armed this iteration: the turn about to run
		// is history-aware, so it sees every note persisted so far, including any that
		// landed while we waited for the slot.
		slot.pending = false
		slot.mu.Unlock()

		if r.coordRunFn != nil {
			r.coordRunFn(coordSessionID)
		} else {
			r.runCoordinatorTurn(coordSessionID)
		}
		release()

		slot.mu.Lock()
		// Hard-halt escalation (FND-99caeb31): the turn-end stall guard just spent the
		// last nudge on a coordinator STILL narrating phantom spawns and escalated to a
		// halt. Stop auto-turning it — no re-arm, and skip the idle-reconcile turn below
		// (which would otherwise hand the wedged model one more shot). A real worker
		// notification (enqueueCoordinatorTurn) still starts a fresh drain, and a turn
		// that finally calls a coordination tool clears the flag. The sweeper stays live
		// as the long-horizon backstop.
		if slot.stallHalted {
			slot.stopRequested = false
			slot.pending = false
			slot.driving = false
			slot.mu.Unlock()
			return
		}
		if slot.pending {
			// A real worker notification supersedes an earlier human Stop: a still-running
			// or just-finished worker is allowed to continue the coordinator (only the
			// no-pending idle-reconcile is suppressed by a Stop — see below).
			slot.stopRequested = false
			slot.mu.Unlock()
			continue
		}
		// Human Stop honoured (see coordSlot.stopRequested): the user cancelled the live
		// coordinator turn AND no worker notification is pending. Suppress ONLY the
		// idle-reconcile turn below — running it would inject a fresh <coordination-status>
		// note and hand the coordinator one more turn, making a manual Stop look like it
		// kept going. Consumed one-shot; a still-running worker re-arms the loop via
		// enqueueCoordinatorTurn when it finishes (workers keep running through a Stop).
		stopped := slot.stopRequested
		slot.stopRequested = false
		// Idle reconciliation (liveness backstop): if EVERY worker is now finished
		// and we have not yet run a reconcile turn for this all-idle transition,
		// inject an authoritative "all workers finished" note and loop ONCE more.
		// This guarantees the coordinator gets a final, unambiguous turn even when
		// it overlooked one notification in a coalesced batch — breaking the "waits
		// forever on an already-finished worker" stall. One-shot per all-idle
		// transition (ackedIdle, re-armed by the next notification) and bounded by
		// CoordinatorMaxTurns (checked at the loop top), so it can never loop.
		if !stopped && !slot.ackedIdle && slot.hadWorkers && slot.workers.Load() == 0 {
			slot.ackedIdle = true
			slot.mu.Unlock()
			r.appendCoordinationStatus(coordSessionID)
			continue
		}
		slot.driving = false
		slot.mu.Unlock()
		// No worker and no queued notification can wake this coordinator now.
		r.markCoordinatorBlocked(context.Background(), coordSessionID)
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
		busy := slot.driving || slot.pending
		slot.mu.Unlock()
		// Also check the admission queue: a turn from ANY path (a user message, a
		// peer delivery) may have taken this session meanwhile — the node is alive
		// and should report for itself.
		if busy || r.sessionTurnBusy(coordSessionID) {
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
	v, err := view.ProjectWorkers(view.WorkersInput{Workers: toViewWorkers(ws)}, view.LevelCard)
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

	// Metadata reads are quick and must not be bound to the turn watchdog (which the
	// resume loop owns per attempt) — use the background context for them.
	sess, err := r.db.GetSession(context.Background(), coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: coordinator session gone", "coordinator", coordSessionID, "error", err)
		return
	}
	agent, err := r.db.GetAgent(context.Background(), sess.AgentID)
	if err != nil {
		r.logger.Warn("coordination: coordinator agent gone", "coordinator", coordSessionID, "error", err)
		return
	}

	// Single-shot idle-resume (FND-708844f8): an idle-cut coordinator DRAIN turn gets
	// ONE more attempt under a fresh window before reconcileTurnOutcome marks it
	// unfinished. Safe against the drain/stall machinery: the resume is transparent to
	// the notify loop (it just sees one longer turn); slot.lastTurnUnix is stamped
	// AFTER, and guardCoordinatorStall still runs only on a clean (err==nil, untruncated)
	// turn — a resumed-then-idle turn stays truncated and skips it, exactly as Faz E.
	// No double recovery: a coordinator reports UP via runWorker, not this drain turn.
	var (
		turnCtx context.Context
		meta    *turnMeta
	)
	turnStart := time.Now()
	// Own cancelable context for the drain turn so a human "Durdur"
	// (CancelSession) can stop the coordinator mid-drain.
	drainCtx, cancelDrain := context.WithCancel(context.Background())
	defer cancelDrain()
	r.trackSession(coordSessionID, cancelDrain)
	ctx, cancel, output, steps, err := r.runTurnWithIdleResume(drainCtx, hardCap, idleCap, r.tun.IdleResumeMax(),
		func(attemptCtx context.Context, _ context.CancelFunc, attempt int, prevOutput string) (string, []TurnStep, error) {
			turnCtx = tools.WithAsyncChat(WithSessionID(WithCallKind(attemptCtx, KindSpawn), coordSessionID))
			turnCtx, meta = WithTurnMeta(turnCtx)
			p := "" // the coordinator drains its inbox via history, not a prompt
			if attempt > 1 {
				p = resumeContinuationPrompt("", prevOutput)
			}
			return r.runSessionTurn(turnCtx, agent, coordSessionID, p, true)
		})
	defer cancel()
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
			// Signal the drain loop to exit without the idle-reconcile turn: honour the
			// human Stop instead of injecting a <coordination-status> note and running
			// once more (see coordSlot.stopRequested).
			stopSlot := r.coordSlotFor(coordSessionID)
			stopSlot.mu.Lock()
			stopSlot.stopRequested = true
			stopSlot.mu.Unlock()
		} else {
			text = "⚠️ Koordinatör turu çalıştırılamadı:\n\n" + err.Error()
			r.logger.Error("coordination: coordinator turn failed", "coordinator", coordSessionID, "error", err)
		}
	} else if strings.TrimSpace(text) == "" {
		text = "ℹ️ Koordinatör bu tur için boş yanıt döndürdü."
	}
	if DetectUnbackedSpawnClaim(text, turnToolNames(steps), sess.IsCoordinator()) {
		const warning = "⚠️ No worker was actually spawned this turn (no spawn tool call was made)."
		text = strings.TrimRight(text, "\n") + "\n\n" + warning
		r.emitDebug(ctx, db.DebugEvent{
			Type:    "guard",
			AgentID: agent.ID,
			Name:    "unbacked_spawn_claim",
			Detail:  "coordinator claimed worker delegation without a spawn tool call",
		})
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
// alike (mcp__tionharness_interaction__spawn_worker on the claude-cli path).
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
	target := map[string]string{
		"view":          "executions",
		"sessionId":     workerSessionID,
		"coordinatorId": coordSessionID,
		"phase":         "start",
	}
	r.tagRootCoordinator(target, coordSessionID)
	r.publish(events.Event{
		Type:   events.TypeWorker,
		Level:  "info",
		Title:  "🤖 Worker başladı — " + agent.Name,
		Target: target,
	})
}

// tagRootCoordinator adds "rootCoordinatorId" to a worker event's target when the
// direct coordinator is itself nested, i.e. the tree root is a DIFFERENT session.
//
// The frontend keys its running-worker banner on the coordinator id it is shown
// under, so a grandchild's transition tagged only with its direct (sub-)coordinator
// never reaches the root coordinator's open chat and its banner goes stale. The
// extra tag lets the root refetch on any transition below it.
//
// On a lookup failure the field is omitted rather than guessed: a wrong root would
// fan the refetch out to an unrelated chat, and the direct-coordinator tag still
// behaves exactly as before.
func (r *Runtime) tagRootCoordinator(target map[string]string, coordSessionID string) {
	sess, err := r.db.GetSession(context.Background(), coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: root coordinator lookup failed", "coordinator", coordSessionID, "err", err)
		return
	}
	if root := sess.RootCoordinator(); root != "" && root != coordSessionID {
		target["rootCoordinatorId"] = root
	}
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
	target := map[string]string{
		"view":          "executions",
		"sessionId":     workerSessionID,
		"coordinatorId": coordSessionID,
	}
	r.tagRootCoordinator(target, coordSessionID)
	r.publish(events.Event{
		Type:   events.TypeWorker,
		Level:  level,
		Title:  "🤖 Worker " + status + " — " + agent.Name,
		Target: target,
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
	if trimmed := strings.TrimSpace(result); trimmed != "" {
		// Defensive structural cap for EVERY caller (leaf-worker results are already
		// shaped by buildWorkerResult, but a sub-coordinator's report_to_coordinator
		// summary and the settle backstop's salvaged text arrive here uncapped). Cap
		// is rune-safe; overflow gets a short truncation notice.
		capped, _ := capText(trimmed, coordinatorResultCapChars)
		fmt.Fprintf(&b, "<result>%s</result>\n", capped)
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

func turnToolNames(steps []TurnStep) []string {
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Kind != StepTool {
			continue
		}
		if step.CallName != "" {
			names = append(names, step.CallName)
		} else {
			names = append(names, step.Tool)
		}
	}
	return names
}
