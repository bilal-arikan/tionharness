import {
  MessageSquare,
  LayoutGrid,
  GitBranch,
  Clock,
  Activity,
  Sparkles,
  Compass,
  Zap,
  Telescope,
  Bot,
  type LucideIcon,
} from 'lucide-react'

// Shared display metadata for session-kind rendering. Every execution path
// (chat / task / flow / schedule / spawn) funnels into a Session tagged with a
// kind, so the sessions sidebar, the bulk overview table and the agent activity
// rail all render the same icon, label, id trimming and status pill.

export const KIND_META: Record<string, { label: string; icon: LucideIcon }> = {
  chat: { label: 'Sohbet', icon: MessageSquare },
  task: { label: 'Görev', icon: LayoutGrid },
  flow: { label: 'Akış', icon: GitBranch },
  schedule: { label: 'Zamanlama', icon: Clock },
  // One persistent maintenance thread per token automation (see internal/agent/
  // automation_deliver.go); every fire continues it instead of spawning fresh.
  automation: { label: 'Otomasyon', icon: Zap },
  // A one-shot automation fire (session mode != continue) still opens its own
  // fresh session per fire, same shape as 'spawned' — just tagged distinctly so
  // it groups under "Otomasyon" here instead of "Spawn" (see kindChipKey below).
  'automation-run': { label: 'Otomasyon', icon: Zap },
  // A spawn-mode schedule fire's own fresh session (see internal/agent/
  // scheduler.go deliverSpawnedPrompt) — the "schedule" counterpart of
  // 'automation-run', same rationale.
  'schedule-run': { label: 'Otomasyon', icon: Clock },
  spawned: { label: 'Spawn', icon: Sparkles },
  subagent: { label: 'Subagent', icon: Bot },
  // A flow's coordinator node opens one of these per run (see internal/agent/
  // flow_coordinator.go); its workers hang off it like any coordinator's.
  'flow-coordinator': { label: 'Akış Koordinatörü', icon: Compass },
  // One read-only transcript per insight scan (see internal/agent/
  // insightsession.go); SourceID is the insight run id.
  insight: { label: 'İçgörü', icon: Telescope },
}

// Sidebar chips (multi-select, display order). The kind chips cover EVERY
// Session.Kind the backend can produce — including the ones the old filter tabs
// left out (flow-coordinator, inbox) plus an "Diğer" catch-all for a kind this
// build does not know — so nothing can be invisible in the list. The four
// trailing scope chips widen the list instead of narrowing it by kind: sessions
// with a live turn of their own, sessions idle but waiting on live workers below
// them, worker sessions (coordinator-spawned) and archived ones. Every chip is
// selected by default, so the sidebar shows everything and the user unticks what
// they don't want to see.
export const WORKER_CHIP = 'worker'
export const SUBAGENT_CHIP = 'subagent'
export const ARCHIVED_CHIP = 'archived'
export const OTHER_CHIP = 'other'
// The two live-state scope chips. `running` is the session's OWN turn streaming;
// `awaiting-workers` is the coordinator shape: its own turn is idle but at least
// one worker below it is live. They are mutually exclusive by construction (see
// sessionLiveScope) so a row never needs both chips on to stay visible.
export const RUNNING_CHIP = 'running'
export const AWAITING_WORKERS_CHIP = 'awaiting-workers'

export const SESSION_CHIPS: { key: string; label: string }[] = [
  { key: 'chat', label: 'Sohbet' },
  { key: 'task', label: 'Görev' },
  { key: 'flow', label: 'Akış' },
  { key: 'spawned', label: 'Spawn' },
  { key: SUBAGENT_CHIP, label: 'Subagent' },
  // Cron schedules are time-triggered automations, so the chip unifies both
  // kinds under one "Otomasyon" label (matching the management screen's umbrella
  // naming). The per-row icon still distinguishes them (Clock vs Zap).
  { key: 'automation', label: 'Otomasyon' },
  { key: 'insight', label: 'İçgörü' },
  { key: 'flow-coordinator', label: 'Akış Koord.' },
  { key: 'inbox', label: 'Inbox' },
  { key: OTHER_CHIP, label: 'Diğer' },
  { key: RUNNING_CHIP, label: 'Çalışan' },
  { key: AWAITING_WORKERS_CHIP, label: 'Worker Bekleyen' },
  { key: WORKER_CHIP, label: 'Worker' },
  { key: ARCHIVED_CHIP, label: 'Arşiv' },
]

export const ALL_SESSION_CHIPS: string[] = SESSION_CHIPS.map((c) => c.key)

// Persisted chip state — the sidebar lists every session kind, so the choice is
// worth remembering across reloads (same rationale as the width). Storage holds
// the UNTICKED chips, not the ticked ones: that way a chip added by a later
// build (a new session kind) starts visible instead of silently hiding rows for
// everyone who already has a saved selection.
export const SESSION_CHIPS_OFF_KEY = 'tionharness.sessionChipsOff'

// normalizeChipsOff coerces an untrusted value (storage) to a list of unticked
// chip keys; anything unparseable means "nothing unticked" (all chips on).
export function normalizeChipsOff(raw: string | null | undefined): string[] {
  if (!raw) return []
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []
  return parsed.filter((k): k is string => typeof k === 'string' && ALL_SESSION_CHIPS.includes(k))
}

// Modifier held while clicking a chip. 'toggle' is the plain click (flip just
// this chip), 'solo' (Ctrl) leaves only the clicked chip on, 'invert' (Shift)
// flips every OTHER chip and leaves the clicked one as it was.
export type ChipClickMode = 'toggle' | 'solo' | 'invert'

// nextChipsOff computes the new unticked-chip list for a click. Pure so the
// modifier semantics can be tested without a DOM.
export function nextChipsOff(chipsOff: string[], key: string, mode: ChipClickMode): string[] {
  const off = new Set(chipsOff)
  switch (mode) {
    case 'solo':
      return ALL_SESSION_CHIPS.filter((k) => k !== key)
    case 'invert':
      return ALL_SESSION_CHIPS.filter((k) => (k === key ? off.has(k) : !off.has(k)))
    default:
      return off.has(key) ? chipsOff.filter((k) => k !== key) : [...chipsOff, key]
  }
}

// kindChipKey maps a Session.Kind to the chip that owns it. Every kind lands on
// a chip: an unrecognised one falls to the "Diğer" catch-all rather than
// becoming unfilterable.
export function kindChipKey(kind: string): string {
  if (kind === '' || kind === 'chat') return 'chat'
  // The "Otomasyon" chip is an umbrella over event-triggered automations
  // (persistent thread 'automation' and their one-shot 'automation-run' fires)
  // and time-triggered cron schedules (persistent 'schedule' and their one-shot
  // 'schedule-run' fires).
  if (
    kind === 'automation' ||
    kind === 'automation-run' ||
    kind === 'schedule' ||
    kind === 'schedule-run'
  )
    return 'automation'
  return SESSION_CHIPS.some((c) => c.key === kind) ? kind : OTHER_CHIP
}

// sessionChipKey classifies new sessions by stable metadata first. Legacy
// sessions retain their old kind-based placement when those fields are absent.
export function sessionChipKey(s: {
  kind: string
  category?: string
  executionType?: string
}): string {
  if (s.category === 'subagent') return SUBAGENT_CHIP
  if (s.category && ALL_SESSION_CHIPS.includes(s.category)) return s.category
  if (s.executionType === 'subagent') return SUBAGENT_CHIP
  if (s.executionType && ALL_SESSION_CHIPS.includes(s.executionType)) return s.executionType
  return kindChipKey(s.kind)
}

// sessionLiveScope classifies a session on the live-activity axis, which is
// independent of its kind. `running` wins over `awaiting-workers`: a coordinator
// that is itself streaming is reported as running, so the two scopes never
// overlap and a row is filtered by exactly one of them.
export type SessionLiveScope = typeof RUNNING_CHIP | typeof AWAITING_WORKERS_CHIP | null

export function sessionLiveScope(s: {
  isRunning: boolean
  liveWorkerCount: number
}): SessionLiveScope {
  if (s.isRunning) return RUNNING_CHIP
  if (s.liveWorkerCount > 0) return AWAITING_WORKERS_CHIP
  return null
}

// sessionMatchesChips reports whether a session survives the sidebar's chip
// selection: its kind chip must be on, and a worker/archived/live session also
// needs its scope chip on. The live fields are optional so callers that have no
// runtime snapshot (tests, the bulk overview table) keep the previous behaviour
// of ignoring the live axis entirely.
export function sessionMatchesChips(
  s: {
    kind: string
    category?: string
    executionType?: string
    isWorker: boolean
    isArchived: boolean
    isRunning?: boolean
    liveWorkerCount?: number
  },
  selected: ReadonlySet<string>,
): boolean {
  if (s.isArchived && !selected.has(ARCHIVED_CHIP)) return false
  if (s.isWorker && !selected.has(WORKER_CHIP)) return false
  const live = sessionLiveScope({
    isRunning: s.isRunning ?? false,
    liveWorkerCount: s.liveWorkerCount ?? 0,
  })
  if (live && !selected.has(live)) return false
  return selected.has(sessionChipKey(s))
}

export function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind || 'Diğer', icon: Activity }
}
