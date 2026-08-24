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
  type LucideIcon,
} from 'lucide-react'
import { Badge } from '@/shared/components'
import { runStateMeta } from './runStateMeta'

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
  spawned: { label: 'Spawn', icon: Sparkles },
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
// build does not know — so nothing can be invisible in the list. The two
// trailing scope chips widen the list instead of narrowing it by kind: worker
// sessions (coordinator-spawned) and archived ones. Every chip is selected by
// default, so the sidebar shows everything and the user unticks what they don't
// want to see.
export const WORKER_CHIP = 'worker'
export const ARCHIVED_CHIP = 'archived'
export const OTHER_CHIP = 'other'

export const SESSION_CHIPS: { key: string; label: string }[] = [
  { key: 'chat', label: 'Sohbet' },
  { key: 'task', label: 'Görev' },
  { key: 'flow', label: 'Akış' },
  { key: 'spawned', label: 'Spawn' },
  // Cron schedules are time-triggered automations, so the chip unifies both
  // kinds under one "Otomasyon" label (matching the management screen's umbrella
  // naming). The per-row icon still distinguishes them (Clock vs Zap).
  { key: 'automation', label: 'Otomasyon' },
  { key: 'insight', label: 'İçgörü' },
  { key: 'flow-coordinator', label: 'Akış Koord.' },
  { key: 'inbox', label: 'Inbox' },
  { key: OTHER_CHIP, label: 'Diğer' },
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

// kindChipKey maps a Session.Kind to the chip that owns it. Every kind lands on
// a chip: an unrecognised one falls to the "Diğer" catch-all rather than
// becoming unfilterable.
export function kindChipKey(kind: string): string {
  if (kind === '' || kind === 'chat') return 'chat'
  // The "Otomasyon" chip is an umbrella over event-triggered automations and
  // time-triggered cron schedules (two distinct Session.Kind values).
  if (kind === 'automation' || kind === 'schedule') return 'automation'
  return SESSION_CHIPS.some((c) => c.key === kind) ? kind : OTHER_CHIP
}

// sessionMatchesChips reports whether a session survives the sidebar's chip
// selection: its kind chip must be on, and a worker/archived session also needs
// its scope chip on.
export function sessionMatchesChips(
  s: { kind: string; isWorker: boolean; isArchived: boolean },
  selected: ReadonlySet<string>,
): boolean {
  if (s.isArchived && !selected.has(ARCHIVED_CHIP)) return false
  if (s.isWorker && !selected.has(WORKER_CHIP)) return false
  return selected.has(kindChipKey(s.kind))
}

export function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind || 'Diğer', icon: Activity }
}

// RunStateBadge renders a session's last run outcome, or nothing when the session
// has never run a background turn (or carries a value this UI does not know). The
// label/tone table and the never-ran rule live in ./runStateMeta.
export function RunStateBadge({ runState }: { runState?: string }) {
  const meta = runStateMeta(runState)
  if (!meta) return null
  return (
    <Badge tone={meta.tone} className="shrink-0">
      <span title={meta.title}>{meta.label}</span>
    </Badge>
  )
}

// StatusPill shows a finished run's pass/fail outcome (task/flow kinds).
export function StatusPill({ status }: { status: string }) {
  if (status !== 'success' && status !== 'failure') return null
  const ok = status === 'success'
  return (
    <span
      className="rounded px-1.5 py-0.5 text-[10px] font-semibold"
      style={{
        color: ok ? 'var(--color-success)' : 'var(--color-danger)',
        backgroundColor: ok
          ? 'color-mix(in srgb, var(--color-success) 14%, transparent)'
          : 'color-mix(in srgb, var(--color-danger) 14%, transparent)',
      }}
    >
      {ok ? 'başarılı' : 'hata'}
    </span>
  )
}
