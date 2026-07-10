import {
  MessageSquare,
  LayoutGrid,
  GitBranch,
  Clock,
  Activity,
  Sparkles,
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
  spawned: { label: 'Spawn', icon: Sparkles },
}

// Filter tabs (in display order). '' is "all".
export const FILTERS: { key: string; label: string }[] = [
  { key: '', label: 'Tümü' },
  { key: 'chat', label: 'Sohbet' },
  { key: 'task', label: 'Görev' },
  { key: 'flow', label: 'Akış' },
  { key: 'spawned', label: 'Spawn' },
  { key: 'schedule', label: 'Zamanlama' },
]

export function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind || 'Diğer', icon: Activity }
}

// matchesKindFilter reports whether a session kind belongs under a filter tab.
// A legacy session persisted before the `kind` field existed carries '' and is
// treated as a plain chat, so it stays reachable under the "Sohbet" tab.
export function matchesKindFilter(kind: string, filter: string): boolean {
  if (!filter) return true
  if (filter === 'chat') return kind === '' || kind === 'chat'
  return kind === filter
}

// shortId trims a session id to a compact, recognisable suffix for list rows
// (the full id is shown — and copyable — in the detail header).
export function shortId(id: string) {
  return id.length > 8 ? id.slice(-8) : id
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
