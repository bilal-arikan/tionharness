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
export const SIGNAL_EXPLORER = 'explorer' // Özet Haritası drill-down (open branches)
export const SIGNAL_ACTIVITY = 'activity' // useActivity's per-view busy flags
export const SIGNAL_EXECUTIONS = 'executions' // GET /api/executions consumers (session runtime map)
export const SIGNAL_AGENTS = 'agents' // AgentsView
export const SIGNAL_FLOWS = 'flows' // FlowsPanel
export const SIGNAL_SCHEDULES = 'schedules' // SchedulesPanel
export const SIGNAL_AUTOMATIONS = 'automations' // AutomationBoard
export const SIGNAL_ARTIFACTS = 'artifacts' // Artifact views
export const SIGNAL_WORKSPACE_ACTIVITY = 'workspace-activity' // cross-workspace live-run flags (switcher pulse)

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
      return [SIGNAL_BOARD, SIGNAL_NETWORK, SIGNAL_EXPLORER, SIGNAL_EXECUTIONS]
    case 'session':
      // Session CRUD (create / delete / archive / state / title /
      // workdir / agent / role / tags / spawn / handoff / rewind / feedback).
      // App.tsx's existing onEventRef branch already calls refreshSessions()
      // for the chat sidebar; this signal additionally nudges the executions
      // feed (each execution is a session, so a new / deleted / state-changed
      // session can change the list).
      return [SIGNAL_EXECUTIONS, SIGNAL_EXPLORER]
    case 'chat':
      // Chat turn started / ended. App.tsx already handles wake phases +
      // active transcript reload. Panel-wise: executions list may flip
      // (running → done) and network may flip the agent's running glow.
      return [SIGNAL_EXECUTIONS, SIGNAL_NETWORK, SIGNAL_EXPLORER, SIGNAL_ACTIVITY]
    case 'flow':
      return [SIGNAL_FLOWS, SIGNAL_NETWORK, SIGNAL_EXPLORER, SIGNAL_EXECUTIONS, SIGNAL_ACTIVITY]
    case 'schedule':
      return [SIGNAL_SCHEDULES, SIGNAL_NETWORK, SIGNAL_EXPLORER, SIGNAL_EXECUTIONS, SIGNAL_ACTIVITY]
    case 'automation':
      return [SIGNAL_AUTOMATIONS]
    case 'insight':
      // Scan started/finished: re-poll /api/activity (nav-rail İçgörü dot) and
      // /api/workspaces/activity (switcher pulse) so both update without lag.
      return [SIGNAL_ACTIVITY, SIGNAL_WORKSPACE_ACTIVITY]
    case 'spawned':
      return [SIGNAL_EXECUTIONS, SIGNAL_NETWORK, SIGNAL_EXPLORER, SIGNAL_ACTIVITY]
    case 'worker':
      return [SIGNAL_EXECUTIONS, SIGNAL_SCHEDULES, SIGNAL_NETWORK, SIGNAL_EXPLORER, SIGNAL_ACTIVITY]
    case 'task':
      // Task run lifecycle (different from board CRUD). The network cares too:
      // agent nodes ARE running sessions, so a task run starting or finishing
      // adds/removes a node rather than just re-tinting one.
      return [SIGNAL_BOARD, SIGNAL_EXECUTIONS, SIGNAL_ACTIVITY, SIGNAL_NETWORK, SIGNAL_EXPLORER]
    case 'agent':
    case 'agent-model-changed':
      return [SIGNAL_AGENTS, SIGNAL_NETWORK, SIGNAL_EXPLORER]
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

// Run-lifecycle event types: those whose start/end flips a workspace's live-run
// state (a turn / task / flow beginning or finishing). Used to instantly refresh
// the cross-workspace switcher pulse.
const RUN_LIFECYCLE_TYPES = new Set(['chat', 'flow', 'schedule', 'spawned', 'worker', 'task'])

// bumpWorkspaceActivityForEvent nudges the cross-workspace live-run signal
// (useWorkspaceActivity) when a run started or ended — for ANY workspace,
// including non-active ones. It is intentionally separate from
// bumpSignalsForEvent: the latter's executions/activity keys are active-workspace
// scoped, so bumping them for a NON-active workspace's event would only trigger a
// wasted refetch of unchanged active-workspace data. This dedicated key drives the
// one consumer that IS cross-workspace, so a run elsewhere lights the switcher
// pulse instantly instead of waiting for the next poll tick.
export function bumpWorkspaceActivityForEvent(e: AppEvent): void {
  if (RUN_LIFECYCLE_TYPES.has(e.type)) debouncedBump(SIGNAL_WORKSPACE_ACTIVITY)
}
