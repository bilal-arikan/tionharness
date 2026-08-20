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

// Filter tabs (in display order). '' is "all".
export const FILTERS: { key: string; label: string }[] = [
  { key: '', label: 'Tümü' },
  { key: 'chat', label: 'Sohbet' },
  { key: 'task', label: 'Görev' },
  { key: 'flow', label: 'Akış' },
  { key: 'spawned', label: 'Spawn' },
  // Cron schedules are time-triggered automations, so the list filter unifies
  // both kinds under one "Otomasyon" chip (matching the management screen's
  // umbrella naming). The per-row icon still distinguishes them (Clock vs Zap).
  { key: 'automation', label: 'Otomasyon' },
  { key: 'insight', label: 'İçgörü' },
]

// Top-level sidebar tab (Aktif / Arşiv / Workers). 'active' is the default and
// is therefore omitted from the URL.
export type SessionListTab = 'active' | 'archived' | 'workers'

// Persisted kind filter — the sidebar lists every session kind, so the tab
// choice is worth remembering across reloads (same rationale as the width). The
// URL wins over this when it carries an explicit ?kind=.
export const KIND_FILTER_KEY = 'tionswarm.sessionKindFilter'

// normalizeSessionListTab coerces an untrusted value (URL segment, storage) to a
// live tab; anything unknown falls back to the default view.
export function normalizeSessionListTab(v: string | null | undefined): SessionListTab {
  return v === 'archived' || v === 'workers' ? v : 'active'
}

// normalizeKindFilter coerces an untrusted value to a known filter key ('' = all).
export function normalizeKindFilter(v: string | null | undefined): string {
  // A legacy ?kind=schedule deep-link now resolves to the unified automation chip.
  if (v === 'schedule') return 'automation'
  return v != null && FILTERS.some((f) => f.key === v) ? v : ''
}

export function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind || 'Diğer', icon: Activity }
}

// matchesKindFilter reports whether a session kind belongs under a filter tab.
// A legacy session persisted before the `kind` field existed carries '' and is
// treated as a plain chat, so it stays reachable under the "Sohbet" tab.
export function matchesKindFilter(kind: string, filter: string): boolean {
  // Insight scans open one machine-generated read-only session per run, which
  // would flood the default "Tümü" list and bury the conversations the user
  // actually came for (the old "chat listesini kirletme" problem). They are
  // therefore opt-in: reachable only through their own "İçgörü" chip.
  if (kind === 'insight') return filter === 'insight'
  if (!filter) return true
  if (filter === 'chat') return kind === '' || kind === 'chat'
  // The "Otomasyon" chip is an umbrella over both event-triggered automations
  // and time-triggered cron schedules (two distinct Session.Kind values).
  if (filter === 'automation') return kind === 'automation' || kind === 'schedule'
  return kind === filter
}

// shortId trims a session id to a compact, recognisable suffix for list rows
// (the full id is shown — and copyable — in the detail header).
export function shortId(id: string) {
  return id.length > 8 ? id.slice(-8) : id
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
