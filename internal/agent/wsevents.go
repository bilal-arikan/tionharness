package agent

import (
	"encoding/json"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// Workspace-stream events (_Docs/77 R3).
//
// Structured lifecycle facts the runtime already knows — a session appeared or
// finished, a flow run changed status, a schedule was armed, an automation
// fired, a trajectory gained a revision — are published on the process bus
// under events.WorkspaceStreamPrefix types with a JSON payload in Event.Data.
// The API layer bridges them onto the ordered, replayable per-workspace hub
// stream and keeps them OFF the fire-and-forget notification feed. This file is
// the single emit point so every payload shape lives in one place.

// emitWorkspaceEvent publishes one workspace-stream event. Marshal failures
// degrade to a null payload rather than dropping the fact.
func (r *Runtime) emitWorkspaceEvent(typ string, target map[string]string, data any) {
	if r == nil || r.bus == nil {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		raw = json.RawMessage("null")
	}
	r.publish(events.Event{Type: typ, Level: "info", Target: target, Data: raw})
}

// SessionLifecyclePayload is the Data shape of a ws:session_lifecycle event.
type SessionLifecyclePayload struct {
	SessionID     string           `json:"sessionId"`
	Op            string           `json:"op"` // db.SessionOp*
	Kind          string           `json:"kind"`
	AgentID       string           `json:"agentId,omitempty"`
	Title         string           `json:"title,omitempty"`
	State         string           `json:"state"`
	PrevState     string           `json:"prevState,omitempty"`
	RunState      string           `json:"runState,omitempty"`
	PrevRunState  string           `json:"prevRunState,omitempty"`
	RootSessionID string           `json:"rootSessionId"`
	Origin        db.SessionOrigin `json:"origin"`
	Coordinator   bool             `json:"coordinator,omitempty"`
	UpdatedAt     int64            `json:"updatedAt"`
}

// OnSessionChange is the db.SessionChangeFn the workspace manager wires at boot
// (db.SetSessionHook). It runs after the store released its locks and must stay
// cheap: it only shapes a payload and hands it to the non-blocking bus.
func (r *Runtime) OnSessionChange(ev db.SessionChangeEvent) {
	s := ev.Session
	r.emitWorkspaceEvent(events.TypeWSSessionLifecycle,
		map[string]string{"sessionId": ev.SessionID, "op": ev.Op},
		SessionLifecyclePayload{
			SessionID: ev.SessionID, Op: ev.Op, Kind: s.Kind, AgentID: s.AgentID, Title: s.Title,
			State: s.State, PrevState: ev.PrevState, RunState: s.RunState, PrevRunState: ev.PrevRunState,
			RootSessionID: s.RootSession(), Origin: s.Lineage(), Coordinator: s.IsCoordinator(),
			UpdatedAt: s.UpdatedAt,
		})
}

// TrajectoryPayload is the Data shape of a ws:trajectory event — the index row,
// not the graph: a client that cares re-reads the trajectory by id.
type TrajectoryPayload struct {
	TrajectoryID  string `json:"trajectoryId"`
	RootSessionID string `json:"rootSessionId"`
	Op            string `json:"op"` // db.TrajectoryOp*
	TemplateRef   string `json:"templateRef,omitempty"`
	Status        string `json:"status,omitempty"`
	Revision      uint64 `json:"revision"`
	NodeCount     int    `json:"nodeCount"`
	UpdatedAt     int64  `json:"updatedAt,omitempty"`
}

// OnTrajectoryChange is the db.TrajectoryChangeFn wired next to OnSessionChange.
func (r *Runtime) OnTrajectoryChange(ev db.TrajectoryChangeEvent) {
	t := ev.Trajectory
	r.emitWorkspaceEvent(events.TypeWSTrajectory,
		map[string]string{"trajectoryId": ev.TrajectoryID, "rootSessionId": ev.RootSessionID, "op": ev.Op},
		TrajectoryPayload{
			TrajectoryID: ev.TrajectoryID, RootSessionID: ev.RootSessionID, Op: ev.Op,
			TemplateRef: t.TemplateRef, Status: t.Status, Revision: t.Revision,
			NodeCount: len(t.Nodes), UpdatedAt: t.UpdatedAt,
		})
}

// FlowRunPayload is the Data shape of a ws:flow_run event.
type FlowRunPayload struct {
	RunID        string `json:"runId"`
	FlowID       string `json:"flowId"`
	ParentRunID  string `json:"parentRunId,omitempty"`
	ParentNodeID string `json:"parentNodeId,omitempty"`
	RootRunID    string `json:"rootRunId"`
	SessionID    string `json:"sessionId,omitempty"`
	Status       string `json:"status"` // db.FlowRunning | FlowWaiting | FlowSuccess | FlowFailure
	Error        string `json:"error,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

// emitFlowRunEvent publishes a flow run's current status (start, waiting,
// finish). Called right after the corresponding store write.
func (r *Runtime) emitFlowRunEvent(run db.FlowRun) {
	r.emitWorkspaceEvent(events.TypeWSFlowRun,
		map[string]string{"flowRunId": run.ID, "flowId": run.FlowID, "rootRunId": run.RootOf(), "status": run.Status},
		FlowRunPayload{
			RunID: run.ID, FlowID: run.FlowID, ParentRunID: run.ParentRunID, ParentNodeID: run.ParentNodeID,
			RootRunID: run.RootOf(), SessionID: run.SessionID, Status: run.Status, Error: run.Error,
			CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
		})
}

// ScheduleArmedPayload is the Data shape of a ws:schedule_armed event: the
// "future" edge of the workspace timeline — when this schedule (or one-shot
// wake) is next expected to fire.
type ScheduleArmedPayload struct {
	ScheduleID string `json:"scheduleId"`
	Name       string `json:"name,omitempty"`
	AgentID    string `json:"agentId,omitempty"`
	FlowID     string `json:"flowId,omitempty"`
	OneShot    bool   `json:"oneShot,omitempty"`
	SessionID  string `json:"sessionId,omitempty"` // wake target for a one-shot
	FireAt     int64  `json:"fireAt"`              // unix seconds
}

// emitScheduleArmed publishes a schedule's next fire time.
func (r *Runtime) emitScheduleArmed(sc db.Schedule, fireAt int64) {
	r.emitWorkspaceEvent(events.TypeWSScheduleArmed,
		map[string]string{"scheduleId": sc.ID},
		ScheduleArmedPayload{
			ScheduleID: sc.ID, Name: sc.Name, AgentID: sc.AgentID, FlowID: sc.FlowID,
			OneShot: sc.OneShot, SessionID: sc.SessionID, FireAt: fireAt,
		})
}

// AutomationFirePayload is the Data shape of a ws:automation_fire event.
// Outcome is "fired" today; the trigger registry (_Docs/77 R5) adds "skipped"
// with a Reason (cooldown, max_iterations, expired, disabled, autonomy_paused).
type AutomationFirePayload struct {
	AutomationID     string `json:"automationId"`
	Name             string `json:"name,omitempty"`
	TriggerKind      string `json:"triggerKind"`
	Outcome          string `json:"outcome"`
	Reason           string `json:"reason,omitempty"`
	SessionID        string `json:"sessionId,omitempty"` // produced session (fired)
	TriggerSessionID string `json:"triggerSessionId,omitempty"`
	IterationCount   int    `json:"iterationCount,omitempty"`
}

// automationTriggerKind resolves the legacy empty TriggerKind to its meaning (tag).
func automationTriggerKind(a db.Automation) string {
	if a.TriggerKind == "" {
		return db.TriggerTag
	}
	return a.TriggerKind
}

// emitAutomationFire publishes one automation fire outcome.
func (r *Runtime) emitAutomationFire(a db.Automation, outcome, reason, sessionID, triggerSessionID string) {
	r.emitWorkspaceEvent(events.TypeWSAutomationFire,
		map[string]string{"automationId": a.ID, "sessionId": sessionID, "outcome": outcome},
		AutomationFirePayload{
			AutomationID: a.ID, Name: a.Name, TriggerKind: automationTriggerKind(a), Outcome: outcome,
			Reason: reason, SessionID: sessionID, TriggerSessionID: triggerSessionID,
			IterationCount: a.IterationCount,
		})
}
