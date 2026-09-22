// Native OS processes TionHarness spawned on the agents' behalf, as tracked by
// internal/procwatch and served by GET /api/workspace/processes.

// ProcessKind names the call site that started the process.
export type ProcessKind =
  | 'shell' // foreground Bash/PowerShell tool call
  | 'shell_background' // detached background shell (run_in_background)
  | 'code' // run_code / transform_data interpreter run
  | 'provider' // agentic CLI subprocess (claude-cli, codex-cli) + its probes
  | 'mcp' // stdio MCP server for the workspace tool pool
  | 'hook' // user-configured hook command
  | 'external' // external tool CLI run by TionHarness itself

// ProcessStatus is 'running' until the process is reaped; the rest are terminal.
export type ProcessStatus = 'running' | 'succeeded' | 'failed' | 'killed' | 'timed_out'

// ProcessOwner is the agent-side identity a process belongs to. Every field is
// optional: a process started outside a session (an MCP server for the workspace
// pool, a startup probe) carries only what it knows.
export interface ProcessOwner {
  workspaceId?: string
  sessionId?: string
  agentId?: string
  agentName?: string
  // The coordinator/spawning session when this one is a worker or sub-session.
  parentSessionId?: string
}

// ProcessEntry is one tracked process. Times are unix milliseconds; endedAt is
// absent while the process runs.
export interface ProcessEntry {
  id: string
  kind: ProcessKind
  label?: string
  command: string
  dir?: string
  pid?: number
  status: ProcessStatus
  // Meaningful once status is terminal. -1 means the process never produced an
  // exit code (it failed to start, or was killed before exiting).
  exitCode: number
  error?: string
  startedAt: number // unix milliseconds
  endedAt?: number // unix milliseconds, absent while running
  owner: ProcessOwner
  // Last few KB of combined stdout+stderr, captured only by sites that already
  // buffer output. Absent elsewhere — a process is tracked whether or not its
  // output is readable.
  outputTail?: string
  // Whether the backend can terminate this entry on request.
  stoppable: boolean
}

// StopProcessResult reports which of the outcomes POST .../stop produced:
// stopped=true (the signal went out) or stopped=false with a reason (already
// finished, or registered without a stop path).
export interface StopProcessResult {
  id: string
  stopped: boolean
  reason?: string
}
