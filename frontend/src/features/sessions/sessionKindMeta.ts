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
import { i18next } from '@/i18n'

// Shared display metadata for session-kind rendering. Every execution path
// (chat / task / flow / schedule / spawn) funnels into a Session tagged with a
// kind, so the sessions sidebar, the bulk overview table and the agent activity
// rail all render the same icon, label, id trimming and status pill.

const KIND_META: Record<string, { labelKey: string; icon: LucideIcon }> = {
  chat: { labelKey: 'kind.chat', icon: MessageSquare },
  task: { labelKey: 'kind.task', icon: LayoutGrid },
  flow: { labelKey: 'kind.flow', icon: GitBranch },
  schedule: { labelKey: 'kind.schedule', icon: Clock },
  // One persistent maintenance thread per token automation (see internal/agent/
  // automation_deliver.go); every fire continues it instead of spawning fresh.
  automation: { labelKey: 'kind.automation', icon: Zap },
  // A one-shot automation fire (session mode != continue) still opens its own
  // fresh session per fire, same shape as 'spawned' — just tagged distinctly so
  // it groups under "Otomasyon" here instead of "Spawn" (see kindChipKey below).
  'automation-run': { labelKey: 'kind.automation', icon: Zap },
  // A spawn-mode schedule fire's own fresh session (see internal/agent/
  // scheduler.go deliverSpawnedPrompt) — the "schedule" counterpart of
  // 'automation-run', same rationale.
  'schedule-run': { labelKey: 'kind.automation', icon: Clock },
  spawned: { labelKey: 'kind.spawned', icon: Sparkles },
  subagent: { labelKey: 'kind.subagent', icon: Bot },
  // A flow's coordinator node opens one of these per run (see internal/agent/
  // flow_coordinator.go); its workers hang off it like any coordinator's.
  'flow-coordinator': { labelKey: 'kind.flowCoordinator', icon: Compass },
  // One read-only transcript per insight scan (see internal/agent/
  // insightsession.go); SourceID is the insight run id.
  insight: { labelKey: 'kind.insight', icon: Telescope },
}

// Session filter chips (multi-select, display order). The kind chips cover EVERY
// Session.Kind the backend can produce — including the ones the old filter tabs
// left out (flow-coordinator, inbox) plus an "Diğer" catch-all for a kind this
// build does not know — so nothing can be invisible in the list. The four
// trailing scope chips widen the list instead of narrowing it by kind: sessions
// with a live turn of their own, sessions idle but waiting on live workers below
// them, worker sessions (coordinator-spawned) and archived ones. Every chip is
// selected by default. The chat list presents archived sessions through its
// separate archive view, while Rota still exposes the archived scope as a chip.
export const WORKER_CHIP = 'worker'
export const SUBAGENT_CHIP = 'subagent'
export const ARCHIVED_CHIP = 'archived'
const OTHER_CHIP = 'other'
// The two live-state scope chips. `running` is the session's OWN turn streaming;
// `awaiting-workers` is the coordinator shape: its own turn is idle but at least
// one worker below it is live. They are mutually exclusive by construction (see
// sessionLiveScope) so a row never needs both chips on to stay visible.
export const RUNNING_CHIP = 'running'
export const AWAITING_WORKERS_CHIP = 'awaiting-workers'

function chip(key: string, labelKey: string): { key: string; readonly label: string } {
  return {
    key,
    get label() {
      return i18next.t(labelKey, { ns: 'sessions' })
    },
  }
}

export const SESSION_CHIPS: { key: string; readonly label: string }[] = [
  chip('chat', 'chip.chat'),
  chip('task', 'chip.task'),
  chip('flow', 'chip.flow'),
  chip('spawned', 'chip.spawned'),
  chip(SUBAGENT_CHIP, 'chip.subagent'),
  // Cron schedules are time-triggered automations, so the chip unifies both
  // kinds under one "Otomasyon" label (matching the management screen's umbrella
  // naming). The per-row icon still distinguishes them (Clock vs Zap).
  chip('automation', 'chip.automation'),
  chip('insight', 'chip.insight'),
  chip('flow-coordinator', 'chip.flowCoordinator'),
  // Legacy: peer messages now land in the recipient's ordinary chat thread
  // (TSK507), so nothing creates an 'inbox' session any more. The chip stays so
  // sessions created before that change remain filterable rather than falling
  // into "Diğer".
  chip('inbox', 'chip.inbox'),
  chip(OTHER_CHIP, 'chip.other'),
  chip(RUNNING_CHIP, 'chip.running'),
  chip(AWAITING_WORKERS_CHIP, 'chip.awaitingWorkers'),
  chip(WORKER_CHIP, 'chip.worker'),
  chip(ARCHIVED_CHIP, 'chip.archived'),
]

export const ALL_SESSION_CHIPS: string[] = SESSION_CHIPS.map((c) => c.key)
// The chat list uses a separate archive view. Rota still exposes the archived
// scope as a chip, so its filter vocabulary remains unchanged.
export const SESSION_LIST_CHIPS = SESSION_CHIPS.filter((chip) => chip.key !== ARCHIVED_CHIP)

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
function kindChipKey(kind: string): string {
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
  const meta = KIND_META[kind]
  if (!meta) {
    return { label: kind || i18next.t('kind.other', { ns: 'sessions' }), icon: Activity }
  }
  return { label: i18next.t(meta.labelKey, { ns: 'sessions' }), icon: meta.icon }
}
