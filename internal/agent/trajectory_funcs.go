package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// Trajectory tool runner + situation block (Rota F1a).
//
// trajectoryFuncsFor binds the trajectory tool to a coordinator session. Get is
// available on every coordinator; the declaring actions (plan / phase / finish)
// only on the ROOT of the tree, because the graph — and its phases — belong to
// the root session. Writes go through UpdateTrajectory with the store's lock;
// the agent never sees or echoes revisions, its edits are last-writer-wins
// against the observers' appends (which touch disjoint nodes).

func (r *Runtime) trajectoryFuncsFor(sess db.Session) *tools.TrajectoryFuncs {
	if !sess.IsCoordinator() {
		return nil
	}
	root := sess.RootSession()
	f := &tools.TrajectoryFuncs{
		Get: func(ctx context.Context) (string, error) {
			t, err := r.EnsureTrajectory(ctx, root)
			if err != nil {
				return "", err
			}
			return trajRender(&t), nil
		},
	}
	if root != sess.ID {
		return f
	}
	f.Plan = func(ctx context.Context, phases []tools.TrajectoryPhaseInput) (string, error) {
		t, err := r.EnsureTrajectory(ctx, root)
		if err != nil {
			return "", err
		}
		plan := make([]TrajectoryPlanPhase, 0, len(phases))
		for _, p := range phases {
			pp := TrajectoryPlanPhase{ID: p.ID, Label: p.Label, Profile: p.Profile, Optional: p.Optional}
			if p.Gate != nil {
				pp.GateKind, pp.GateVal = strings.TrimSpace(p.Gate.Kind), p.Gate.Value
			}
			plan = append(plan, pp)
		}
		updated, err := r.PlanTrajectory(ctx, t.ID, plan, 0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Plan recorded on %s (revision %d).\nPhases: %s\nCall trajectory{action:\"phase\", id, state:\"active\"} when you start a phase.",
			updated.ID, updated.Revision, trajPhaseLine(&updated)), nil
	}
	f.Phase = func(ctx context.Context, id, state, reason string, force bool) (string, error) {
		t, err := r.EnsureTrajectory(ctx, root)
		if err != nil {
			return "", err
		}
		updated, err := r.SetTrajectoryPhase(ctx, t.ID, id, state, reason, 0, force)
		if err != nil {
			if errors.Is(err, ErrGatePending) {
				return fmt.Sprintf("Phase %s has a HUMAN gate: a question card was opened on session %s. The phase stays active until it is answered; end your turn.", strings.TrimSpace(id), t.RootSessionID), nil
			}
			return "", err
		}
		return fmt.Sprintf("Phase %s is now %s (trajectory %s, revision %d).\nPhases: %s",
			strings.TrimSpace(id), state, updated.ID, updated.Revision, trajPhaseLine(&updated)), nil
	}
	f.Finish = func(ctx context.Context, status, reason string) (string, error) {
		var final string
		switch status {
		case "done":
			final = db.TrajStatusDone
		case "failed":
			final = db.TrajStatusFailed
		default:
			return "", fmt.Errorf("status must be done or failed, got %q", status)
		}
		t, err := r.EnsureTrajectory(ctx, root)
		if err != nil {
			return "", err
		}
		updated, err := r.FinishTrajectory(ctx, t.ID, final, reason, 0)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Trajectory %s finished as %s (revision %d).", updated.ID, final, updated.Revision), nil
	}
	return f
}

// coordinatorTrajectoryBlock is the situation-block section that pushes the
// trajectory to a coordinator every turn: id, status, the phase line and the
// one instruction that keeps the graph honest (say when you move on). Empty
// when the tree has no trajectory yet — a fresh coordinator that has not
// spawned anything owes nothing.
func (r *Runtime) coordinatorTrajectoryBlock(ctx context.Context, coordSessionID string) string {
	sess, err := r.db.GetSession(ctx, coordSessionID)
	if err != nil {
		return ""
	}
	root := sess.RootSession()
	id := r.trajectoryIDForRoot(ctx, root)
	if id == "" {
		return ""
	}
	t, err := r.db.GetTrajectory(ctx, id)
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<trajectory>\n")
	fmt.Fprintf(&b, "Rota %s — status %s, revision %d", t.ID, t.Status, t.Revision)
	if t.TemplateRef != "" {
		fmt.Fprintf(&b, ", recipe %s", t.TemplateRef)
	}
	b.WriteString("\n")
	if line := trajPhaseLine(&t); line != "" {
		b.WriteString("Phases: " + line + "\n")
		active := trajActivePhase(&t)
		var running, done, failed int
		for _, n := range t.Nodes {
			if n.Kind != db.TrajNodeSession || n.Lane == 0 || n.PhaseID != active {
				continue
			}
			switch n.State {
			case db.TrajStateActive, db.TrajStatePending:
				running++
			case db.TrajStateDone:
				done++
			case db.TrajStateFailed:
				failed++
			}
		}
		if active != "" {
			fmt.Fprintf(&b, "Active phase %s: %d worker(s) running, %d done, %d failed.\n",
				strings.TrimPrefix(active, trajPhasePrefix), running, done, failed)
		}
		if root == sess.ID {
			b.WriteString("When you move to the next phase call trajectory{action:\"phase\", id, state:\"active\"}; " +
				"when the whole job is over call trajectory{action:\"finish\"}.\n")
		}
	} else if root == sess.ID {
		b.WriteString("No phases declared. If the work has distinct steps, announce them once with " +
			"trajectory{action:\"plan\", phases:[{id, profile}]}; otherwise ignore this block.\n")
	}
	if trajOpenGate(&t) {
		b.WriteString("A human gate is OPEN: the trajectory is waiting for an answer.\n")
	}
	b.WriteString("</trajectory>")
	return b.String()
}
