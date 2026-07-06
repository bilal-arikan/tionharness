import {
  MessageSquare,
  LayoutGrid,
  GitBranch,
  Clock,
  Activity,
  Sparkles,
  type LucideIcon,
} from 'lucide-react'

// Shared display metadata for the unified executions feed, used by both the
// ExecutionsPanel list and the SessionsOverview bulk table so the kind badge,
// filter tabs, id trimming and status pill stay identical across the two views.

// Per-kind display metadata: every execution path funnels into a Session tagged
// with a kind, so each view renders it uniformly with its own icon + label.
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
