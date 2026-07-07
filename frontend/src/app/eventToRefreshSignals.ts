// Maps a global SSE AppEvent to the set of panel-level refresh signal keys
// that should re-fetch in response. App.tsx's central onEventRef handler calls
// this for every event and bumps the returned keys (debounced) so subscribers
// in useRefreshTrigger re-render and re-fetch.
//
// Keep this list narrow: every bumped key forces every subscribed panel to
// re-fetch, so a noisy event mapped to many keys creates a stampede. The
// set of keys here also defines the public surface between event types and
// the panel subscription system.
import type { AppEvent } from '@/types'
import { bumpSignal } from '@/shared/lib/refreshSignals'

// Signal key names — mirrored on the consumer side via useRefreshTrigger('key').
// Centralized here so a rename / add / remove is a single-file change.
export const SIGNAL_BOARD = 'board' // task CRUD + board column changes
export const SIGNAL_NETWORK = 'network' // collaboration graph
export const SIGNAL_ACTIVITY = 'activity' // useActivity's per-view busy flags
export const SIGNAL_EXECUTIONS = 'executions' // ExecutionsPanel
export const SIGNAL_AGENTS = 'agents' // AgentsView
export const SIGNAL_FLOWS = 'flows' // FlowsPanel
export const SIGNAL_SCHEDULES = 'schedules' // SchedulesPanel
export const SIGNAL_ARTIFACTS = 'artifacts' // Artifact views

// signalsForEvent returns the set of signal keys that should bump for the
// given event. App.tsx's onEventRef walks this set and calls debouncedBump.
//
// The mapping is intentionally many-to-one (one event can bump several panels):
//   - a task create / update / delete affects board, network, AND executions
//     (the task is a board card, a graph node, AND an execution row).
//   - a chat end affects executions (status flips running → done) and
//     network (running flag may flip).
//
// An empty array means "no panel needs to re-fetch" (e.g. a `navigate` event
// for a workspace the user isn't viewing, a `settings` change, etc.).
export function signalsForEvent(e: AppEvent): string[] {
  switch (e.type) {
    case 'board':
      // Task CRUD + column changes (op="create"/"update"/"move"/"delete"/
      // "columns_changed"). All three consumers re-fetch; useActivity doesn't
      // care about individual task mutations.
      return [SIGNAL_BOARD, SIGNAL_NETWORK, SIGNAL_EXECUTIONS]
    case 'session':
      // Session CRUD (create / delete / archive / state / title / goal /
      // workdir / agent / role / tags / spawn / handoff / rewind / feedback).
      // App.tsx's existing onEventRef branch already calls refreshSessions()
      // for the chat sidebar; this signal additionally nudges the executions
      // feed (each execution is a session, so a new / deleted / state-changed
      // session can change the list).
      return [SIGNAL_EXECUTIONS]
    case 'chat':
      // Chat turn started / ended. App.tsx already handles wake phases +
      // active transcript reload. Panel-wise: executions list may flip
      // (running → done) and network may flip the agent's running glow.
      return [SIGNAL_EXECUTIONS, SIGNAL_NETWORK, SIGNAL_ACTIVITY]
    case 'flow':
      return [SIGNAL_FLOWS, SIGNAL_NETWORK, SIGNAL_EXECUTIONS, SIGNAL_ACTIVITY]
    case 'schedule':
      return [
        SIGNAL_SCHEDULES,
        SIGNAL_NETWORK,
        SIGNAL_EXECUTIONS,
        SIGNAL_ACTIVITY,
      ]
    case 'spawned':
      return [SIGNAL_EXECUTIONS, SIGNAL_NETWORK, SIGNAL_ACTIVITY]
    case 'worker':
      return [SIGNAL_EXECUTIONS, SIGNAL_SCHEDULES, SIGNAL_NETWORK, SIGNAL_ACTIVITY]
    case 'task':
      // Task run lifecycle (different from board CRUD). The same set of
      // consumers is interested.
      return [SIGNAL_BOARD, SIGNAL_EXECUTIONS, SIGNAL_ACTIVITY]
    case 'agent':
      return [SIGNAL_AGENTS, SIGNAL_NETWORK]
    case 'artifact':
      return [SIGNAL_ARTIFACTS]
    default:
      // settings / workspaces / navigate / session_step (routed through the
      // separate `step` SSE channel) don't drive panel refreshes.
      return []
  }
}

// Per-key debounce: a burst of events (e.g. five task CRUDs in 100ms) collapses
// into a single bump, so subscribers re-fetch once. 200ms is below the human
// "instant" threshold and short enough to feel live in cross-window tests.
const DEBOUNCE_MS = 200
const pending = new Map<string, number>()

export function debouncedBump(key: string): void {
  if (pending.has(key)) return // already scheduled
  const t = window.setTimeout(() => {
    pending.delete(key)
    bumpSignal(key)
  }, DEBOUNCE_MS)
  pending.set(key, t)
}

// bumpSignalsForEvent is the one entry point App.tsx calls. It schedules
// per-key debounced bumps for everything the event implies.
export function bumpSignalsForEvent(e: AppEvent): void {
  const keys = signalsForEvent(e)
  for (const k of keys) debouncedBump(k)
}
