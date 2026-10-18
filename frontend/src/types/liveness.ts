// Workspace liveness snapshot (internal/liveness, GET /api/workspace/liveness,
// _Docs/77 R2): the ONE server-side answer to "what is this workspace doing
// right now" — every session that is running, queued, or waiting on a human or
// on flow input — plus the spawn capacity left.

type LivenessState = 'running' | 'queued' | 'waiting_ask' | 'waiting_input'

interface LivenessEntry {
  sessionId: string
  state: LivenessState
  // Source in a compact form: "turn:user", "run", "turn:chat", "ask:SAK3",
  // "flow:RUN88".
  reason?: string
  // Unix seconds the entry entered its state; 0/absent when unknown.
  since?: number
  // Turns queued behind the running one (admission queue depth).
  waiting?: number
}

export interface LivenessCapacity {
  spawnActive: number
  spawnMax: number
  queueDepth: number
  queueMax: number
  busyTurns: number
  autonomyPaused: boolean
}

export interface LivenessSnapshot {
  entries: LivenessEntry[]
  capacity: LivenessCapacity
  at: number
}
