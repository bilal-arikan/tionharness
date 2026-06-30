package tools

// Risk classifies a tool by how dangerous it is to run. It drives the agent
// permission gate: read-only mode blocks anything above RiskRead, and "ask" mode
// prompts for approval before running RiskWrite / RiskExec tools.
type Risk string

const (
	RiskRead  Risk = "read"  // no host side effects (reads, in-app, interaction) — always allowed
	RiskWrite Risk = "write" // mutates files / external state — needs approval in "ask"
	RiskExec  Risk = "exec"  // arbitrary command execution — needs approval in "ask"
)

// toolRisk maps known built-in tools to their risk tier. Interaction and in-app
// tools (ask_user, todo_write, artifacts) are RiskRead: they have no host side
// effects, so Explore (read-only) agents can still use them.
var toolRisk = map[string]Risk{
	// SwarmGo-specific built-ins (no claude-cli analog).
	"memory_recall":        RiskRead,
	"ask_user":             RiskRead,
	"request_confirmation": RiskRead,
	"todo_write":           RiskRead,
	"create_artifact":      RiskRead,
	"update_artifact":      RiskRead,
	"list_artifacts":       RiskRead,
	"read_artifact":        RiskRead,

	// Core file/shell built-ins share claude-cli's tool names (Read/Write/Edit/
	// LS/Glob/Grep/Bash), so native and CLI agents — and the CLI permission-prompt
	// tool, which reports the CLI's own names — classify against ONE set.
	"Read":         RiskRead,
	"Glob":         RiskRead,
	"Grep":         RiskRead,
	"LS":           RiskRead,
	"NotebookRead": RiskRead,
	"WebFetch":     RiskRead,
	"WebSearch":    RiskRead,
	"TodoWrite":    RiskRead,
	// EnterPlanMode just flips the CLI into read-only planning state — no host side
	// effects, so it auto-allows if it ever reaches the permission gate. ExitPlanMode
	// is handled specially (plan-approval card), not via this table.
	"EnterPlanMode": RiskRead,
	"Edit":         RiskWrite,
	"Write":        RiskWrite,
	"MultiEdit":    RiskWrite,
	"NotebookEdit": RiskWrite,
	"Bash":         RiskExec,
	"PowerShell":   RiskExec, // Windows-native shell sibling of Bash

	// transform_data runs an arbitrary host script (python/node/bun) in a
	// subprocess. The env is stripped of secrets and it is time-bounded, but it is
	// still host code execution — same risk tier as the shell.
	"transform_data": RiskExec,
}

// Classify returns the risk tier for a tool name. Unknown tools — including
// namespaced MCP tools whose behaviour SwarmGo cannot inspect — default to
// RiskWrite, so they require approval in "ask" mode and are blocked in
// "read-only".
func Classify(name string) Risk {
	if r, ok := toolRisk[name]; ok {
		return r
	}
	return RiskWrite
}
