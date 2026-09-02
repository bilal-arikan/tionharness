package db

// Automation trigger kinds. An automation fires either on a tagged session
// finishing a turn (the original, default kind) or on a board (kanban) card
// change. TriggerKind == "" is treated as TriggerTag for backward compatibility
// with automation files written before board triggers existed.
const (
	TriggerTag     = "tag"     // fire when a session carrying TriggerTag ends a turn
	TriggerBoard   = "board"   // fire when a board card changes (see BoardOp)
	TriggerToken   = "token"   // fire when cumulative token spend crosses a TokenThreshold multiple
	TriggerCounter = "counter" // fire when a session activity counter crosses a CounterInterval multiple
)

// Counter automation metrics. A counter-triggered automation watches one of a
// session's monotonic activity counters cross an interval multiple. Unlike the
// token trigger (which counts cache-inflated spend and so fires unpredictably), a
// counter is a stable, intuitive cadence: message count grows ~1-2 per exchange,
// tool count grows by the tools a turn actually ran. CounterMetric == "" is
// treated as CounterMetricMessage.
const (
	CounterMetricMessage = "message" // Session.MessageCount (every user/assistant message)
	CounterMetricTool    = "tool"    // Session.ToolCallCount (executed tool calls)
)

// ValidCounterMetric reports whether metric is empty (defaults to message) or a
// known counter metric.
func ValidCounterMetric(metric string) bool {
	switch metric {
	case "", CounterMetricMessage, CounterMetricTool:
		return true
	default:
		return false
	}
}

// Counter automation scopes. A counter-triggered automation watches either a
// single session's lifetime counter (CounterScopeSession, the default) or the
// whole workspace's lifetime counter — the sum of every session's counter
// (CounterScopeWorkspace). The workspace total is cumulative (not daily-reset):
// it fires every CounterInterval of new activity across the workspace, which is
// the work-cadence a multi-agent setup wants. CounterScope == "" is treated as
// CounterScopeSession.
const (
	CounterScopeSession   = "session"
	CounterScopeWorkspace = "workspace"
)

// ValidCounterScope reports whether scope is empty (defaults to session) or a
// known counter-automation scope.
func ValidCounterScope(scope string) bool {
	switch scope {
	case "", CounterScopeSession, CounterScopeWorkspace:
		return true
	default:
		return false
	}
}

// Token automation scopes. A token-triggered automation watches either a single
// session's lifetime token spend (TokenScopeSession, the default) or the whole
// workspace's spend for the current day (TokenScopeWorkspace). TokenScope == ""
// is treated as TokenScopeSession.
const (
	TokenScopeSession   = "session"
	TokenScopeWorkspace = "workspace"
)

// ValidTokenScope reports whether scope is empty (defaults to session) or a known
// token-automation scope.
func ValidTokenScope(scope string) bool {
	switch scope {
	case "", TokenScopeSession, TokenScopeWorkspace:
		return true
	default:
		return false
	}
}

// EffectiveTokenScope resolves the stored TokenScope to a concrete scope, mapping
// the empty default to session. Callers (prompt vars, fire notifications) should
// use this instead of repeating the `== "" → session` fallback.
func (a Automation) EffectiveTokenScope() string {
	if a.TokenScope == "" {
		return TokenScopeSession
	}
	return a.TokenScope
}

// EffectiveCounterScope resolves the stored CounterScope to a concrete scope,
// mapping the empty default to session (mirrors EffectiveTokenScope).
func (a Automation) EffectiveCounterScope() string {
	if a.CounterScope == "" {
		return CounterScopeSession
	}
	return a.CounterScope
}

// Automation session modes. An agent-backed automation either spawns a FRESH
// session on every fire (SessionModeSpawn — the tag/board default: each fire is
// independent) or CONTINUES one persistent per-automation thread that carries its
// earlier turns forward (SessionModeContinue — the token/counter default: each
// fire resumes where the last left off, cron-schedule style). SessionMode == "" is
// resolved per trigger kind by EffectiveSessionMode.
const (
	SessionModeSpawn    = "spawn"
	SessionModeContinue = "continue"
)

// ValidSessionMode reports whether mode is empty (kind default) or a known mode.
func ValidSessionMode(mode string) bool {
	switch mode {
	case "", SessionModeSpawn, SessionModeContinue:
		return true
	default:
		return false
	}
}

// EffectiveSessionMode resolves the stored SessionMode to a concrete mode. An
// explicit value wins; empty falls back to the per-kind default that preserves the
// original behavior — token/counter continue their maintenance thread, everything
// else spawns fresh. (Meaningful only for agent-backed rules; the fire path ignores
// it for flow-backed ones.)
func (a Automation) EffectiveSessionMode() string {
	if a.SessionMode != "" {
		return a.SessionMode
	}
	switch a.TriggerKind {
	case TriggerToken, TriggerCounter:
		return SessionModeContinue
	default:
		return SessionModeSpawn
	}
}

// Board operation filters for a board-triggered automation. BoardOp == "" is
// treated as BoardOpMove (the common case: a card dragged to another column).
// BoardOpAny matches every card change (create/move/update/delete).
const (
	BoardOpAny    = "any"
	BoardOpMove   = "move"
	BoardOpCreate = "create"
	BoardOpUpdate = "update"
	BoardOpDelete = "delete"
)

// ValidBoardOp reports whether op is empty (defaults to move) or a known board
// operation filter.
func ValidBoardOp(op string) bool {
	switch op {
	case "", BoardOpAny, BoardOpMove, BoardOpCreate, BoardOpUpdate, BoardOpDelete:
		return true
	default:
		return false
	}
}

// Board automation actions. A board-triggered automation either SPAWNS a session
// (BoardActionSpawn, the default — the card change starts an agent or flow, i.e.
// the board drives execution) or performs a lightweight bookkeeping ACTION with
// no LLM call. BoardActionArchive archives the card (see SetTaskArchived) — the
// natural "done → archive" cleanup that would be wasteful as a spawned session.
// BoardAction == "" is treated as BoardActionSpawn. Ignored for non-board triggers.
const (
	BoardActionSpawn   = "spawn"
	BoardActionArchive = "archive"
	BoardActionMove    = "move"
)

// ValidBoardAction reports whether action is empty (defaults to spawn) or a known
// board action.
func ValidBoardAction(action string) bool {
	switch action {
	case "", BoardActionSpawn, BoardActionArchive, BoardActionMove:
		return true
	default:
		return false
	}
}

// Automation is an event-driven rule that starts a NEW session whenever a
// session carrying TriggerTag finishes a turn (StopReason end_turn). It takes
// the finishing session's final reply as the "result", renders it into
// PromptTemplate, and spawns TargetAgentID with the composed prompt. When the
// spawned session itself carries TriggerTag (the default — see SpawnTags) each
// completion re-fires the rule, forming a self-continuing loop.
//
// Automations are the tag-triggered complement to cron Schedules: a Schedule
// fires on a clock, an Automation fires on a tagged session completing. They are
// surfaced in the same Schedules screen (a separate "Automations" section) but
// kept a distinct entity so the cron Schedule model stays clean.
//
// Guardrails bound the loop: MaxIterations caps total fires, CooldownSec spaces
// them out, and Enabled is the kill switch. IterationCount accumulates across the
// whole chain (every spawned session shares TriggerTag → the same rule) so a
// runaway loop stops at MaxIterations.
type Automation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// TriggerKind selects what fires the automation: TriggerTag (default, "" is
	// treated the same) watches a finishing session's tags; TriggerBoard watches
	// board (kanban) card changes; TriggerToken watches cumulative token spend. The
	// board fields apply only when TriggerKind == TriggerBoard; the token fields
	// only when TriggerKind == TriggerToken.
	TriggerKind string `json:"triggerKind,omitempty"`
	// TokenScope selects what a token automation watches: TokenScopeSession
	// (default, "" is treated the same) = a single session's lifetime token spend;
	// TokenScopeWorkspace = the whole workspace's spend for the current day. Ignored
	// unless TriggerKind == TriggerToken.
	TokenScope string `json:"tokenScope,omitempty"`
	// TokenThreshold is the token INTERVAL for a token automation: it fires each
	// time cumulative spend crosses another multiple of this value (e.g. 100000 →
	// fires at 100k, 200k, 300k…). Crossing is detected statelessly from the
	// previous vs new cumulative total at record time, so no per-scope ledger is
	// needed. "Tokens" here means input+output+cacheRead+cacheWrite. Must be >=
	// MinTokenThreshold. Ignored unless TriggerKind == TriggerToken.
	TokenThreshold int `json:"tokenThreshold,omitempty"`
	// CounterMetric selects which session activity counter a counter automation
	// watches: CounterMetricMessage (default, "" is treated the same) or
	// CounterMetricTool. Ignored unless TriggerKind == TriggerCounter.
	CounterMetric string `json:"counterMetric,omitempty"`
	// CounterScope selects what a counter automation watches: CounterScopeSession
	// (default, "" is treated the same) = the crossing session's own counter;
	// CounterScopeWorkspace = the whole workspace's cumulative counter (sum of every
	// session). Ignored unless TriggerKind == TriggerCounter.
	CounterScope string `json:"counterScope,omitempty"`
	// CounterInterval is the count INTERVAL for a counter automation: it fires each
	// time the watched session counter crosses another multiple of this value (e.g.
	// 10 → fires at 10, 20, 30…). Crossing is detected statelessly from the previous
	// vs new count (new − delta = previous) at append time, so no per-scope ledger
	// is needed. A counter automation is session-scoped: it watches the counter of
	// the session whose activity crossed the boundary. Must be >= MinCounterInterval.
	// Ignored unless TriggerKind == TriggerCounter.
	CounterInterval int `json:"counterInterval,omitempty"`
	// TriggerTag is the session tag this rule watches (TriggerTag kind). A
	// finishing session whose Tags contain TriggerTag fires the rule.
	TriggerTag string `json:"triggerTag"`
	// BoardOp filters which card change fires a board automation: BoardOpMove
	// (default when empty), BoardOpCreate, BoardOpUpdate, BoardOpDelete, or
	// BoardOpAny. Ignored for tag automations.
	BoardOp string `json:"boardOp,omitempty"`
	// BoardFromState, when set, requires the card to have LEFT this column for a
	// board automation to fire (source-column filter). Empty = any source.
	BoardFromState string `json:"boardFromState,omitempty"`
	// BoardToState, when set, requires the card to have ENTERED this column for a
	// board automation to fire (target-column filter). Empty = any target.
	BoardToState string `json:"boardToState,omitempty"`
	// BoardPriority orders board automations that match the SAME card change.
	// Lower fires first; ties break on ID so the order is stable across restarts
	// (map iteration is not). Without it, two rules watching the same column fired
	// in arbitrary order and raced on the card. Default 0.
	BoardPriority int `json:"boardPriority,omitempty"`
	// BoardExclusive claims sole ownership of a card change: when an exclusive
	// automation matches, it is the ONLY one that fires for that event — every
	// lower-priority rule matching the same change is suppressed. This is the
	// "one owner per column" guarantee that replaces manually disabling the loser.
	// Among several exclusive matches the lowest BoardPriority wins.
	BoardExclusive bool `json:"boardExclusive,omitempty"`
	// BoardAction selects what a board automation does when it fires:
	// BoardActionSpawn (default, "" is treated the same) starts a session/flow —
	// the board drives execution; BoardActionArchive archives the card with no LLM
	// call (the cheap "done → archive" cleanup). Ignored for non-board triggers.
	BoardAction string `json:"boardAction,omitempty"`
	// BoardMoveToState is the explicit destination column for BoardActionMove.
	// It is not inferred from workspace column order.
	BoardMoveToState string `json:"boardMoveToState,omitempty"`
	// TargetAgentID is the agent that runs the spawned session. Optional when
	// FlowID is set (a flow-backed automation runs a flow instead of one agent).
	TargetAgentID string `json:"targetAgentId"`
	// FlowID, when set, makes this a flow-backed automation: on fire the rendered
	// prompt is run as that orchestration flow's input (RunFlowRecorded) instead
	// of spawning a session for TargetAgentID. A flow-backed automation is a
	// per-trigger dispatch — it does not self-loop via SpawnTags (flow sessions
	// carry no trigger tag and do not re-fire the rule), so the loop guardrails
	// (MaxIterations/Cooldown/ExpiresAt) still bound how often the trigger fires.
	FlowID string `json:"flowId,omitempty"`
	// SessionMode selects, for an AGENT-backed automation, whether each fire runs in
	// a fresh session (SessionModeSpawn) or continues one persistent per-automation
	// thread that carries prior turns forward (SessionModeContinue, history-aware).
	// Empty resolves per kind via EffectiveSessionMode (tag/board → spawn, token/
	// counter → continue) so old rows keep their original behavior. Ignored for
	// flow-backed automations (a flow always accumulates its own transcript).
	SessionMode string `json:"sessionMode,omitempty"`
	// PromptTemplate is the prompt delivered to the spawned session. Placeholders:
	// {{result}} (the finishing session's final reply), {{title}} (its title),
	// {{tag}} (TriggerTag), {{sessionId}} (the finishing session's id).
	PromptTemplate string `json:"promptTemplate"`
	// SpawnTags are the tags applied to the spawned session. When nil it defaults
	// to [TriggerTag], so the spawned session re-triggers this rule (the loop). Set
	// it to an empty non-nil slice ([]) or different tags to break/redirect the loop.
	// NO omitempty: [] is semantically distinct from nil (loop-break vs default) and
	// must survive the persist/reload round-trip — omitempty silently turned every
	// deliberate [] back into the self-looping default (found by the self-healing E2E).
	SpawnTags []string `json:"spawnTags"`
	// Enabled is the kill switch. A disabled automation never fires.
	Enabled bool `json:"enabled"`
	// Archived hides the rule from the default lists and stops it firing, without
	// deleting it: the curator's ceiling on destructive action (_Docs/77 R5). An
	// archived rule keeps its configuration, ledger and seed identity and can be
	// restored. Distinct from Enabled, which is the user's on/off switch.
	Archived bool `json:"archived,omitempty"`
	// MaxIterations caps the total number of fires. Valid range 1..MaxIterationsHardCap
	// (500); 0 ("unlimited") is rejected at every write path by ValidateMaxIterations
	// because a self-moving card could loop forever. Default 50 in the create paths.
	// Legacy rows persisted with <=0 are bounded at fire time by AbsoluteIterationBackstop.
	MaxIterations int `json:"maxIterations"`
	// CooldownSec is the minimum number of seconds between two fires of this rule
	// (0 = no cooldown). Bounds burst re-triggering.
	CooldownSec int `json:"cooldownSec"`
	// ExpiresAt is an optional end date (unix seconds). When > 0 the automation
	// stops firing once the time passes and is auto-disabled on the next attempt.
	// 0 means "no end date" (runs until maxIterations / manual disable).
	ExpiresAt int64 `json:"expiresAt,omitempty"`

	// --- runtime bookkeeping (updated by RecordAutomationFire) ---
	IterationCount int    `json:"iterationCount"`
	LastFiredAt    int64  `json:"lastFiredAt,omitempty"`
	LastSessionID  string `json:"lastSessionId,omitempty"`
	LastError      string `json:"lastError,omitempty"`
	// ActivityDispatchReceipts makes counter activity replay idempotent at the
	// automation bookkeeping boundary. Keyed by durable activity EventID.
	ActivityDispatchReceipts map[string]AutomationActivityDispatchOutcome `json:"activityDispatchReceipts,omitempty"`

	// CreatedBy is the ID of the agent that created this automation via a
	// self-management tool ("" = created by the user). Agents may only edit/delete
	// agent-created automations.
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`

	// Seed is the stable identity of a built-in default automation provisioned by
	// EnsureDefaultAutomations. Empty for user/agent-created automations. It
	// is used both to skip re-seeding an existing default and to record it in the
	// deletion ledger so a user-deleted default is never resurrected (mirrors
	// Flow.Seed).
	Seed string `json:"seed,omitempty"`
}
