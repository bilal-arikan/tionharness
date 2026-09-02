package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Trajectory-triggered automations (Rota F2).
//
// Two sources fire on a trajectory transition:
//
//  1. explicit rules — enabled automations of kind phase / trajectory_end whose
//     filters (phase id, event, recipe, terminal status) match;
//  2. declared watchers — the ghost automation nodes the recipe seeded on the
//     graph (`a:<watcher>@<phase>` for a phase's watchers, `a:<watcher>` for
//     the trajectory-wide ones), resolved by automation id or name.
//
// Every attempt is written back onto the graph: the watcher node turns done
// with a `fired` edge to the session it produced, or skipped with the reason
// the guard gave (cooldown, disabled, not found …) — that is the "neden
// ateşlenmedi" the Rota screen shows on a ghost. The ledger and the workspace
// stream get the same record through the shared notifyFired / recordSkip /
// recordFailure paths, so the fires list on the automation card agrees.

// AutomationSkipNotFound: a recipe watcher names no existing automation.
const AutomationSkipNotFound = "not_found"

// OnTrajectoryTransition is the transition hook (Runtime.SetTrajectoryTransitionHook).
func (e *AutomationEngine) OnTrajectoryTransition(ctx context.Context, tr TrajectoryTransition) {
	autos, err := e.db.ListAutomations(ctx)
	if err != nil {
		e.logger.Warn("automation: list failed (trajectory)", "trajectory", tr.Trajectory.ID, "error", err)
		return
	}
	fired := map[string]string{} // automation id → session id ("" = attempted, no session)
	// 1) explicit rules.
	for _, a := range autos {
		if !a.Enabled || a.Archived || !trajectoryRuleMatches(a, tr) {
			continue
		}
		sid, _ := e.fireTrajectory(ctx, a, tr, "")
		fired[a.ID] = sid
	}
	// 2) declared watchers on the graph.
	for _, n := range tr.Trajectory.Nodes {
		if n.Kind != db.TrajNodeAutomation || n.State != db.TrajStateGhost {
			continue
		}
		switch tr.Kind {
		case TrajTransitionPhase:
			if tr.Event != db.TrajEventExit || n.PhaseID != "p:"+tr.PhaseID {
				continue
			}
		case TrajTransitionEnd:
			if n.PhaseID != "" {
				continue
			}
		default:
			continue
		}
		a, ok := findAutomationByRef(autos, n.RefID)
		if !ok {
			e.markTrajectoryAutomation(ctx, tr, n.ID, db.Automation{ID: n.RefID, Name: n.Label}, db.TrajStateSkipped, AutomationSkipNotFound, "")
			continue
		}
		if sid, done := fired[a.ID]; done {
			// Already fired as an explicit rule on this transition; bind the
			// declared node to that outcome instead of firing twice.
			state, reason := db.TrajStateDone, ""
			if sid == "" {
				state, reason = db.TrajStateSkipped, "see ledger"
			}
			e.markTrajectoryAutomation(ctx, tr, n.ID, a, state, reason, sid)
			continue
		}
		if !a.Enabled || a.Archived {
			reason := db.AutomationSkipDisabled
			if a.Archived {
				reason = db.AutomationSkipArchived
			}
			e.recordSkip(ctx, a, reason)
			e.markTrajectoryAutomation(ctx, tr, n.ID, a, db.TrajStateSkipped, reason, "")
			continue
		}
		sid, _ := e.fireTrajectory(ctx, a, tr, n.ID)
		fired[a.ID] = sid
	}
}

// trajectoryRuleMatches reports whether an explicit phase / trajectory_end
// rule's filters accept the transition.
func trajectoryRuleMatches(a db.Automation, tr TrajectoryTransition) bool {
	if a.TrajRecipe != "" && !strings.EqualFold(strings.TrimSpace(a.TrajRecipe), tr.RecipeSlug()) {
		return false
	}
	switch a.TriggerKind {
	case db.TriggerPhase:
		if tr.Kind != TrajTransitionPhase || a.EffectiveTrajEvent() != tr.Event {
			return false
		}
		return a.TrajPhase == "" || strings.EqualFold(strings.TrimSpace(a.TrajPhase), tr.PhaseID)
	case db.TriggerTrajectoryEnd:
		if tr.Kind != TrajTransitionEnd {
			return false
		}
		return a.TrajStatus == "" || a.TrajStatus == tr.Status
	}
	return false
}

// findAutomationByRef resolves a recipe watcher (automation id or name,
// case-insensitive) to a stored automation.
func findAutomationByRef(autos []db.Automation, ref string) (db.Automation, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return db.Automation{}, false
	}
	for _, a := range autos {
		if a.ID == ref {
			return a, true
		}
	}
	for _, a := range autos {
		if strings.EqualFold(strings.TrimSpace(a.Name), ref) {
			return a, true
		}
	}
	return db.Automation{}, false
}

// fireTrajectory runs the guards and, when they pass, the automation, then
// writes the outcome onto the graph node (nodeID, or a node created for this
// rule). Returns the produced session id and whether it fired.
func (e *AutomationEngine) fireTrajectory(ctx context.Context, a db.Automation, tr TrajectoryTransition, nodeID string) (string, bool) {
	if reason := e.guardReason(ctx, a); reason != "" {
		e.recordSkip(ctx, a, reason)
		e.markTrajectoryAutomation(ctx, tr, nodeID, a, db.TrajStateSkipped, reason, "")
		return "", false
	}
	prompt := renderAutomationPrompt(a.PromptTemplate, e.trajectoryVars(a, tr))
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		e.markTrajectoryAutomation(ctx, tr, nodeID, a, db.TrajStateFailed, db.AutomationSkipEmptyPrompt, "")
		return "", false
	}
	trigger := TriggerAutomationPhase
	if tr.Kind == TrajTransitionEnd {
		trigger = TriggerAutomationTrajectoryEnd
	}
	sid, driver, err := e.dispatchFire(ctx, a, prompt, trigger, SpawnOptions{
		Title:     "◈ " + automationLabel(a),
		CreatedBy: "automation:" + a.ID,
		// The root session tripped this fire: the produced session hangs off
		// it (Origin.TriggerSessionID → the binder's fired edge).
		ParentSessionID: tr.Trajectory.RootSessionID,
		Tags:            a.SpawnTags, // trajectory rules do not self-loop; nil = no tag
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		e.markTrajectoryAutomation(ctx, tr, nodeID, a, db.TrajStateFailed, err.Error(), "")
		return "", false
	}
	suffix := " (rota·" + tr.Kind + ")"
	if tr.Kind == TrajTransitionPhase {
		suffix = " (rota·" + tr.PhaseID + "·" + tr.Event + ")"
	}
	if driver == "flow" {
		suffix = strings.TrimSuffix(suffix, ")") + "·akış)"
	}
	e.logger.Info("automation: fired (trajectory)",
		"automation", a.ID, "trajectory", tr.Trajectory.ID, "kind", tr.Kind, "phase", tr.PhaseID, "event", tr.Event,
		"status", tr.Status, "session", sid, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, sid, "◈", suffix, prompt)
	e.markTrajectoryAutomation(ctx, tr, nodeID, a, db.TrajStateDone, "", sid)
	return sid, true
}

// trajectoryVars assembles the placeholder values for a trajectory rule's
// prompt: the trajectory / root session / recipe, the phase and its state, the
// event, the terminal status, and the phase line. No {{result}}.
func (e *AutomationEngine) trajectoryVars(a db.Automation, tr TrajectoryTransition) map[string]string {
	v := commonVars(a)
	t := tr.Trajectory
	v["trajectoryId"] = t.ID
	v["rootSessionId"] = t.RootSessionID
	v["sessionId"] = t.RootSessionID
	v["recipe"] = tr.RecipeSlug()
	v["phase"] = tr.PhaseID
	v["phaseState"] = tr.PhaseState
	v["event"] = tr.Event
	v["status"] = t.Status
	if tr.Status != "" {
		v["status"] = tr.Status
	}
	v["phases"] = trajPhaseLine(&t)
	return v
}

// markTrajectoryAutomation records the outcome of one automation attempt on
// the trajectory: the declared ghost node (nodeID) or a node created for the
// explicit rule flips to done / skipped / failed with the reason, and a fired
// edge points at the produced session. Best-effort (logged by the updater).
func (e *AutomationEngine) markTrajectoryAutomation(ctx context.Context, tr TrajectoryTransition, nodeID string, a db.Automation, state, reason, sessionID string) {
	if e.rt == nil {
		return
	}
	at := nowMs()
	e.rt.updateTrajectoryByRoot(ctx, tr.Trajectory.RootSessionID, func(t *db.Trajectory) error {
		id := nodeID
		if id == "" {
			id = trajAutomationPfx + a.ID
			if tr.Kind == TrajTransitionPhase {
				id += "@" + tr.PhaseID
			}
		}
		n := trajNodePtr(t, id)
		if n == nil {
			phase := ""
			if tr.Kind == TrajTransitionPhase {
				phase = trajPhaseNodeID(tr.PhaseID)
				if trajNodePtr(t, phase) == nil {
					phase = ""
				}
			}
			trajAddNode(t, db.TrajectoryNode{
				ID: id, Kind: db.TrajNodeAutomation, Origin: db.TrajOriginObserved,
				RefKind: "automation", PhaseID: phase, Lane: trajNextLane(t), State: db.TrajStateGhost,
			})
			n = trajNodePtr(t, id)
		}
		if a.ID != "" {
			n.RefID = a.ID
		}
		if label := strings.TrimSpace(a.Name); label != "" {
			n.Label = label
		} else if n.Label == "" {
			n.Label = n.RefID
		}
		n.State = state
		n.Reason = reason
		if n.StartMs == 0 {
			n.StartMs = at
		}
		n.EndMs = at
		if sessionID != "" {
			sess := trajSessionNodeID(sessionID)
			if trajNodePtr(t, sess) == nil {
				trajAddNode(t, db.TrajectoryNode{
					ID: sess, Kind: db.TrajNodeSession, Origin: db.TrajOriginObserved,
					RefKind: "session", RefID: sessionID, PhaseID: n.PhaseID,
					Lane: trajNextLane(t), State: db.TrajStateActive, StartMs: at,
				})
			}
			trajAddEdge(t, id, sess, db.TrajEdgeFired, db.TrajOriginObserved)
		}
		return nil
	})
}
