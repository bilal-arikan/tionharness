package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// Phase gates (Rota F5, brief §5 / §11 F5). A declared phase may carry a gate:
// the exit condition that must hold before the phase counts as done.
//
//   - artifact: an artifact with that title (or kind) exists in the tree;
//   - verdict:  the root transcript's recent messages contain the verdict line
//     ("VERDICT: PASS");
//   - human:    a durable ask is parked on the root session; the phase stays
//     active (trajectory waiting) until someone answers;
//   - schema:   not verified in v1 (passes with a note).
//
// The check runs whenever a phase is moved to done — by the agent's trajectory
// tool or by the canvas — unless the caller forces it. A human gate is the
// same Durable Ask the agent's ask_user uses (card in the chat, restart-safe,
// answered from any window); its answer flows back through ResolvePhaseGate.

// gateAskPayload is what a human-gate ask carries in SessionAsk.Payload: the
// ordinary ask card fields plus the gate it stands for.
type gateAskPayload struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	Gate     *gateRef `json:"gate,omitempty"`
}

type gateRef struct {
	TrajectoryID string `json:"trajectoryId"`
	Phase        string `json:"phase"`
	Kind         string `json:"kind"`
	Value        string `json:"value,omitempty"`
}

// GateAskRef returns the gate an ask stands for, ok=false for an ordinary ask.
func GateAskRef(ask db.SessionAsk) (trajectoryID, phase string, ok bool) {
	var p gateAskPayload
	if ask.Payload == "" || json.Unmarshal([]byte(ask.Payload), &p) != nil || p.Gate == nil {
		return "", "", false
	}
	return p.Gate.TrajectoryID, p.Gate.Phase, true
}

// gateApproved reads a human answer as approval.
func gateApproved(answer string) bool {
	a := strings.ToLower(strings.TrimSpace(answer))
	for _, yes := range []string{"onay", "evet", "yes", "approve", "ok", "tamam", "geç", "pass"} {
		if strings.HasPrefix(a, yes) {
			return true
		}
	}
	return false
}

// ErrGateBlocked: the phase's gate did not pass (the message says why).
var ErrGateBlocked = errors.New("phase gate not satisfied")

// ErrGatePending: a human gate was opened; the phase stays active until the
// ask is answered.
var ErrGatePending = errors.New("phase gate waiting for a human")

// checkPhaseGate evaluates a phase's gate. pending=true means a human ask was
// opened (nothing else to decide now); pass=false with a reason means blocked.
func (r *Runtime) checkPhaseGate(ctx context.Context, t db.Trajectory, phase db.TrajectoryNode) (pass, pending bool, reason string) {
	g := phase.Gate
	if g == nil {
		return true, false, ""
	}
	value := strings.TrimSpace(g.Value)
	switch g.Kind {
	case "artifact":
		for _, sid := range r.trajectorySessionIDs(t) {
			arts, err := r.db.ListArtifacts(ctx, sid)
			if err != nil {
				continue
			}
			for _, a := range arts {
				if value == "" || strings.Contains(strings.ToLower(a.Title), strings.ToLower(value)) || strings.EqualFold(a.Kind, value) {
					return true, false, ""
				}
			}
		}
		return false, false, fmt.Sprintf("artifact %q not found in the tree", value)
	case "verdict":
		if value == "" {
			return true, false, ""
		}
		msgs, _, _ := r.db.ListMessagesTail(ctx, t.RootSessionID, 60)
		for i := len(msgs) - 1; i >= 0; i-- {
			if strings.Contains(strings.ToLower(msgs[i].Text), strings.ToLower(value)) {
				return true, false, ""
			}
		}
		return false, false, fmt.Sprintf("verdict %q not found in the root transcript", value)
	case "human":
		if err := r.openPhaseGateAsk(ctx, t, phase); err != nil {
			return false, false, "could not open the human gate: " + err.Error()
		}
		return false, true, ""
	default:
		return true, false, "schema gates are not verified (v1)"
	}
}

// trajectorySessionIDs lists the sessions bound on a trajectory (root first).
func (r *Runtime) trajectorySessionIDs(t db.Trajectory) []string {
	out := []string{t.RootSessionID}
	for _, n := range t.Nodes {
		if n.Kind == db.TrajNodeSession && n.RefID != "" && n.RefID != t.RootSessionID {
			out = append(out, n.RefID)
		}
	}
	return out
}

// openPhaseGateAsk parks a human gate on the root session: one durable ask
// that renders as an ordinary question card. The binder hangs it under the
// phase; the workspace stream tells the API to open the card live.
func (r *Runtime) openPhaseGateAsk(ctx context.Context, t db.Trajectory, phase db.TrajectoryNode) error {
	phaseID := strings.TrimPrefix(phase.ID, "p:")
	// One open gate per phase: a repeat request re-uses the parked ask.
	if asks, err := r.db.ListWaitingSessionAsks(ctx); err == nil {
		for _, a := range asks {
			if tid, p, ok := GateAskRef(a); ok && tid == t.ID && p == phaseID {
				return nil
			}
		}
	}
	label := phase.Label
	if label == "" {
		label = phaseID
	}
	question := fmt.Sprintf("Rota %s · %q fazı bitti sayılsın mı?", t.ID, label)
	if v := strings.TrimSpace(phase.Gate.Value); v != "" {
		question += " Kapı: " + v
	}
	payload, _ := json.Marshal(gateAskPayload{
		Question: question,
		Options:  []string{"Onayla", "Reddet"},
		Gate:     &gateRef{TrajectoryID: t.ID, Phase: phaseID, Kind: phase.Gate.Kind, Value: phase.Gate.Value},
	})
	ask, err := r.db.CreateSessionAsk(ctx, db.SessionAsk{
		SessionID: t.RootSessionID, Kind: "ask", CallID: "gate:" + t.ID + ":" + phaseID, Payload: string(payload),
	})
	if err != nil {
		return err
	}
	r.bindAskToTrajectory(ask)
	r.emitWorkspaceEvent(events.TypeWSAsk,
		map[string]string{"sessionId": t.RootSessionID, "askId": ask.ID, "op": "open", "trajectoryId": t.ID, "phase": phaseID},
		map[string]string{"askId": ask.ID, "sessionId": t.RootSessionID, "trajectoryId": t.ID, "phase": phaseID})
	r.logger.Info("trajectory: human gate opened", "trajectory", t.ID, "phase", phaseID, "ask", ask.ID)
	return nil
}

// ResolvePhaseGate applies a human gate's answer: approval closes the phase as
// done, rejection keeps it active with the answer as the reason. The ask is
// released on the graph either way. Called by the API's answer route.
func (r *Runtime) ResolvePhaseGate(ctx context.Context, ask db.SessionAsk, answer string) error {
	tid, phaseID, ok := GateAskRef(ask)
	if !ok {
		return errors.New("not a gate ask")
	}
	approved := gateApproved(answer)
	r.ReleaseAsk(ask, db.SessionAskResolved)
	_, err := r.db.UpdateTrajectory(ctx, tid, 0, func(t *db.Trajectory) error {
		n := trajNodePtr(t, trajPhaseNodeID(phaseID))
		if n == nil {
			return fmt.Errorf("phase %q not on trajectory %s", phaseID, tid)
		}
		if approved {
			n.State = db.TrajStateDone
			n.Reason = "kapı onaylandı"
			n.EndMs = nowMs()
		} else {
			n.State = db.TrajStateActive
			n.Reason = "kapı reddedildi: " + strings.TrimSpace(answer)
		}
		trajDeriveStatus(t)
		return nil
	})
	if err != nil {
		return err
	}
	note := fmt.Sprintf("<gate trajectory=%q phase=%q approved=%t>%s</gate>", tid, phaseID, approved, strings.TrimSpace(answer))
	if _, nerr := r.recordInjectedUserNote(ctx, ask.SessionID, "gate", note); nerr != nil {
		r.logger.Warn("trajectory: gate note failed", "ask", ask.ID, "error", nerr)
	}
	r.logger.Info("trajectory: human gate resolved", "trajectory", tid, "phase", phaseID, "approved", approved)
	return nil
}

// --- graph edits shared by the agent tool and the canvas API ---

// SetTrajectoryPhase moves a phase and runs its gate when the move is to done.
// expectedRev 0 skips the CAS (the agent tool); the canvas echoes the revision
// it rendered. force skips the gate.
func (r *Runtime) SetTrajectoryPhase(ctx context.Context, trajectoryID, phaseID, state, reason string, expectedRev uint64, force bool) (db.Trajectory, error) {
	t, err := r.db.GetTrajectory(ctx, trajectoryID)
	if err != nil {
		return db.Trajectory{}, err
	}
	if state == db.TrajStateDone && !force {
		if p := trajNodePtr(&t, trajPhaseNodeID(strings.TrimSpace(phaseID))); p != nil && p.Gate != nil && p.State != db.TrajStateDone {
			pass, pending, why := r.checkPhaseGate(ctx, t, *p)
			if pending {
				return t, fmt.Errorf("%w: phase %q — answer the card on session %s", ErrGatePending, phaseID, t.RootSessionID)
			}
			if !pass {
				return t, fmt.Errorf("%w: phase %q — %s (pass force to override)", ErrGateBlocked, phaseID, why)
			}
			if why != "" {
				reason = strings.TrimSpace(reason + " " + why)
			}
		}
	}
	return r.db.UpdateTrajectory(ctx, trajectoryID, expectedRev, func(t *db.Trajectory) error {
		if err := trajSetPhaseState(t, phaseID, state, reason, nowMs()); err != nil {
			return err
		}
		trajDeriveStatus(t)
		return nil
	})
}

// PlanTrajectory replaces the declared phase list (see trajApplyPlan).
func (r *Runtime) PlanTrajectory(ctx context.Context, trajectoryID string, plan []TrajectoryPlanPhase, expectedRev uint64) (db.Trajectory, error) {
	return r.db.UpdateTrajectory(ctx, trajectoryID, expectedRev, func(t *db.Trajectory) error {
		if err := trajApplyPlan(t, plan); err != nil {
			return err
		}
		trajDeriveStatus(t)
		return nil
	})
}

// FinishTrajectory closes the trajectory as done or failed.
func (r *Runtime) FinishTrajectory(ctx context.Context, trajectoryID, status, reason string, expectedRev uint64) (db.Trajectory, error) {
	var final string
	switch status {
	case db.TrajStatusDone:
		final = db.TrajStatusDone
	case db.TrajStatusFailed:
		final = db.TrajStatusFailed
	default:
		return db.Trajectory{}, fmt.Errorf("status must be done or failed, got %q", status)
	}
	at := nowMs()
	return r.db.UpdateTrajectory(ctx, trajectoryID, expectedRev, func(t *db.Trajectory) error {
		for i := range t.Nodes {
			n := &t.Nodes[i]
			if n.Kind == db.TrajNodePhase && n.State == db.TrajStateActive {
				n.State = db.TrajStateDone
				if final == db.TrajStatusFailed {
					n.State = db.TrajStateFailed
					n.Reason = strings.TrimSpace(reason)
				}
				n.EndMs = at
			}
		}
		if n := trajNodePtr(t, trajSessionNodeID(t.RootSessionID)); n != nil {
			n.EndMs = at
			n.State = db.TrajStateDone
			if final == db.TrajStatusFailed {
				n.State = db.TrajStateFailed
			}
			n.Reason = strings.TrimSpace(reason)
		}
		t.Status = final
		return nil
	})
}
