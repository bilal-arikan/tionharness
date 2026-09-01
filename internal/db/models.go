package db

// backfillProviderInstance fills a zero-value ProviderInstanceID at READ time
// (_Docs/71 §2.5/§3): an agent row written before this field existed carries
// "" here, but its Provider already holds a kind id, and every migrated
// default instance's id equals its kind id — so Provider resolves to the
// right instance with no separate bulk-migration pass. A wholly empty
// Provider (row predates even that field) falls back to the keyless
// claude-cli default, matching Registry.Get's historical empty-provider
// behaviour. Called on every read path (GetAgent, listAgents) rather than
// once at load, per the plan's "no bulk write" requirement — the on-disk row
// is left untouched until the agent is next explicitly saved.
func (a Agent) backfillProviderInstance() Agent {
	if a.ProviderInstanceID != "" {
		return a
	}
	if a.Provider != "" {
		a.ProviderInstanceID = a.Provider
	} else {
		a.ProviderInstanceID = "claude-cli"
	}
	return a
}

// ProviderRef is the provider INSTANCE id this agent must be resolved against
// (Registry.Get/Available take an instance id, NOT a kind id — _Docs/71 §2.5,
// K3). Every runtime call site uses this instead of Agent.Provider: the two are
// identical for a migrated default instance (id == kind id), but an agent bound
// to a second instance of the same kind (e.g. a second Anthropic account, or a
// separately-authenticated Claude CLI home) would otherwise silently resolve to
// the DEFAULT instance of its kind and run on the wrong credentials/config home.
//
// The Provider fallback covers an Agent value built in memory (tests, imports)
// that never passed through backfillProviderInstance; an unknown instance id
// still errors at Registry.Get rather than falling back to a default.
func (a Agent) ProviderRef() string {
	if a.ProviderInstanceID != "" {
		return a.ProviderInstanceID
	}
	return a.Provider
}

// Agent is an autonomous AI entity bound to a provider/model.
type Agent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Soul     string `json:"soul"`
	Identity string `json:"identity"`
	// System agents back built-in AI jobs. Their stable SystemKey identifies the
	// role across workspaces; they may be edited or disabled, but not deleted.
	System    bool   `json:"system,omitempty"`
	SystemKey string `json:"systemKey,omitempty"`
	Disabled  bool   `json:"disabled,omitempty"`
	// Provider is now DERIVED: the KIND id of the provider instance this agent
	// is bound to (ProviderInstanceID), kept in sync on every write via
	// agent.SyncProviderFields (_Docs/71 §2.5, K3). Every kind-keyed reader
	// (billing, context-window sizing, usage rollups — _Docs/71 §4.1) keeps
	// consuming this field unchanged; only Registry.Get/Available take the
	// instance id instead.
	Provider string `json:"provider"`
	// ProviderInstanceID is the single source of truth for which provider
	// INSTANCE this agent uses (_Docs/71 §2.5, K3) — the id Registry.Get/
	// Available resolve against. May be "" on an agent row written before this
	// field existed; GetAgent/listAgents backfill it from Provider at read time
	// (kind id == default instance id for every migrated instance, _Docs/71 §3).
	ProviderInstanceID string `json:"providerInstanceId"`
	Model              string `json:"model"`
	// ThinkingLevel requests extended reasoning. Valid values are exactly
	// "off" | "low" | "medium" | "high" | "xhigh" | "max" | "ultra" (providers.
	// ValidThinkingLevels) — the empty string is NO LONGER valid. It used to be
	// a third state next to "off" that meant two different things depending on
	// the path (no thinking natively, "high" effort on the CLI); writes are now
	// rejected at the API and legacy rows are filled in once at boot by
	// BackfillThinkingLevels.
	ThinkingLevel string `json:"thinkingLevel"`
	// NativeWebSearch controls whether the agent may use its CLI provider's OWN
	// web search (codex `web_search`; Claude Code's WebSearch/WebFetch built-ins).
	//
	// NIL (field absent on disk, e.g. every agent written before this toggle
	// existed) MEANS ENABLED — that is the product default: an agent can search
	// the web out of the box. An explicit false switches the natives off, leaving
	// TionHarness's bridged WebSearch/WebFetch tools as the only path (their calls
	// are the ones that carry trace and usage accounting). Read it through
	// NativeWebSearchEnabled, never as a bare bool: the zero value of a plain bool
	// would silently mean "off" for every pre-existing agent. Ignored by non-CLI
	// providers, which get their web tools from the request itself.
	NativeWebSearch *bool `json:"nativeWebSearch,omitempty"`
	// PermissionMode gates how the agent's tool use is approved:
	// "read-only" | "ask" | "auto". Empty defaults to "auto". For the claude-cli
	// path this maps to the CLI's --permission-mode / --dangerously-skip-permissions
	// flags (headless mode refuses edits without an explicit mode).
	PermissionMode string `json:"permissionMode"`

	// InboundPolicy decides what happens to messages ADDRESSED AT this agent
	// (send_message into its inbox, send_to_worker into one of its worker
	// sessions): "accept" | "hold" | "refuse". Empty = unset = accept, which is
	// exactly the behaviour before this field existed, so old agent rows keep
	// working unchanged. A session may override it (Session.InboundPolicy).
	// See internal/db/models_agentmsg.go.
	InboundPolicy string `json:"inboundPolicy,omitempty"`

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

	// CoordinatorMode makes this agent a coordinator BY DEFAULT: every new session
	// opened for it starts with Session.CoordinatorMode on, so the agent arrives
	// with the coordinator manual and the spawn_worker/send_to_worker/stop_worker/
	// list_workers tools instead of needing a per-session toggle. This is what lets
	// a workspace template ship a ready-to-run PM/CTO team.
	//
	// It is a DEFAULT, not the live state: the session keeps its own flag, which is
	// what every gate actually reads (Session.IsCoordinator). An agent may still
	// turn its own session's mode off with set_coordinator_mode when it drops back
	// to single-threaded work, exactly as before — same shape as Model, which is
	// also configured on the agent and snapshotted per session.
	//
	// Seeding is skipped for sessions created INSIDE a coordinator tree: there the
	// spawner decides (see Runtime.SpawnWorker), because that is the only place
	// that knows the depth budget and can degrade to a plain worker.
	CoordinatorMode bool `json:"coordinatorMode,omitempty"`

	// CoordinatorPrompt is free-text orchestration guidance injected into the
	// system context ONLY while the session is actually coordinating
	// (Session.IsCoordinator), right after the shared coordinator manual. It is
	// where per-agent delegation direction belongs — "which workers to spawn",
	// "how to split this team's work" — instead of the Soul, which is injected
	// unconditionally and therefore costs tokens (and misleads) on a session that
	// has no delegation tools at all.
	//
	// Optional: an empty value injects NOTHING — no header, no blank block — so an
	// agent that never coordinates pays zero tokens for it. Agents persisted
	// before this field existed load with the zero value; no migration needed.
	CoordinatorPrompt string `json:"coordinatorPrompt,omitempty"`

	// CoordinatorWorkflow optionally pins a coordinator recipe (a skill with
	// kind=coordinator-workflow) on the sessions this agent opens. Only meaningful
	// together with CoordinatorMode. An unresolvable slug degrades to free
	// coordination rather than failing the session (see api/coordinator_prompt.go),
	// so a template whose recipe skill failed to install still runs.
	CoordinatorWorkflow string `json:"coordinatorWorkflow,omitempty"`

	// CreatedBy records the ID of the agent that created this agent through a
	// self-management tool. Empty means it was created by the user (UI/API).
	// Agents may only edit/delete entities that were created by an agent.
	CreatedBy string `json:"createdBy,omitempty"`

	// Deleted marks the agent as removed WITHOUT destroying it: the sessions it
	// owns stay readable and still resolve the name/avatar/colour they were
	// written with, so past conversations render their author as deleted instead
	// of degrading to a raw id. A deleted agent drops out of ListAgents (rosters,
	// pickers, defaults) while GetAgent still returns it — that is what history
	// rendering reads. Same shape as Session.State "archived" and
	// Artifact.Archived: put away, never destroyed.
	Deleted   bool  `json:"deleted,omitempty"`
	DeletedAt int64 `json:"deletedAt,omitempty"`

	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
}

// NativeWebSearchEnabled resolves the three-state NativeWebSearch toggle: an
// unset (nil) field means ENABLED, so an agent stored before the toggle existed —
// and any agent whose owner never touched the checkbox — keeps provider-native
// web search. Only an explicit false turns it off.
func (a Agent) NativeWebSearchEnabled() bool {
	return a.NativeWebSearch == nil || *a.NativeWebSearch
}

// SessionSchemaVersion is the current session-header format version, stamped on
// new sessions (Session.SchemaVersion). Bump it whenever header fields are added
// so a future loader can branch on the version. 1 = first versioned header
// (added pinned + the enriched per-message fields). 2 = generic participant model
// (Session.Participants + per-message AuthorKind/AuthorID/RecipientID). 3 adds
// execution lineage/classification metadata.
const SessionSchemaVersion = 3

const (
	ExecutionInteractive = "interactive"
	ExecutionSubagent    = "subagent"
	ExecutionWorker      = "worker"
	ExecutionFlow        = "flow"
	ExecutionSchedule    = "schedule"
	ExecutionAutomation  = "automation"
	ExecutionSystem      = "system"

	CategoryChat       = "chat"
	CategorySubagent   = "subagent"
	CategoryWorker     = "worker"
	CategoryFlow       = "flow"
	CategoryAutomation = "automation"
	CategorySystem     = "system"

	ContextIsolated  = "isolated"
	ContextInherited = "inherited"

	VisibilityUser     = "user"
	VisibilityInternal = "internal"
)

// UserParticipantID is the stable participant id of the human principal — the
// top-authority participant every session implicitly contains. A human-authored
// message has AuthorID == UserParticipantID, distinguishing it uniformly from an
// agent-authored one (whose AuthorID is the agent id).
const UserParticipantID = "user"

// BroadcastRecipientID addresses every other participant in the thread at once.
const BroadcastRecipientID = "*"

// SessionKindInsight is the Session.Kind of a retrospective insight scan's
// read-only transcript (SourceID == the insight run's id). It is read-only in the
// strong sense: the API refuses user messages and agent turns for it.
const SessionKindInsight = "insight"

// machineTranscriptKinds is the set of Session.Kind values whose transcript is
// WRITTEN BY the system as a record of an automated run, rather than driven by a
// user or an agent turn. They are a distinct class from the execution kinds
// (task/flow/schedule/...): those are real work the user cares to read and
// re-enter, these are read-only reports.
//
// One set, one rule: retrospective scanning must not feed on its own output, and
// such a session must not raise an unread badge the user cannot clear (it is
// hidden from the default sessions view). Both call sites derive their behaviour
// from IsMachineTranscriptKind so a future kind gets both properties at once,
// instead of each site growing its own `kind == "insight"` special case.
var machineTranscriptKindList = []string{SessionKindInsight}

// IsMachineTranscriptKind reports whether a Session.Kind is a system-written
// record of an automated run (see machineTranscriptKindList).
func IsMachineTranscriptKind(kind string) bool {
	for _, k := range machineTranscriptKindList {
		if k == kind {
			return true
		}
	}
	return false
}

// MachineTranscriptKinds returns the machine-written session kinds, for callers
// that need the set itself (e.g. a scan's default exclusion list).
func MachineTranscriptKinds() []string {
	out := make([]string, len(machineTranscriptKindList))
	copy(out, machineTranscriptKindList)
	return out
}

// writableSessionKindList is the set of Session.Kind values into which a NEW
// USER TURN may be started. Manual chats ("" / "chat") plus "spawned" sessions
// (spawn tool and handoff children) are linear transcripts a human is meant to
// keep talking to. Every other kind — task, flow, automation, flow-coordinator,
// insight — is an orchestrator-owned transcript: a new user turn there has
// no run to attach to.
//
// "worker" is writable (TSK507). A worker session is not a finished run log but a
// live single-agent conversation the coordinator is already talking INTO —
// SendToWorker injects user-role turns into it while it runs. The human watching
// that transcript must be able to do the same: answer a question the worker
// asked, correct its course, or add missing context, instead of being limited to
// reading. Interleaving is bounded the same way "schedule" is: a human turn and a
// coordinator-injected turn claim the session's single turn slot (turnqueue), so
// they serialize.
//
// Peer messages between agents no longer have a kind of their own: send_message
// delivers into the recipient's standing "chat" thread (see
// internal/agent/agentmsg.go), which is writable by virtue of being a chat, so
// the human can join that conversation too.
//
// "schedule" is writable too, and is the one deliberate exception to that rule. A
// schedule session is not a per-run log: it is the agent's single long-lived cron
// thread that every fire of a reuse-mode schedule appends to, so it reads as one
// continuing conversation the user is meant to steer — answer a question the
// scheduled turn asked, correct it, or hand it more context before the next tick.
// The concurrency worry that keeps the other kinds closed does not apply: a user
// turn and a scheduled turn both claim the session's single turn slot (turnqueue),
// which serializes them instead of letting them interleave.
//
// "automation-run" and "schedule-run" are writable for the same reason: a
// one-shot automation fire (see internal/agent.SessionKindAutomationRun) or a
// spawn-mode schedule fire's own fresh session is a linear transcript like
// "spawned", just tagged distinctly so the sidebar groups it under "Otomasyon"
// instead of "Spawn". Neither reuses the plain "automation"/"schedule" kind: those
// are looked up by (agentID, Kind) alone (SourceID ignored, see
// getOrCreateKindSession/GetOrCreateSourceSession), so a one-shot fire tagged with
// the shared kind would collide with — and silently absorb turns meant for — the
// agent's one persistent maintenance/cron thread.
//
// This list is the single source of truth for the whole product; the frontend's
// isWritableSessionKind (frontend/src/shared/lib/sessionKind.ts) mirrors it and the
// two must be changed together.
var writableSessionKindList = []string{"", "chat", "spawned", "schedule", "automation-run", "schedule-run", "worker"}

// IsWritableSessionKind reports whether a new user turn may be started in a
// session of this kind (see writableSessionKindList).
func IsWritableSessionKind(kind string) bool {
	for _, k := range writableSessionKindList {
		if k == kind {
			return true
		}
	}
	return false
}

// WritableSessionKinds returns the kinds that accept a new user turn.
func WritableSessionKinds() []string {
	out := make([]string, len(writableSessionKindList))
	copy(out, writableSessionKindList)
	return out
}

// IsImmutableSessionKind reports whether a session of this kind can never be
// modified at all — no turn ever runs in it, so not even the control/answer/
// rewind paths of a live turn are meaningful.
//
// This is deliberately the SAME set as the machine-written transcripts: a
// system-written record of an automated run is exactly the thing that has no
// live turn to steer. Rather than duplicating the list, immutability is defined
// as a property of that class, so a future machine-transcript kind gets it for
// free. Non-writable is the weaker rule (no NEW turn); immutable is the strong
// one (nothing at all).
func IsImmutableSessionKind(kind string) bool {
	return IsMachineTranscriptKind(kind)
}

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
	ID            string `json:"id"`
	AgentID       string `json:"agentId"`
	Kind          string `json:"kind"`
	SourceID      string `json:"sourceId,omitempty"`
	ExecutionType string `json:"executionType,omitempty"`
	Category      string `json:"category,omitempty"`
	ContextMode   string `json:"contextMode,omitempty"`
	Visibility    string `json:"visibility,omitempty"`
	TargetProfile string `json:"targetProfile,omitempty"`
	TargetAgentID string `json:"targetAgentId,omitempty"`
	// RetryOfSessionID links a subagent child session to the attempt it replaces,
	// and Attempt numbers the run within that chain (1 for the first try). Both are
	// empty/zero outside a retry chain. The link points at the IMMEDIATE previous
	// attempt rather than the first one, so walking it backwards reconstructs the
	// whole chain in order; Attempt exists so a single row is readable ("attempt
	// 3 of this task") without that walk.
	RetryOfSessionID string `json:"retryOfSessionId,omitempty"`
	Attempt          int    `json:"attempt,omitempty"`
	Title            string `json:"title"`
	MessageCount     int    `json:"messageCount"`
	// ToolCallCount is the session's LIFETIME count of executed tool calls, summed
	// from each persisted assistant message's tool steps (kind=="tool"). Like
	// MessageCount it is a monotonic per-session counter; a counter-triggered
	// automation with metric "tool" watches it crossing an interval multiple.
	// Recomputed from the message lines on load (see rebuild paths) so it survives
	// the header's stale on-disk value.
	ToolCallCount int    `json:"toolCallCount"`
	State         string `json:"state"`
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

	// InboundPolicy overrides the recipient agent's inbound policy for messages
	// delivered INTO this session ("accept" | "hold" | "refuse"). Empty = inherit
	// the agent's, which itself defaults to accept. Lets one worker session be
	// put on hold without changing the agent for every other session.
	InboundPolicy string `json:"inboundPolicy,omitempty"`

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

	// StallNudges counts how many times the coordinator stall guard has caught this
	// session claiming a spawn it never made (see internal/agent/coordination_stall.go).
	// The in-memory slot streak resets on a clean coordination call and dies with the
	// process; this is the cumulative, restart-surviving tally a later escalation tier
	// (or a forensic pass over session.json) can act on.
	StallNudges int `json:"stallNudges,omitempty"`

	// RunState records how this session's LAST background work turn ended, using the
	// turn-outcome vocabulary (completed / failed / killed / timeout / incomplete).
	// DISTINCT from State, which is the two-valued visibility field (active/archived).
	// Empty on a session that has never run a background turn — including every
	// session written before this field existed, which is why it is omitempty: no
	// migration rewrites old session.json files.
	RunState string `json:"runState,omitempty"`
	// RunStateAt is when RunState was last written (unix seconds).
	RunStateAt int64 `json:"runStateAt,omitempty"`

	// Conversation compaction state (see internal/conversation).
	Summary         string `json:"summary"`
	SummaryMsgCount int    `json:"summaryMsgCount"`
	// CompactionCount is how many rolling-summary folds this session has taken —
	// bumped by every SetSessionSummary write (budgeted "auto" fold and manual
	// /compact alike) and reset together with the summary when a rewind truncates
	// past the summarized boundary. It is the fold ORDINAL that rides each
	// compaction debug event, so cumulative semantic drift (fold #5's summary is
	// not fold #1's) is observable instead of inferred. 0 on sessions written
	// before this field existed: their first fold after the upgrade reads as #1.
	CompactionCount int `json:"compactionCount,omitempty"`

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
	//     stop_worker/list_workers tools). It is seeded at creation from the
	//     agent's own default (Agent.CoordinatorMode) and stays the live value
	//     from then on — set_coordinator_mode and the UI toggle move THIS field,
	//     never the agent's.
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
	// CLINativeCompactionPending is durably set before invoking a CLI native
	// compactor and cleared only when its rotated resume state and boundary are
	// committed together. A surviving marker forces the next turn cold so an
	// externally-mutated CLI thread is never resumed with the old delta cursor.
	CLINativeCompactionPending bool `json:"cliNativeCompactionPending,omitempty"`

	// CLICompactMsgCount is the transcript boundary the PROVIDER compacted its own
	// context at — recorded whenever a CLI reports a completed native compaction,
	// whether it fired on its own mid-turn (auto) or was asked for by /compact.
	// Messages before it live on only as the CLI's internal summary, so the tool
	// trace persisted for them is NO LONGER in the warm thread and must not be
	// charged to the context meter or the fold gate. 0 = the CLI has not compacted
	// this session yet (the rolling-summary boundary alone applies).
	CLICompactMsgCount int `json:"cliCompactMsgCount,omitempty"`

	// Model is a snapshot of the model serving this session's CURRENT turn —
	// set at creation from the agent's configured model and updated when a turn's
	// actual response model differs (e.g. the agent was reconfigured mid-session).
	// It answers "which model is this session running on?" in O(1) without
	// scanning messages or SessionUsage.ByModel. Per-message accuracy is still in
	// Message.Model.
	Model string `json:"model,omitempty"`

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
	// CLIColdStart marks an assistant turn that a CLI provider had to serve from a
	// BRAND NEW underlying CLI session because the warm `--resume` path did not
	// apply — the stored thread was unresumable, a fold re-baselined it, or this is
	// the session's first CLI turn. Only ever set on a turn where resume was
	// otherwise enabled, so it reads as "here the CLI conversation restarted", not
	// "resume is off". The transcript draws a divider at this boundary, mirroring
	// the cold-cache divider. Empty/false everywhere else.
	CLIColdStart bool `json:"cliColdStart,omitempty"`
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
	InputTokens        int `json:"in"`
	OutputTokens       int `json:"out"`
	CacheReadTokens    int `json:"cacheRead,omitempty"`
	CacheWriteTokens   int `json:"cacheWrite,omitempty"`
	CacheWrite5mTokens int `json:"cacheWrite5m,omitempty"`
	CacheWrite1hTokens int `json:"cacheWrite1h,omitempty"`
}

// MessageFeedback is the user's rating of an assistant turn. Rating is +1 (up),
// -1 (down) or 0 (cleared); Note is an optional free-text comment; At is the
// unix-second timestamp the rating was set.
type MessageFeedback struct {
	Rating int    `json:"rating"`
	Note   string `json:"note,omitempty"`
	At     int64  `json:"at"`
}
