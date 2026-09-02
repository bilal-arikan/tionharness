package agent

import (
	"context"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Trajectory transitions (Rota F2).
//
// The trajectory hook delivers the graph AFTER every change, never the diff.
// The automation engine wants the diff: "phase X just finished", "the
// trajectory just ended". This differ remembers each live trajectory's phase
// states and status and turns consecutive snapshots into TrajectoryTransition
// values, handed to the transition hook (the automation engine, wired by the
// workspace manager) on the ordered off-path queue — never on the goroutine
// that wrote the graph.
//
// Memory starts empty at boot: the first snapshot of an existing trajectory is
// a baseline, so a phase that finished while the process was down is not
// re-announced (a deliberate miss; the ledger has no record either way).

// Transition kinds.
const (
	TrajTransitionPhase = "phase"
	TrajTransitionEnd   = "trajectory_end"
)

// TrajectoryTransition is one observed change of a trajectory: a declared
// phase entered / exited, or the trajectory reached a terminal status.
type TrajectoryTransition struct {
	Trajectory db.Trajectory
	Kind       string // TrajTransitionPhase | TrajTransitionEnd
	// Phase transitions: bare phase id (without "p:"), the new state and the
	// event (db.TrajEventEnter | db.TrajEventExit).
	PhaseID    string
	PhaseState string
	Event      string
	// End transitions: the terminal status reached.
	Status string
}

// RecipeSlug is the recipe the trajectory was seeded from, without version
// ("" for an agent-planned trajectory).
func (t TrajectoryTransition) RecipeSlug() string {
	ref := t.Trajectory.TemplateRef
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

type trajMemo struct {
	phases map[string]string // phase node id → state
	status string
}

type trajTransitionDiffer struct {
	mu   sync.Mutex
	memo map[string]trajMemo
}

// diff records ev and returns the transitions it implies.
func (d *trajTransitionDiffer) diff(ev db.TrajectoryChangeEvent) []TrajectoryTransition {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.memo == nil {
		d.memo = map[string]trajMemo{}
	}
	if ev.Op == db.TrajectoryOpDelete {
		delete(d.memo, ev.TrajectoryID)
		return nil
	}
	t := ev.Trajectory
	next := trajMemo{phases: map[string]string{}, status: t.Status}
	for _, n := range t.Nodes {
		if n.Kind == db.TrajNodePhase {
			next.phases[n.ID] = n.State
		}
	}
	prev, known := d.memo[ev.TrajectoryID]
	d.memo[ev.TrajectoryID] = next
	if !known || ev.Op == db.TrajectoryOpCreate {
		return nil // baseline
	}
	var out []TrajectoryTransition
	for _, n := range t.Nodes {
		if n.Kind != db.TrajNodePhase {
			continue
		}
		was := prev.phases[n.ID]
		if was == n.State {
			continue
		}
		id := strings.TrimPrefix(n.ID, "p:")
		switch {
		case n.State == db.TrajStateActive:
			out = append(out, TrajectoryTransition{Trajectory: t, Kind: TrajTransitionPhase, PhaseID: id, PhaseState: n.State, Event: db.TrajEventEnter})
		case trajPhaseClosed(n.State) && !trajPhaseClosed(was):
			out = append(out, TrajectoryTransition{Trajectory: t, Kind: TrajTransitionPhase, PhaseID: id, PhaseState: n.State, Event: db.TrajEventExit})
		}
	}
	if t.IsTerminal() && !(db.Trajectory{Status: prev.status}).IsTerminal() {
		out = append(out, TrajectoryTransition{Trajectory: t, Kind: TrajTransitionEnd, Status: t.Status})
	}
	return out
}

func trajPhaseClosed(state string) bool {
	return state == db.TrajStateDone || state == db.TrajStateSkipped || state == db.TrajStateFailed
}

// SetTrajectoryTransitionHook registers the subscriber (the automation
// engine). One hook; nil clears.
func (r *Runtime) SetTrajectoryTransitionHook(fn func(context.Context, TrajectoryTransition)) {
	r.trajTransitionMu.Lock()
	r.trajTransitionHook = fn
	r.trajTransitionMu.Unlock()
}

// observeTrajectoryTransitions is called from OnTrajectoryChange: it diffs the
// snapshot and queues one hook call per transition on the trajectory work
// queue (ordered, off the writer's goroutine).
func (r *Runtime) observeTrajectoryTransitions(ev db.TrajectoryChangeEvent) {
	transitions := r.trajTransitions.diff(ev)
	if len(transitions) == 0 {
		return
	}
	r.trajTransitionMu.RLock()
	fn := r.trajTransitionHook
	r.trajTransitionMu.RUnlock()
	if fn == nil {
		fn = func(context.Context, TrajectoryTransition) {}
	}
	for _, tr := range transitions {
		tr := tr
		if tr.Kind == TrajTransitionEnd {
			// The end-of-run summary (F3) lands before the trajectory_end rules
			// run, so their prompts and the curator see final numbers.
			id := tr.Trajectory.ID
			r.enqueueTrajectoryWork(func() { r.summarizeOnEnd(id) })
		}
		r.enqueueTrajectoryWork(func() { fn(context.Background(), tr) })
		if tr.Kind == TrajTransitionEnd {
			// The optimizer (F4) checks its threshold last; the LLM call itself
			// leaves the queue on its own goroutine.
			slug, trigger := tr.RecipeSlug(), optimizerTriggerRuns
			if tr.Status == db.TrajStatusFailed {
				trigger = optimizerTriggerFail
			}
			r.enqueueTrajectoryWork(func() { r.MaybeOptimizeRecipe(context.Background(), slug, trigger) })
		}
	}
}
