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
	// Reads & harmless helpers.
	"read_file":            RiskRead,
	"list_dir":             RiskRead,
	"glob":                 RiskRead,
	"grep":                 RiskRead,
	"get_current_time":     RiskRead,
	"memory_recall":        RiskRead,
	"http_get":             RiskRead,
	"ask_user":             RiskRead,
	"request_confirmation": RiskRead,
	"todo_write":           RiskRead,
	"create_artifact":      RiskRead,
	"update_artifact":      RiskRead,
	// Filesystem mutations.
	"write_file": RiskWrite,
	"edit_file":  RiskWrite,
	// Arbitrary execution.
	"shell": RiskExec,
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
