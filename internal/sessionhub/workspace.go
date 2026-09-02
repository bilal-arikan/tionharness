package sessionhub

import (
	"encoding/json"
	"time"
)

// Workspace-scoped stream.
//
// Besides one ordered log per session, the hub keeps ONE ordered log per
// workspace for structured lifecycle events that are not about a single
// transcript: sessions appearing/finishing, flow runs, armed schedules,
// automation fires, trajectory revisions. It reuses the per-session machinery
// unchanged — seq, ring, epoch, replay, gap detection — under a reserved scope
// id, so the client contract (cursor + epoch + reset) is identical to the
// session stream and _Docs/58's reasoning carries over.
//
// Two deliberate differences from a session scope:
//
//   - A FRESH subscriber replays nothing. There is no "in-flight tail" to catch
//     up on: the client loads the current picture over REST and applies live
//     events from `head`. Every workspace publish therefore auto-commits, which
//     makes Replay(since<=0) empty while keeping reconnect gap-fill intact.
//   - The ring is larger (workspaceRingFactor × the session cap): one workspace
//     multiplexes every session's lifecycle, so a client that was away for a
//     minute should still gap-fill instead of resetting.

// workspaceScope is the reserved scope id for a workspace's own stream. It
// starts with NUL, which no session id can contain, so it can never alias a
// session scope under the same workspace.
const workspaceScope = "\x00workspace"

// workspaceRingFactor multiplies Hub.ringCap for the workspace scope.
const workspaceRingFactor = 4

// Workspace event kinds (mirrored by the frontend). The API derives them from
// the bus type by stripping events.WorkspaceStreamPrefix; they are listed here
// only so the two sides have a named contract.
const (
	KindWSSessionLifecycle = "session_lifecycle"
	KindWSTrajectory       = "trajectory"
	KindWSFlowRun          = "flow_run"
	KindWSScheduleArmed    = "schedule_armed"
	KindWSAutomationFire   = "automation_fire"
	KindWSSpawn            = "spawn"
	KindWSReport           = "report"
	KindWSLiveness         = "liveness"
	KindWSCoordination     = "coordination"
)

// PublishWorkspace appends one durable event to the workspace stream and fans
// it out to live subscribers. Event.SessionID stays empty — the subject lives in
// the payload. Returns the assigned seq; 0 on a nil hub or empty workspace.
func (h *Hub) PublishWorkspace(wsID, kind string, payload json.RawMessage) int64 {
	if h == nil || wsID == "" {
		return 0
	}
	ev := Event{Kind: kind, Payload: payload, Time: time.Now().Unix()}
	return h.publish(scopeKey(wsID, workspaceScope), ev, false, h.ringCap*workspaceRingFactor, true)
}

// SubscribeWorkspace registers a listener for a workspace's stream and returns
// its id, receive channel and the current head seq.
func (h *Hub) SubscribeWorkspace(wsID string) (int, <-chan Event, int64) {
	return h.Subscribe(wsID, workspaceScope)
}

// UnsubscribeWorkspace removes a workspace-stream listener.
func (h *Hub) UnsubscribeWorkspace(wsID string, id int) {
	h.Unsubscribe(wsID, workspaceScope, id)
}

// ReplayWorkspace returns the durable events a reconnecting client is missing
// (seq > since). A fresh subscribe (since <= 0) gets nothing — see the package
// note above. ok=false means the cursor fell out of the ring → reset.
func (h *Hub) ReplayWorkspace(wsID string, since int64) ([]Event, bool) {
	return h.Replay(wsID, workspaceScope, since)
}

// HeadWorkspace returns the workspace stream's latest durable seq (0 if none).
func (h *Hub) HeadWorkspace(wsID string) int64 {
	return h.Head(wsID, workspaceScope)
}

// DropWorkspace discards a workspace's stream state (on workspace delete).
func (h *Hub) DropWorkspace(wsID string) {
	h.Drop(wsID, workspaceScope)
}
