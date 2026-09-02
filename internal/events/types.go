package events

// Canonical event-type registry. Every Emit/Publish call names its event with one
// of these constants instead of a bare string literal, so the set of event types
// lives in ONE place (previously the type strings were scattered across ~20 files
// with no compile-time link, and the frontend settings screen silently drifted out
// of sync — e.g. it listed a "task" type that is never emitted).
//
// Two groups:
//   - Notify kinds (NotifyKinds): user-facing outcomes that may surface as a desktop
//     toast. The frontend derives its per-type mute settings from this exact set, so
//     "settings reflect every notification" holds by construction.
//   - Control/stream types: live UI signals (session_step/flow_node/log/progress) and
//     app-global state changes (session/settings/workspaces/navigate). These drive
//     refreshes/badges but are never toasts, so they are excluded from NotifyKinds.
const (
	// --- Notify kinds (toastable, appear in notification settings) ---

	// TypeChat is a completed assistant reply (chat/scheduler/spawn/coordination
	// turn end). The frontend raises the "reply ready" chime + toast for it.
	TypeChat = "chat"
	// TypeSchedule is a scheduled-prompt run outcome.
	TypeSchedule = "schedule"
	// TypeFlow is an orchestration flow-run outcome.
	TypeFlow = "flow"
	// TypeSpawned is a spawned sub-session run outcome.
	TypeSpawned = "spawned"
	// TypeWorker is a coordinator worker run outcome.
	TypeWorker = "worker"
	// TypeCoordination is a coordinator lifecycle signal (e.g. worker-cap warning).
	TypeCoordination = "coordination"
	// TypeAutomation is a tag/board automation fire outcome.
	TypeAutomation = "automation"
	// TypeArtifact is an artifact create/update (the "produced something" signal).
	TypeArtifact = "artifact"
	// TypeBoard is a kanban task create/move/update/delete. NOTE: there is no
	// distinct "task" event — task changes ARE board events. The settings screen
	// keys its "tasks" mute on TypeBoard for this reason.
	TypeBoard = "board"
	// TypeAgent is an agent-driven notify() tool call (general agent notification).
	TypeAgent = "agent"
	// TypeAnomaly is a warn-severity debug anomaly detected at turn end.
	TypeAnomaly = "anomaly"

	// --- Control / stream types (never toasts) ---

	// TypeSession is a per-session metadata mutation (title/goal/state/rewind/...).
	TypeSession = "session"
	// TypeSettings signals app settings changed elsewhere (re-apply client prefs).
	TypeSettings = "settings"
	// TypeWorkspaces signals the workspace set changed elsewhere (refresh switcher).
	TypeWorkspaces = "workspaces"
	// TypeNavigate drives open windows to a view/entity immediately (focus_view).
	TypeNavigate = "navigate"
	// TypeProgress is a persistent todo-list change.
	TypeProgress = "progress"
	// TypeSkills signals that the resolved skill catalog changed. It refreshes the
	// open Skills screen and lights its unread nav dot, but never raises a toast.
	TypeSkills = "skills"
	// TypeSessionStep carries one live turn-step frame (Event.Step).
	TypeSessionStep = "session_step"
	// TypeSessionUserMessage carries one runtime-injected user-role message
	// (Event.Msg) for live hub bridging — a worker task-notification, a
	// send_to_worker prompt, a coordination status/guard note. A control signal,
	// never a toast (the assistant reply that follows carries the TypeChat toast).
	TypeSessionUserMessage = "session_user_message"
	// TypeSessionTurnQueue signals that a session's TURN ADMISSION state changed —
	// a turn took the slot, finished it, or queued behind it (internal/turnqueue).
	// Deliberately payload-free: the API re-reads the snapshot when it bridges this,
	// so a burst of changes coalesces into one read. A control signal, never a toast.
	TypeSessionTurnQueue = "session_turn_queue"
	// TypeFlowNode carries one live flow-node lifecycle frame (Event.Node).
	TypeFlowNode = "flow_node"
	// TypeLog carries one captured log record for the live Logs tail (Event.Log).
	TypeLog = "log"

	// --- Workspace stream types (structured; never toasts; never on /api/events) ---
	//
	// Every type below carries WorkspaceStreamPrefix. The API layer does NOT put
	// them on the fire-and-forget /api/events feed; it bridges them onto the
	// ordered, replayable per-workspace hub stream (GET /api/workspace/stream) so a
	// live workspace view can keep an incremental picture and gap-fill after a
	// reconnect instead of re-fetching everything. Payload rides Event.Data.

	// TypeWSSessionLifecycle: a session was created / changed state or run-state /
	// gained its origin run id / was deleted (db.SetSessionHook).
	TypeWSSessionLifecycle = WorkspaceStreamPrefix + "session_lifecycle"
	// TypeWSTrajectory: a trajectory ("Rota") was created / updated / deleted.
	TypeWSTrajectory = WorkspaceStreamPrefix + "trajectory"
	// TypeWSFlowRun: a flow run started, suspended (waiting) or finished.
	TypeWSFlowRun = WorkspaceStreamPrefix + "flow_run"
	// TypeWSScheduleArmed: a schedule's next fire time (or a one-shot wake) was armed.
	TypeWSScheduleArmed = WorkspaceStreamPrefix + "schedule_armed"
	// TypeWSAutomationFire: an automation fired (or, later, was skipped with a reason).
	TypeWSAutomationFire = WorkspaceStreamPrefix + "automation_fire"
	// TypeWSSpawn / TypeWSReport: coordinator spawned a worker / a worker reported
	// back. Reserved for the coordination observer (_Docs/77 R7).
	TypeWSSpawn  = WorkspaceStreamPrefix + "spawn"
	TypeWSReport = WorkspaceStreamPrefix + "report"
	// TypeWSCoordination: a coordinator's drain turn started/ended or its
	// phantom-spawn guard halted it (target phase: turn_start | turn_end | stall_halt).
	TypeWSCoordination = WorkspaceStreamPrefix + "coordination"
	// TypeWSBoard: a kanban card was created / moved / updated / deleted. Mirrors
	// the "board" notify onto the ordered stream: that feed (/api/events) has no
	// cursor and no replay, so a consumer that drops the connection loses board
	// changes permanently and can only recover by re-polling GET /api/tasks.
	// Payload target carries taskId + op, same as the notify.
	TypeWSBoard = WorkspaceStreamPrefix + "board"
	// TypeWSLiveness: a session's turn-admission state changed (a turn took the
	// slot, released it, or queued behind it) — the live "running" edge of the
	// workspace picture. Payload-light: sessionId + busy + waiting depth; the
	// full picture is GET /api/workspace/liveness (_Docs/77 R2).
	TypeWSLiveness = WorkspaceStreamPrefix + "liveness"
	// TypeWSAsk: a durable ask was parked outside a turn (a phase gate, Rota F5)
	// — the API opens the card on the session hub when it sees op=open.
	TypeWSAsk = WorkspaceStreamPrefix + "ask"
)

// WorkspaceStreamPrefix marks the event types that ride the per-workspace hub
// stream instead of the global notification feed.
const WorkspaceStreamPrefix = "ws:"

// IsWorkspaceStream reports whether t is a workspace-stream type.
func IsWorkspaceStream(t string) bool {
	return len(t) > len(WorkspaceStreamPrefix) && t[:len(WorkspaceStreamPrefix)] == WorkspaceStreamPrefix
}

// WorkspaceStreamKind strips the prefix: the hub event kind the client sees
// ("session_lifecycle", "flow_run", …). Returns t unchanged when it has no prefix.
func WorkspaceStreamKind(t string) string {
	if IsWorkspaceStream(t) {
		return t[len(WorkspaceStreamPrefix):]
	}
	return t
}

// NotifyKinds is the set of backend-emitted event types that may surface as a
// desktop toast. The frontend notification-type registry mirrors this list (adding
// its own frontend-only "prompt" kind for ask/permission cues, which has no backend
// event). Keep the two in sync when adding a notifiable event type.
var NotifyKinds = []string{
	TypeChat,
	TypeSchedule,
	TypeFlow,
	TypeSpawned,
	TypeWorker,
	TypeCoordination,
	TypeAutomation,
	TypeArtifact,
	TypeBoard,
	TypeAgent,
	TypeAnomaly,
}

var notifyKindSet = func() map[string]bool {
	m := make(map[string]bool, len(NotifyKinds))
	for _, t := range NotifyKinds {
		m[t] = true
	}
	return m
}()

// IsNotifyKind reports whether an event type is a user-facing notifiable kind
// (as opposed to a control/stream signal).
func IsNotifyKind(t string) bool { return notifyKindSet[t] }
