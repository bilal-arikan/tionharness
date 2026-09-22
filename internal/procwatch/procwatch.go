// Package procwatch is the process-wide ledger of NATIVE OS processes
// TionHarness spawns on an agent's behalf: shell tool calls, background shells,
// run_code interpreters, agentic CLI subprocesses, stdio MCP servers, user hooks
// and external tool CLIs.
//
// It exists because those processes are otherwise invisible. A turn that hangs
// on a `go test` nobody can see, a codex-cli that survived its session, an MCP
// server that respawns in a loop — each is a real process on the user's machine
// with no window, no log line and no way to stop it short of Task Manager. The
// ledger gives every spawn an id, an owner (session + agent) and a terminal
// status, so the UI can show what is running now and what recently finished.
//
// Scope: it RECORDS, it does not spawn. The caller still builds and runs its own
// exec.Cmd; it only brackets the run with Begin/Finish (see Handle). Two
// consequences follow. A spawn site that is not instrumented simply does not
// appear — absence from the ledger is not proof no process is running. And the
// ledger never keeps a process alive: an entry holds a cancel func at most, and
// dropping an entry does not touch the process.
//
// The package deliberately depends on nothing inside internal/: the deep layers
// that spawn (providers, mcp, tools) sit below the agent runtime and the HTTP
// API that reads the ledger, so it has to sit below all of them.
package procwatch

// Kind classifies what a process was spawned FOR. It is the coarse facet the
// panel filters on, not the executable name (which lives in Entry.Command).
type Kind string

const (
	// KindShell is a foreground Bash/PowerShell tool call: the turn is blocked
	// on it, and it dies with the tool call.
	KindShell Kind = "shell"
	// KindShellBackground is a detached background shell (run_in_background),
	// managed afterwards through the shell_manage tool.
	KindShellBackground Kind = "shell_background"
	// KindCode is a code-mode interpreter run (run_code / transform_data).
	KindCode Kind = "code"
	// KindProvider is an agentic CLI subprocess acting as the model transport
	// (claude-cli, codex-cli) — including its short-lived probes.
	KindProvider Kind = "provider"
	// KindMCP is a stdio MCP server process started for the workspace tool pool.
	KindMCP Kind = "mcp"
	// KindHook is a user-configured hook command run around a tool call or turn.
	KindHook Kind = "hook"
	// KindExternal is an external tool CLI run by TionHarness itself on the
	// agent's behalf: search-index builds, tool version probes, updates.
	KindExternal Kind = "external"
)

// Status is where a tracked process ended up. Exactly one terminal status is
// assigned, and only once (see Handle.Finish).
type Status string

const (
	// StatusRunning: started and not yet reaped.
	StatusRunning Status = "running"
	// StatusSucceeded: exited with code 0.
	StatusSucceeded Status = "succeeded"
	// StatusFailed: exited non-zero, or never started (ExitCode -1 with Error).
	StatusFailed Status = "failed"
	// StatusKilled: stopped on request — from the panel, the shell_manage tool,
	// or a cancelled turn.
	StatusKilled Status = "killed"
	// StatusTimedOut: the run context hit its deadline and the process was
	// terminated by it.
	StatusTimedOut Status = "timed_out"
)

// Owner is the agent-side identity a process belongs to. Every field is
// optional: a process started outside a session (an MCP server for the
// workspace pool, a startup probe) carries only what it knows.
type Owner struct {
	WorkspaceID string `json:"workspaceId,omitempty"`
	SessionID   string `json:"sessionId,omitempty"`
	AgentID     string `json:"agentId,omitempty"`
	AgentName   string `json:"agentName,omitempty"`
	// ParentSessionID is the coordinator/spawning session when this one is a
	// worker or sub-session, so the panel can group a fan-out under its root.
	ParentSessionID string `json:"parentSessionId,omitempty"`
}

// Entry is one tracked process, as the API and UI see it. Times are unix
// milliseconds; EndedAt is 0 while the process runs.
type Entry struct {
	ID      string `json:"id"`
	Kind    Kind   `json:"kind"`
	Label   string `json:"label,omitempty"`
	Command string `json:"command"`
	Dir     string `json:"dir,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Status  Status `json:"status"`
	// ExitCode is meaningful once Status is terminal. -1 means the process never
	// produced an exit code (it failed to start, or was killed before exiting).
	ExitCode  int    `json:"exitCode"`
	Error     string `json:"error,omitempty"`
	StartedAt int64  `json:"startedAt"`
	EndedAt   int64  `json:"endedAt,omitempty"`
	Owner     Owner  `json:"owner"`
	// OutputTail is the last few KB of combined stdout+stderr, captured only by
	// sites that already buffer output. Empty elsewhere — a process is tracked
	// whether or not its output is readable.
	OutputTail string `json:"outputTail,omitempty"`
	// Stoppable reports whether Registry.Stop can terminate this entry. False
	// once it is finished, and for sites that registered no cancel func.
	Stoppable bool `json:"stoppable"`
}

// Meta is what a spawn site declares about a process at Begin time.
type Meta struct {
	Kind  Kind
	Label string
	// Command is the human-readable command line. Sites that run a shell pass
	// the COMMAND the agent wrote, not the `bash -c` wrapper: the wrapper is
	// noise the panel would have to strip back off.
	Command string
	Dir     string
	// Owner overrides, field by field, whatever the context carries. A site that
	// knows its session/agent better than the context (a background shell
	// outliving its turn) sets them here.
	Owner Owner
	// Stop terminates the process. Usually the run context's cancel func.
	// nil marks the entry unstoppable from the panel.
	Stop func()
}
