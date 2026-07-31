package db

// Agent is an autonomous AI entity bound to a provider/model.
type Agent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Soul     string `json:"soul"`
	Identity string `json:"identity"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// ThinkingLevel requests extended reasoning: "" / "off" | "low" | "medium" |
	// "high". Applied on plain (non-tool) completions; anthropic provider only.
	ThinkingLevel string `json:"thinkingLevel"`
	// PermissionMode gates how the agent's tool use is approved:
	// "read-only" | "ask" | "auto". Empty defaults to "auto". For the claude-cli
	// path this maps to the CLI's --permission-mode / --dangerously-skip-permissions
	// flags (headless mode refuses edits without an explicit mode).
	PermissionMode string `json:"permissionMode"`

	// Visual identity for the roster avatar. Avatar holds an optional emoji/glyph
	// rendered inside the circle; Color is an optional hex accent (e.g. "#7c3aed").
	// Both may be empty — the frontend then derives a deterministic look from ID.
	Avatar string `json:"avatar"`
	Color  string `json:"color"`

	// Tool access. MCPEnabled gates whether the agent is offered tools at all.
	//
	// Agents reach EVERY workspace-active tool by default (after per-creation-path
	// defaulting — see each db.Agent literal at the call site that flips false to
	// true; the DB layer does NOT default this because Go bools cannot distinguish
	// "unset" from "explicit false"). Access is then narrowed two independent ways:
	//   - BlockedTools: per-agent denylist (the user-facing model in agent
	//     detail). Empty = nothing blocked = all tools. New tools added later are
	//     reachable automatically unless explicitly blocked here.
	//   - AllowedTools: a legacy allowlist of tool-name patterns. Retained because
	//     the built-in subagent profiles (explore/coder/reviewer) restrict an
	//     isolated worker to a fixed tool set. Empty = allow all. User-facing
	//     agents leave this empty and use the denylist instead.
	// Both compose: a tool is offered iff it is not blocked AND (the allowlist is
	// empty OR matches it).
	MCPEnabled   bool   `json:"mcpEnabled"`
	AllowedTools string `json:"allowedTools"` // JSON array (legacy allowlist; subagent profiles)
	BlockedTools string `json:"blockedTools"` // JSON array (per-agent denylist)

	// ToolOverrides is the per-agent tool override map (JSON object: tool name —
	// or a "prefix*" pattern — → tier). Tiers are the four visibility tiers
	// ("full"/"summary"/"name-only"/"hidden") plus "blocked". It is the THIRD and
	// last layer of the tool precedence chain:
	//
	//	code default  <  workspace ToolVisibility  <  agent ToolOverrides
	//
	// A key absent from the map inherits the workspace-effective tier. The
	// "blocked" tier is not a visibility state: it drops the tool from the agent's
	// catalog entirely and is the successor to BlockedTools, which is now DERIVED
	// from this map on every write (a one-way mirror kept so market packs,
	// workspace templates and pre-existing agent files still parse). Reading code
	// should go through agent.ParseToolOverrides, which folds a legacy
	// BlockedTools list back into this map.
	ToolOverrides string `json:"toolOverrides"` // JSON object (name/pattern → tier)

	// Skills is the list of skill slugs enabled for this agent. Only these skills
	// are advertised to the agent and loadable via use_skill. Empty means the
	// agent has no assigned skills. Skills themselves are a shared library
	// (global/workspace/project tiers) — never agent-owned.
	Skills []string `json:"skills"`

	// CreatedBy records the ID of the agent that created this agent through a
	// self-management tool. Empty means it was created by the user (UI/API).
	// Agents may only edit/delete entities that were created by an agent.
	CreatedBy string `json:"createdBy,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// SessionSchemaVersion is the current session-header format version, stamped on
// new sessions (Session.SchemaVersion). Bump it whenever header fields are added
// so a future loader can branch on the version. 1 = first versioned header
// (added pinned + the enriched per-message fields). 2 = generic participant model
// (Session.Participants + per-message AuthorKind/AuthorID/RecipientID).
const SessionSchemaVersion = 2

// UserParticipantID is the stable participant id of the human principal — the
// top-authority participant every session implicitly contains. A human-authored
// message has AuthorID == UserParticipantID, distinguishing it uniformly from an
// agent-authored one (whose AuthorID is the agent id).
const UserParticipantID = "user"

// BroadcastRecipientID addresses every other participant in the thread at once.
const BroadcastRecipientID = "*"

// AuthorKind classifies who wrote a message in the participant model: the human
// principal, an agent, or the system.
const (
	AuthorUser   = "user"
	AuthorAgent  = "agent"
	AuthorSystem = "system"
)

// Session is a conversation thread belonging to an agent.
//
// Kind is the broad category of what produced the transcript:
//
//	chat      manual user conversation (default)
//	task      a kanban task's run history (one session per task)
//	flow      an orchestration flow's run history (one session per flow)
//	schedule  an agent's scheduled-prompt deliveries (one per agent)
//
// SourceID links the session back to the entity that owns it (a task or flow id);
// it is empty for plain chat and for agent-keyed kinds (schedule). This
// is the unification primitive: every execution path funnels its output into a
// Session, so a single streamable transcript viewer and the unified "executions"
// feed can render task runs, flow runs and scheduled deliveries like any chat.
type Session struct {
	ID           string `json:"id"`
	AgentID      string `json:"agentId"`
	Kind         string `json:"kind"`
	SourceID     string `json:"sourceId,omitempty"`
	Title        string `json:"title"`
	MessageCount int    `json:"messageCount"`
	State        string `json:"state"`
	// Unread is true when an agent reply landed in this session while it was not
	// the one being viewed; cleared when the user opens it.
	Unread bool `json:"unread"`

	// WorkingDir is this session's working directory (cwd) for the built-in
	// filesystem/shell tools — like `cd /path/to/project` in a terminal. When set
	// it overrides the workspace default: relative paths resolve here and the
	// shell starts here, and the agent is told its cwd + git branch in context.
	// Empty = use the workspace default working dir. Inspired by the external agent project's
	// per-session working directory.
	WorkingDir string `json:"workingDir,omitempty"`

	// SchemaVersion is the session-header format version, bumped whenever new
	// header fields are added so a future loader can detect and migrate old headers.
	// 0 = pre-versioning (sessions created before this field). Set at creation.
	SchemaVersion int `json:"v,omitempty"`

	// Pinned keeps the session at the top of the sidebar list regardless of recency.
	Pinned bool `json:"pinned,omitempty"`

	// Tags are free-form labels on the session, editable by both the user (UI) and
	// agents (set_session_tags). They organize sessions and, crucially, drive
	// tag-triggered automations: when a tagged session's turn finishes, any
	// Automation watching that tag fires (see internal/db/models_automation.go).
	Tags []string `json:"tags,omitempty"`

	// StuckTurns counts CONSECUTIVE turns of this session that ended badly (turn
	// error or a guardrail halt). It is reset to 0 by any clean turn, or when the
	// "stuck" tag is removed (a fixer resolving the session). At the configured
	// threshold the session is tagged "stuck" and further AUTONOMOUS turns are
	// refused until a human (or a repair automation) intervenes — the persistent,
	// process-restart-surviving sibling of the per-turn loop guards.
	StuckTurns int `json:"stuckTurns,omitempty"`

	// Conversation compaction state (see internal/conversation).
	Summary         string `json:"summary"`
	SummaryMsgCount int    `json:"summaryMsgCount"`

	// Context-reset / handoff lineage (see internal/agent/handoff.go). When a
	// session nears its context limit it is not just compacted in place: a handoff
	// artifact is written and a FRESH session is spawned to continue the work in a
	// clean window (Anthropic "context reset" pattern). ParentSessionID points back
	// to the session this one continues, so the UI can walk the reset chain;
	// HandoffArtifactID is the handoff artifact written into the PARENT at reset.
	// Both empty for an ordinary (non-handoff) session.
	ParentSessionID   string `json:"parentSessionId,omitempty"`
	HandoffArtifactID string `json:"handoffArtifactId,omitempty"`

	// Participants is the roster of agent ids taking part in this thread, beyond
	// the implicit human "user" (UserParticipantID) which is always a participant.
	// It grows as agents author or are addressed in the session (see
	// store.AddMessage). AgentID stays the DEFAULT responder — the agent that
	// answers when a turn carries no explicit routing — while Participants is the
	// full set the UI offers to route a message to. Empty on legacy sessions →
	// treat as [AgentID] via SessionParticipants.
	Participants []string `json:"participants,omitempty"`

	// Multi-agent coordination (see internal/agent/coordination.go, _Docs/47).
	//
	// Since the unlimited-depth rework these are TWO ORTHOGONAL axes, because a
	// mid-level node in a coordinator tree is BOTH a worker (it reports up) and a
	// coordinator (it drives its own workers):
	//
	//   - Role is LINEAGE only: "worker" (this session was spawned by a
	//     coordinator) or "" (top-level / ordinary). The legacy value
	//     "coordinator" is still accepted on old sessions and read as
	//     CoordinatorMode=true — always test with Session.IsCoordinator(), never
	//     with Role == "coordinator".
	//   - CoordinatorMode is the CAPABILITY: this session may spawn/drive workers
	//     (gets the coordinator system prompt + the spawn_worker/send_to_worker/
	//     stop_worker/list_workers tools).
	//
	// CoordinatorSessionID is the worker's back-link to the coordinator session
	// that spawned it, so a finished worker turn can inject its
	// <task-notification> into the right coordinator. Empty on a root coordinator
	// or an ordinary session. Distinct from ParentSessionID, which is the handoff
	// "continues-from" lineage — a worker is spawned-by, not a reset of.
	Role                 string `json:"role,omitempty"`
	CoordinatorMode      bool   `json:"coordinatorMode,omitempty"`
	CoordinatorSessionID string `json:"coordinatorSessionId,omitempty"`

	// RootCoordinatorSessionID / CoordinatorDepth address this session inside its
	// coordinator TREE, mirroring FlowRun.RootRunID/ParentRunID (see _Docs/62):
	// the root is reachable in O(1) instead of by walking CoordinatorSessionID
	// hop by hop, and the depth backs the CoordinatorMaxDepth guard. Root is ""
	// on the tree's own root (it is its own root — see RootCoordinator()); depth
	// is 0 there and +1 per level below.
	RootCoordinatorSessionID string `json:"rootCoordinatorSessionId,omitempty"`
	CoordinatorDepth         int    `json:"coordinatorDepth,omitempty"`

	// CoordinatorWorkflow is the slug of the selected coordinator recipe (M5) —
	// a saved orchestration pattern (skill with kind=coordinator-workflow) whose
	// body is injected into this coordinator session's system prompt. Empty means
	// the free (recipe-less) coordinator. Only meaningful on a coordinator.
	// NOT inherited by child coordinators: a recursive recipe (e.g. tournament)
	// would otherwise repeat itself forever down the tree.
	CoordinatorWorkflow string `json:"coordinatorWorkflow,omitempty"`
	// CoordinatorReportPending marks a MID-LEVEL node whose finished turn was NOT
	// reported to its coordinator, because its own workers were still running at
	// the time (see agent/coordination_tree.go). It therefore still owes an upward
	// report, delivered by report_to_coordinator or the settle backstop.
	//
	// Persisted rather than kept in memory: the gap it covers is precisely a
	// restart. A mid-level node in this state looks perfectly healthy on disk — its
	// last message is its own assistant reply — so orphan recovery does not touch
	// it, and without this flag its coordinator would wait forever for a report
	// nothing remembers is owed.
	CoordinatorReportPending bool `json:"coordinatorReportPending,omitempty"`

	// CoordinatorMaxTurns optionally overrides the workspace CoordinatorMaxTurns
	// notify-loop cap for THIS coordinator session (0 = use the workspace default).
	// Resolved from the selected recipe's max_turns when the workflow is set.
	CoordinatorMaxTurns int `json:"coordinatorMaxTurns,omitempty"`

	// claude-cli session resume (opt-in, ClaudeResume setting). CLISessionID is the
	// CLI's server-side session to --resume on the next turn (rotates each turn);
	// CLISentMsgCount is how many of this session's messages the CLI has already
	// seen, so the next turn sends only the newer ones (the delta). Empty/0 = no
	// warm CLI session yet (next turn starts cold and captures a fresh id).
	CLISessionID    string `json:"cliSessionId,omitempty"`
	CLISentMsgCount int    `json:"cliSentMsgCount,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// SessionRoleCoordinator is the LEGACY Role value written before coordinator mode
// became its own field. It is still honoured on read (IsCoordinator) so sessions
// created by older builds keep working without a migration, but nothing writes it
// any more — new coordinators set CoordinatorMode instead.
const SessionRoleCoordinator = "coordinator"

// SessionRoleWorker marks a session that was spawned BY a coordinator. It says
// nothing about whether this session itself drives workers (see CoordinatorMode):
// a mid-level node in a coordinator tree carries Role=="worker" AND
// CoordinatorMode==true.
const SessionRoleWorker = "worker"

// IsCoordinator reports whether this session may spawn and drive workers. Use this
// everywhere instead of comparing Role to "coordinator": since the unlimited-depth
// rework a worker can ALSO be a coordinator, and Role no longer carries the
// capability. The legacy Role value is still accepted so pre-existing coordinator
// sessions keep their tools and prompt.
func (s Session) IsCoordinator() bool {
	return s.CoordinatorMode || s.Role == SessionRoleCoordinator
}

// IsWorker reports whether this session reports UP to a coordinator, i.e. it has a
// parent in a coordinator tree. Derived from the back-link rather than Role so it
// stays true for a mid-level node (which is a worker and a coordinator at once).
func (s Session) IsWorker() bool {
	return s.CoordinatorSessionID != "" || s.Role == SessionRoleWorker
}

// RootCoordinator returns the id of the root of this session's coordinator tree.
// A root is its own root (RootCoordinatorSessionID is stored empty there), so this
// normalizes the "" case to the session's own id — callers can then key tree-wide
// budgets on the result without a special case. Returns "" only for a session that
// is neither a coordinator nor a worker.
func (s Session) RootCoordinator() string {
	if s.RootCoordinatorSessionID != "" {
		return s.RootCoordinatorSessionID
	}
	// Legacy worker (spawned before the root was stamped): every pre-rework tree was
	// exactly one level deep, so its parent IS the root.
	if s.CoordinatorSessionID != "" {
		return s.CoordinatorSessionID
	}
	if s.IsCoordinator() {
		return s.ID
	}
	return ""
}

// Message is a single turn within a session.
type Message struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"`
	Role      string `json:"role"` // system | user | assistant | tool
	// AgentID records which agent produced an assistant turn (empty for user/
	// system). In a multi-agent session different turns may come from different
	// agents (via per-turn routing); the UI shows each turn's agent. On a USER
	// turn it carries the legacy dual meaning "routed recipient agent" — retained
	// for backward compatibility; the participant fields below are the canonical
	// source. For an agent turn AgentID == AuthorID.
	AgentID string `json:"agentId"`

	// Author/recipient participant model (generic multi-participant threads). A
	// session is a thread among participants: the human "user" (top authority) and
	// one or more agents. Every message records who wrote it and, optionally, whom
	// it is addressed to, so any responding agent can reconstruct the full
	// who→whom map of the conversation.
	//
	//   AuthorKind  the writer's class: "user" | "agent" | "system".
	//   AuthorID    the participant id: the agent id for an agent author,
	//               UserParticipantID for the human, empty for system.
	//   RecipientID the participant this message is directed at: an agent id,
	//               BroadcastRecipientID ("*"), or empty = the thread at large.
	//
	// Populated going forward on every write. For messages stored before this
	// model they are derived at read time from Role + the legacy AgentID by
	// NormalizeParticipants, so old session.jsonl stays readable without migration.
	AuthorKind  string `json:"authorKind,omitempty"`
	AuthorID    string `json:"authorId,omitempty"`
	RecipientID string `json:"recipientId,omitempty"`

	Text             string `json:"text"`
	ToolCalls        string `json:"toolCalls"`
	ReasoningContent string `json:"reasoningContent"`
	// Steps is a JSON array of agent.TurnStep records: the ordered trace of
	// thinking, intermediate text and tool calls behind this turn. Empty for
	// plain (non-tool) replies. Drives the rich chat turn renderer.
	Steps string `json:"steps"`
	// Interrupted marks an assistant reply that was reconstructed from a crash
	// sidecar (the process died mid-stream): the text/trace are partial and the UI
	// flags the turn as cut off. Empty/false for normal turns.
	Interrupted bool `json:"interrupted,omitempty"`
	// Cancelled marks an assistant reply the USER stopped mid-turn (Shift+stop) —
	// distinct from Interrupted (a crash-recovered partial). The text/trace are
	// whatever completed before the stop. Empty/false for normal turns.
	Cancelled bool `json:"cancelled,omitempty"`
	// Model is the provider model that actually served THIS assistant turn (the
	// agent's model can change, and a turn may carry a per-turn override). Empty for
	// user/system turns.
	Model string `json:"model,omitempty"`
	// StopReason is why generation ended: "end_turn" | "max_tokens" | "tool_use" |
	// "refusal" | "stop_sequence". Lets the UI flag a truncated/refused reply
	// without re-deriving it. Empty for user/system or unknown.
	StopReason string `json:"stopReason,omitempty"`
	// Usage is THIS turn's token consumption (the per-bubble cost), so the
	// transcript is self-contained for export/analysis without cross-referencing
	// the usage rollup or debug journal. Nil for user/system or non-LLM turns.
	Usage *MessageUsage `json:"usage,omitempty"`
	// DurationMs is the wall-clock time this assistant turn took (request→reply).
	// 0 when unknown (user/system, or not measured).
	DurationMs int64 `json:"durationMs,omitempty"`
	// Feedback is the user's rating of an assistant turn (👍/👎 + optional note) —
	// a durable quality signal the reflector/eval can learn from. Nil = no rating.
	Feedback *MessageFeedback `json:"feedback,omitempty"`
	// Origin marks how a prompt-bearing message was produced, for display only:
	// "wake" = a schedule_wake auto-resume, "schedule" = a scheduled routine prompt.
	// Empty = a real user/assistant message. The role stays "user" so the model's
	// replayed context is unchanged; the UI keys off this to render auto prompts as
	// a "⏰ continuation" note instead of a user bubble (so the agent doesn't look
	// like it is asking itself).
	Origin string `json:"origin,omitempty"`
	// Attachments are user-supplied files (or pasted long text) sent with this
	// message. Stored on user turns; empty for assistant/system. The files live
	// under the workspace's uploads/ directory so agents can read them via their
	// sandboxed read_file tool (see Attachment.RelPath).
	Attachments []Attachment `json:"attachments,omitempty"`
	CreatedAt   int64        `json:"createdAt"`
}

// NormalizeParticipants back-fills AuthorKind/AuthorID/RecipientID from a
// message's Role and the legacy dual meaning of AgentID, for messages written
// before the participant model (or any that omitted the fields). Idempotent: a
// no-op once AuthorKind is set. Called on every write and on legacy read so the
// participant fields are always populated in memory without a disk migration.
func (m *Message) NormalizeParticipants() {
	if m.AuthorKind != "" {
		return
	}
	switch m.Role {
	case "assistant":
		m.AuthorKind = AuthorAgent
		m.AuthorID = m.AgentID // the authoring agent
	case "user":
		m.AuthorKind = AuthorUser
		m.AuthorID = UserParticipantID
		m.RecipientID = m.AgentID // legacy: AgentID on a user turn = routed recipient
	case "system":
		m.AuthorKind = AuthorSystem
	}
}

// SessionParticipants returns a session's participant agent roster, falling back
// to its single default agent for legacy sessions that predate the roster field.
// The human "user" participant is implicit and never included here.
func SessionParticipants(s Session) []string {
	if len(s.Participants) > 0 {
		return s.Participants
	}
	if s.AgentID != "" {
		return []string{s.AgentID}
	}
	return nil
}

// MessageUsage is a single assistant turn's token consumption, stored on the
// message so the transcript carries its own per-turn cost (compact keys to keep
// the JSONL line small). Mirrors providers.Usage but kept dependency-free here.
type MessageUsage struct {
	InputTokens      int `json:"in"`
	OutputTokens     int `json:"out"`
	CacheReadTokens  int `json:"cacheRead,omitempty"`
	CacheWriteTokens int `json:"cacheWrite,omitempty"`
}

// MessageFeedback is the user's rating of an assistant turn. Rating is +1 (up),
// -1 (down) or 0 (cleared); Note is an optional free-text comment; At is the
// unix-second timestamp the rating was set.
type MessageFeedback struct {
	Rating int    `json:"rating"`
	Note   string `json:"note,omitempty"`
	At     int64  `json:"at"`
}
