package db

// Hook event names. These mirror Claude Code's hook contract so the same
// external hook scripts work against TionHarness's native tool loop AND its turn
// lifecycle. The first two fire AROUND a tool call (native path only, since
// claude-cli runs its own tool loop); the rest fire at TURN / SESSION lifecycle
// points and — because they only inject context or observe, never intercept a
// tool — apply to BOTH the native and claude-cli paths (SystemDynamic reaches
// the CLI too). This is the full Claude Code lifecycle event set.
const (
	HookPreToolUse       = "PreToolUse"       // before a native tool call
	HookPostToolUse      = "PostToolUse"      // after a native tool call
	HookUserPromptSubmit = "UserPromptSubmit" // before a turn, on each user prompt (may inject context / block)
	HookSessionStart     = "SessionStart"     // first turn of a session (source: startup|resume|clear)
	HookStop             = "Stop"             // main agent finished its turn (may block → force continuation)
	HookSubagentStop     = "SubagentStop"     // a run_subagent / worker finished
	HookPreCompact       = "PreCompact"       // before the rolling summary compaction runs (matcher: manual|auto)
	HookNotification     = "Notification"     // agent raised a notification (permission needed / idle)
	HookSessionEnd       = "SessionEnd"       // session archived / deleted (cleanup, fire-and-forget)
)

// LifecycleEvents is the ordered set of non-tool lifecycle events, for
// validation and UI enumeration.
var LifecycleEvents = []string{
	HookUserPromptSubmit, HookSessionStart, HookStop,
	HookSubagentStop, HookPreCompact, HookNotification, HookSessionEnd,
}

// IsLifecycleEvent reports whether e is a turn/session lifecycle event (i.e. not
// one of the two tool-call events).
func IsLifecycleEvent(e string) bool {
	for _, x := range LifecycleEvents {
		if x == e {
			return true
		}
	}
	return false
}

// ValidHookEvent reports whether e is any supported hook event (tool or lifecycle).
func ValidHookEvent(e string) bool {
	return e == HookPreToolUse || e == HookPostToolUse || IsLifecycleEvent(e)
}

// Hook is a user-defined interception point around a native tool call. When a
// tool call matches Matcher (a tool-name glob; "" = all tools) the configured
// external Command is run as a subprocess, receiving the call (and, for
// PostToolUse, the result) as JSON on stdin and returning a decision as JSON on
// stdout — the Claude Code hook contract. CreatedBy carries provenance ("" =
// user-defined, protected; non-empty = agent-created) like the rest of the
// self-management surface.
type Hook struct {
	ID         string `json:"id"`
	Event      string `json:"event"`   // PreToolUse | PostToolUse
	Matcher    string `json:"matcher"` // tool-name glob, "" = all tools
	Type       string `json:"type"`    // "command" (only kind for now)
	Command    string `json:"command"` // shell command for type=command
	TimeoutSec int    `json:"timeoutSec"`
	Enabled    bool   `json:"enabled"`
	CreatedBy  string `json:"createdBy,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
}
