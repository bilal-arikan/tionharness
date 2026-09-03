package trajectory

import (
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Trajectory transitions (Rota F2).
//
// The trajectory hook delivers the graph AFTER every change, never the diff.
// The automation engine wants the diff: "phase X just finished", "the
// trajectory just ended". This differ remembers each live trajectory's phase
// states and status and turns consecutive snapshots into Transition
// values, handed to the transition hook (the automation engine, wired by the
// workspace manager) on the ordered off-path queue — never on the goroutine
// that wrote the graph.
//
// Memory starts empty at boot: the first snapshot of an existing trajectory is
// a baseline, so a phase that finished while the process was down is not
// re-announced (a deliberate miss; the ledger has no record either way).

// Transition kinds.
const (
	TransitionPhase = "phase"
	TransitionEnd   = "trajectory_end"
)

// Transition is one observed change of a trajectory: a declared
// phase entered / exited, or the trajectory reached a terminal status.
type Transition struct {
	Trajectory db.Trajectory
	Kind       string // TransitionPhase | TransitionEnd
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
func (t Transition) RecipeSlug() string {
	ref := t.Trajectory.TemplateRef
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

type memo struct {
	phases map[string]string // phase node id → state
	status string
}

type TransitionDiffer struct {
	mu   sync.Mutex
	memo map[string]memo
}

// Diff records ev and returns the transitions it implies.
func (d *TransitionDiffer) Diff(ev db.TrajectoryChangeEvent) []Transition {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.memo == nil {
		d.memo = map[string]memo{}
	}
	if ev.Op == db.TrajectoryOpDelete {
		delete(d.memo, ev.TrajectoryID)
		return nil
	}
	t := ev.Trajectory
	next := memo{phases: map[string]string{}, status: t.Status}
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
	var out []Transition
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
			out = append(out, Transition{Trajectory: t, Kind: TransitionPhase, PhaseID: id, PhaseState: n.State, Event: db.TrajEventEnter})
		case PhaseClosed(n.State) && !PhaseClosed(was):
			out = append(out, Transition{Trajectory: t, Kind: TransitionPhase, PhaseID: id, PhaseState: n.State, Event: db.TrajEventExit})
		}
	}
	if t.IsTerminal() && !(db.Trajectory{Status: prev.status}).IsTerminal() {
		out = append(out, Transition{Trajectory: t, Kind: TransitionEnd, Status: t.Status})
	}
	return out
}

func PhaseClosed(state string) bool {
	return state == db.TrajStateDone || state == db.TrajStateSkipped || state == db.TrajStateFailed
}
