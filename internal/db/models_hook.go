package db

// Hook event names. These mirror Claude Code's hook contract so the same
// external hook scripts work against SwarmGo's native tool loop.
const (
	HookPreToolUse  = "PreToolUse"
	HookPostToolUse = "PostToolUse"
)

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
