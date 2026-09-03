package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/skills"
	"github.com/bilal-arikan/tionharness/internal/trajectory"
)

// Trajectory binder (Rota F1a).
//
// The binder is the runtime-side writer of db.Trajectory. It creates the
// trajectory of a coordinator tree the first time the tree does something
// observable (or at creation, when the root carries a recipe) and appends the
// OBSERVED facts as they happen: a worker spawned, a worker reported, a flow
// run launched from inside the tree, a session forked off it, a durable ask
// opened or answered, the root archived. Declared facts (phases) come from the
// recipe at seed time or from the agent through the trajectory tool
// (trajectory_funcs.go).
//
// Every write goes through UpdateTrajectory with expectedRev 0: the binder
// only appends observations, which are valid on whatever revision is current,
// and the store's per-root lock serialises it against agent/UI edits. Failures
// are logged and dropped — a trajectory is a projection, never a reason to
// fail the work it describes.

// trajectoryBinder subscribes to the coordination observer seam.
type trajectoryBinder struct{ r *Runtime }

func nowMs() int64 { return time.Now().UnixMilli() }

// trajectoryIDForRoot returns the id of the root session's trajectory ("" when
// none) reading only the index.
func (r *Runtime) trajectoryIDForRoot(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}
	for _, e := range r.db.ListTrajectories(ctx, db.TrajectoryFilter{RootSessionID: root}) {
		return e.ID
	}
	return ""
}

// EnsureTrajectory returns the trajectory of a root session, creating it when
// missing: seeded from the root's coordinator recipe when it has one with a
// structured plan, otherwise a bare graph holding the root node. Safe to call
// concurrently — a lost create race re-reads the winner.
func (r *Runtime) EnsureTrajectory(ctx context.Context, rootID string) (db.Trajectory, error) {
	rootID = strings.TrimSpace(rootID)
	if rootID == "" {
		return db.Trajectory{}, errors.New("trajectory: empty root session id")
	}
	if t, err := r.db.GetTrajectoryByRoot(ctx, rootID); err == nil {
		return t, nil
	} else if !errors.Is(err, db.ErrNotFound) {
		return db.Trajectory{}, err
	}
	root, err := r.db.GetSession(ctx, rootID)
	if err != nil {
		return db.Trajectory{}, err
	}
	spec, ref := r.recipeSpecFor(root)
	created, err := r.db.CreateTrajectory(ctx, trajectory.Seed(root, spec, ref))
	if err == nil {
		r.logger.Info("trajectory seeded", "trajectory", created.ID, "root", rootID, "recipe", ref, "phases", len(trajectory.Phases(&created)))
		return created, nil
	}
	if errors.Is(err, db.ErrConflict) {
		return r.db.GetTrajectoryByRoot(ctx, rootID)
	}
	return db.Trajectory{}, err
}

// recipeSpecFor resolves the structured plan of a session's coordinator recipe.
// ref is the versioned recipe ref to record; spec is nil for a prose-only or
// invalid recipe (the trajectory still records which recipe ran).
func (r *Runtime) recipeSpecFor(sess db.Session) (spec *skills.RecipeSpec, ref string) {
	wf := strings.TrimSpace(sess.CoordinatorWorkflow)
	if wf == "" {
		return nil, ""
	}
	sk, err := skills.ResolveRecipe(r.skills, wf)
	if err != nil || sk == nil {
		return nil, wf
	}
	return sk.Recipe, skills.RecipeRef(sk.Slug, sk.Version)
}

// AdoptTrajectoryRecipe is called when a recipe is selected on an existing
// coordinator session (session-role API): it ensures the trajectory and, when
// the graph has no declared phases yet, adds the recipe's plan to it.
func (r *Runtime) AdoptTrajectoryRecipe(ctx context.Context, sessionID string) error {
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !sess.IsCoordinator() || sess.RootSession() != sess.ID {
		return nil
	}
	t, err := r.EnsureTrajectory(ctx, sess.ID)
	if err != nil {
		return err
	}
	spec, ref := r.recipeSpecFor(sess)
	if spec == nil || len(trajectory.Phases(&t)) > 0 {
		return nil
	}
	_, err = r.db.UpdateTrajectory(ctx, t.ID, 0, func(t *db.Trajectory) error {
		t.TemplateRef = ref
		trajectory.ApplyRecipe(t, spec)
		trajectory.DeriveStatus(t)
		return nil
	})
	return err
}

// updateTrajectoryByRoot applies fn to the root's trajectory when one exists.
// Returns false when there is none; errors are logged, not returned — observers
// must never fail their caller over the projection.
func (r *Runtime) updateTrajectoryByRoot(ctx context.Context, root string, fn func(*db.Trajectory) error) bool {
	id := r.trajectoryIDForRoot(ctx, root)
	if id == "" {
		return false
	}
	if _, err := r.db.UpdateTrajectory(ctx, id, 0, fn); err != nil {
		r.logger.Warn("trajectory update failed", "trajectory", id, "root", root, "error", err)
		return false
	}
	return true
}

// --- coordination observer ---
//
// The hooks run on the coordination goroutine; the writes are queued on the
// ordered drainer (trajectory_queue.go) so spawn_worker's latency is untouched.

func (b trajectoryBinder) OnSpawn(ev SpawnEvent) {
	b.r.enqueueTrajectoryWork(func() { b.r.bindSpawn(ev) })
}

func (b trajectoryBinder) OnReport(ev ReportEvent) {
	b.r.enqueueTrajectoryWork(func() { b.r.bindReport(ev) })
}

func (trajectoryBinder) OnDrain(DrainEvent) {}

func (b trajectoryBinder) OnStall(ev StallEvent) {
	b.r.enqueueTrajectoryWork(func() { b.r.bindStall(ev) })
}

func (r *Runtime) bindSpawn(ev SpawnEvent) {
	ctx := context.Background()
	if _, err := r.EnsureTrajectory(ctx, ev.RootID); err != nil {
		r.logger.Warn("trajectory ensure on spawn failed", "root", ev.RootID, "error", err)
		return
	}
	at := ev.At * 1000
	r.updateTrajectoryByRoot(ctx, ev.RootID, func(t *db.Trajectory) error {
		trajectory.AutoStartPhase(t, at)
		coordNode := trajectory.SessionNodeID(ev.CoordinatorID)
		if trajectory.NodePtr(t, coordNode) == nil {
			// A sub-coordinator the binder never saw spawn (trajectory created
			// mid-tree): give it a vertex so the edge below is valid.
			trajectory.AddNode(t, db.TrajectoryNode{
				ID: coordNode, Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved,
				RefKind: "session", RefID: ev.CoordinatorID, PhaseID: trajectory.ActivePhase(t),
				Lane: trajectory.NextLane(t), State: db.TrajStateActive, StartMs: at,
			})
		}
		label := ev.AgentName
		if ev.SubCoordinator {
			label += " (sub-coordinator)"
		}
		state := db.TrajStateActive
		reason := ""
		if ev.Queued {
			state = db.TrajStatePending
			reason = "queued for a free worker slot"
		}
		trajectory.AddNode(t, db.TrajectoryNode{
			ID: trajectory.SessionNodeID(ev.WorkerID), Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved,
			Label: label, RefKind: "session", RefID: ev.WorkerID, PhaseID: trajectory.ActivePhase(t),
			Lane: trajectory.NextLane(t), State: state, Reason: reason, StartMs: at,
		})
		trajectory.AddEdge(t, coordNode, trajectory.SessionNodeID(ev.WorkerID), db.TrajEdgeSpawned, db.TrajOriginObserved)
		trajectory.DeriveStatus(t)
		return nil
	})
}

func (r *Runtime) bindReport(ev ReportEvent) {
	ctx := context.Background()
	coord, err := r.db.GetSession(ctx, ev.CoordinatorID)
	if err != nil {
		return
	}
	at := ev.At * 1000
	r.updateTrajectoryByRoot(ctx, coord.RootSession(), func(t *db.Trajectory) error {
		w := trajectory.NodePtr(t, trajectory.SessionNodeID(ev.WorkerID))
		if w == nil {
			return nil
		}
		switch ev.Status {
		case "", "completed":
			w.State = db.TrajStateDone
			w.Reason = ""
		default:
			w.State = db.TrajStateFailed
			w.Reason = ev.Status
		}
		if w.StartMs == 0 {
			w.StartMs = at
		}
		w.EndMs = at
		trajectory.AddEdge(t, w.ID, trajectory.SessionNodeID(ev.CoordinatorID), db.TrajEdgeReported, db.TrajOriginObserved)
		trajectory.DeriveStatus(t)
		return nil
	})
}

func (r *Runtime) bindStall(ev StallEvent) {
	ctx := context.Background()
	coord, err := r.db.GetSession(ctx, ev.CoordinatorID)
	if err != nil {
		return
	}
	r.updateTrajectoryByRoot(ctx, coord.RootSession(), func(t *db.Trajectory) error {
		if n := trajectory.NodePtr(t, trajectory.SessionNodeID(ev.CoordinatorID)); n != nil {
			n.Reason = "stall halt: " + ev.Reason
		}
		return nil
	})
}

// --- session lifecycle ---

// onSessionChangeForTrajectory is the binder's half of the session hook
// (called from Runtime.OnSessionChange): seeds a recipe-bearing root at
// creation, records a session forked off a trajectory, and closes the graph
// when its root is archived.
func (r *Runtime) onSessionChangeForTrajectory(ev db.SessionChangeEvent) {
	ctx := context.Background()
	s := ev.Session
	switch ev.Op {
	case db.SessionOpCreate:
		if s.RootSession() != s.ID {
			return // a tree member: OnSpawn owns it
		}
		// A root that another trajectory's member started (an automation fire, a
		// handoff) is first and foremost a NODE of that trajectory. It is not seeded
		// with a trajectory of its own at create even when its agent carries a
		// recipe — otherwise every rota-sonu watcher on a coordinator agent listed a
		// second "planlandı" rota that never went anywhere (observed 2026-09-03). If
		// it really delegates later, bindSpawn's EnsureTrajectory seeds it then.
		if r.bindForkedSession(ctx, s) {
			return
		}
		if s.IsCoordinator() && strings.TrimSpace(s.CoordinatorWorkflow) != "" {
			if _, err := r.EnsureTrajectory(ctx, s.ID); err != nil {
				r.logger.Warn("trajectory seed at create failed", "session", s.ID, "error", err)
			}
		}
	case db.SessionOpState:
		if s.State != "archived" || s.RootSession() != s.ID {
			return
		}
		at := nowMs()
		r.updateTrajectoryByRoot(ctx, s.ID, func(t *db.Trajectory) error {
			if n := trajectory.NodePtr(t, trajectory.SessionNodeID(s.ID)); n != nil && n.State == db.TrajStateActive {
				n.State = db.TrajStateDone
				n.EndMs = at
			}
			if t.IsTerminal() {
				return nil
			}
			trajectory.DeriveStatus(t)
			switch t.Status {
			case db.TrajStatusDone, db.TrajStatusFailed:
			default:
				if len(trajectory.Phases(t)) > 0 {
					t.Status = db.TrajStatusAbandoned
				} else {
					t.Status = db.TrajStatusDone
				}
			}
			return nil
		})
	}
}

// bindForkedSession records a new ROOT session that another trajectory's
// member started (an automation fire, a handoff, a spawn_session) as a node of
// that trajectory with a fired / forked_from edge from the trigger. Flow
// transcript sessions are skipped: the flow RUN node stands for them. Reports
// whether the session was bound into a trigger's trajectory.
func (r *Runtime) bindForkedSession(ctx context.Context, s db.Session) bool {
	o := s.Lineage()
	if o.TriggerSessionID == "" || o.TriggerSessionID == s.ID || o.Kind == db.OriginFlow {
		return false
	}
	trigger, err := r.db.GetSession(ctx, o.TriggerSessionID)
	if err != nil {
		return false
	}
	edge := db.TrajEdgeForkedFrom
	if o.Kind == db.OriginAutomation {
		edge = db.TrajEdgeFired
	}
	at := s.CreatedAt * 1000
	bound := false
	r.updateTrajectoryByRoot(ctx, trigger.RootSession(), func(t *db.Trajectory) error {
		from := trajectory.SessionNodeID(trigger.ID)
		if trajectory.NodePtr(t, from) == nil {
			return nil
		}
		bound = true
		label := s.Title
		if o.Kind == db.OriginAutomation && o.EntityID != "" {
			label = o.EntityID + " → " + s.Title
		}
		trajectory.AddNode(t, db.TrajectoryNode{
			ID: trajectory.SessionNodeID(s.ID), Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved,
			Label: label, RefKind: "session", RefID: s.ID, PhaseID: trajectory.ActivePhase(t),
			Lane: trajectory.NextLane(t), State: db.TrajStateActive, StartMs: at,
		})
		trajectory.AddEdge(t, from, trajectory.SessionNodeID(s.ID), edge, db.TrajOriginObserved)
		return nil
	})
	return bound
}

// --- flow runs ---

// bindFlowRunToTrajectory records a flow run launched from inside a
// trajectory (its transcript session's trigger is a tree member) and keeps the
// node's state in step with the run's status. Called from emitFlowRunEvent,
// the single flow-run status emit point.
func (r *Runtime) bindFlowRunToTrajectory(run db.FlowRun) {
	if run.SessionID == "" {
		return
	}
	ctx := context.Background()
	transcript, err := r.db.GetSession(ctx, run.SessionID)
	if err != nil {
		return
	}
	trigger := transcript.Lineage().TriggerSessionID
	if trigger == "" {
		return
	}
	trig, err := r.db.GetSession(ctx, trigger)
	if err != nil {
		return
	}
	state := db.TrajStateActive
	switch run.Status {
	case db.FlowSuccess:
		state = db.TrajStateDone
	case db.FlowFailure:
		state = db.TrajStateFailed
	}
	r.updateTrajectoryByRoot(ctx, trig.RootSession(), func(t *db.Trajectory) error {
		from := trajectory.SessionNodeID(trig.ID)
		if trajectory.NodePtr(t, from) == nil {
			return nil
		}
		id := trajectory.FlowRunNodeID(run.ID)
		n := trajectory.NodePtr(t, id)
		if n == nil {
			trajectory.AddNode(t, db.TrajectoryNode{
				ID: id, Kind: db.TrajNodeFlowRun, Origin: db.TrajOriginObserved,
				Label: transcript.Title, RefKind: "flowrun", RefID: run.ID, PhaseID: trajectory.ActivePhase(t),
				Lane: trajectory.NextLane(t), State: state, StartMs: run.CreatedAt * 1000,
			})
			trajectory.AddEdge(t, from, id, db.TrajEdgeSpawned, db.TrajOriginObserved)
			n = trajectory.NodePtr(t, id)
		}
		n.State = state
		n.Reason = run.Error
		if state != db.TrajStateActive {
			n.EndMs = run.UpdatedAt * 1000
		}
		trajectory.DeriveStatus(t)
		return nil
	})
}

// --- durable asks (human gates) ---

// bindAskToTrajectory opens a human gate for a durable ask parked on a tree
// member; the trajectory goes waiting until the gate closes.
func (r *Runtime) bindAskToTrajectory(ask db.SessionAsk) {
	ctx := context.Background()
	sess, err := r.db.GetSession(ctx, ask.SessionID)
	if err != nil {
		return
	}
	at := ask.CreatedAt * 1000
	if at == 0 {
		at = nowMs()
	}
	// A phase gate (F5) hangs under its phase and names it; an agent's ask_user
	// hangs where the asking session is.
	_, gatePhase, isGate := GateAskRef(ask)
	r.updateTrajectoryByRoot(ctx, sess.RootSession(), func(t *db.Trajectory) error {
		from := trajectory.SessionNodeID(sess.ID)
		asker := trajectory.NodePtr(t, from)
		if asker == nil {
			return nil
		}
		label, phaseID, gateValue := ask.Kind, asker.PhaseID, ask.Kind
		if isGate {
			label, gateValue = "kapı: "+gatePhase, gatePhase
			if trajectory.NodePtr(t, trajectory.PhaseNodeID(gatePhase)) != nil {
				phaseID = trajectory.PhaseNodeID(gatePhase)
			}
		}
		trajectory.AddNode(t, db.TrajectoryNode{
			ID: trajectory.GateNodeID(ask.ID), Kind: db.TrajNodeGate, Origin: db.TrajOriginObserved,
			Label: label, RefKind: "ask", RefID: ask.ID, PhaseID: phaseID,
			Lane: asker.Lane, State: db.TrajStateActive, Gate: &db.TrajectoryGate{Kind: "human", Value: gateValue},
			StartMs: at,
		})
		trajectory.AddEdge(t, from, trajectory.GateNodeID(ask.ID), db.TrajEdgeBlockedBy, db.TrajOriginObserved)
		trajectory.DeriveStatus(t)
		return nil
	})
}

// releaseAskInTrajectory closes the gate of an ask that was answered
// (resolved → done) or dropped (timeout / cancelled → skipped with the reason).
func (r *Runtime) releaseAskInTrajectory(ask db.SessionAsk, status string) {
	ctx := context.Background()
	sess, err := r.db.GetSession(ctx, ask.SessionID)
	if err != nil {
		return
	}
	at := nowMs()
	r.updateTrajectoryByRoot(ctx, sess.RootSession(), func(t *db.Trajectory) error {
		g := trajectory.NodePtr(t, trajectory.GateNodeID(ask.ID))
		if g == nil || g.State != db.TrajStateActive {
			return nil
		}
		if status == db.SessionAskResolved {
			g.State = db.TrajStateDone
		} else {
			g.State = db.TrajStateSkipped
			g.Reason = status
		}
		g.EndMs = at
		if t.Status == db.TrajStatusWaiting {
			t.Status = db.TrajStatusRunning
		}
		trajectory.DeriveStatus(t)
		return nil
	})
}
