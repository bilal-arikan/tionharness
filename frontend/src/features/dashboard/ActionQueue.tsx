import { AlertTriangle, CalendarClock, GitBranch, KanbanSquare, MessageSquare } from 'lucide-react'
import type { ActionItem, ViewRef } from '@/types'
import { ViewButton } from '@/features/view/ViewButton'

// ActionQueue is the "what needs a human" list — the clickable sibling of the
// workspace projection's signal lines. Each row routes to the session, run, card
// or schedule it names, so the overview is a starting point for action, not just
// a readout. Danger rows (failed/stuck) sort ahead of warn rows (waiting/aging);
// the backend already ordered them, oldest-first within a severity.
//
// onNavigate receives the row's kind + id; the parent maps that to a screen. An
// empty queue says so in words — a blank panel would read as "failed to load".

const KIND_ICON = {
  session: MessageSquare,
  run: GitBranch,
  card: KanbanSquare,
  schedule: CalendarClock,
} as const

const KIND_LABEL = {
  session: 'Oturum',
  run: 'Koşu',
  card: 'Kart',
  schedule: 'Zamanlama',
} as const

// refFor maps an action to the projection it drills into (get_view). Every kind
// now resolves: a card points at the single-card board sub-view, a schedule at
// the schedule projection. Returns null only for a future kind with no view.
function refFor(a: ActionItem): ViewRef | null {
  switch (a.kind) {
    case 'session':
      return { kind: 'session', id: a.id }
    case 'run':
      return { kind: 'flowrun', id: a.id }
    case 'card':
      return { kind: 'board', id: 'board', sub: a.id }
    case 'schedule':
      return { kind: 'schedule', id: a.id }
    default:
      return null
  }
}

export function ActionQueue({
  items: rawItems,
  onNavigate,
}: {
  items: ActionItem[] | null
  onNavigate: (kind: ActionItem['kind'], id: string) => void
}) {
  // The backend now returns [] for a clean workspace, but an older build (or a
  // failed field) can still send null; treat both as empty rather than crashing.
  const items = rawItems ?? []
  return (
    <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
      <div className="mb-2 flex items-baseline gap-2">
        <span className="text-sm font-medium">🔔 Dikkat gereken</span>
        {items.length > 0 && (
          <span className="text-[11px] text-[var(--color-text-dim)]">{items.length} öğe</span>
        )}
      </div>

      {items.length === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">
          Şu an dikkat gerektiren bir şey yok — takılmış oturum, başarısız koşu/kart, bekleyen soru
          veya hatalı zamanlama yok. ✓
        </p>
      ) : (
        <ul className="flex flex-col gap-1">
          {items.map((a, i) => {
            const Icon = KIND_ICON[a.kind] ?? AlertTriangle
            const danger = a.severity === 'danger'
            const ref = refFor(a)
            return (
              <li
                key={`${a.kind}-${a.id}-${i}`}
                className="flex items-center gap-1 rounded-md border border-transparent pr-1 transition hover:border-[var(--color-border)] hover:bg-[var(--color-bg)]"
              >
                {/* Label click → the full screen that owns the entity. */}
                <button
                  type="button"
                  onClick={() => onNavigate(a.kind, a.id)}
                  className="flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left"
                >
                  <span
                    className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md"
                    style={{
                      color: danger ? 'var(--color-danger)' : '#eab308',
                      background: danger
                        ? 'color-mix(in srgb, var(--color-danger) 12%, transparent)'
                        : 'rgba(234,179,8,0.12)',
                    }}
                    title={KIND_LABEL[a.kind]}
                  >
                    <Icon size={14} />
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-baseline gap-1.5">
                      <span className="truncate text-xs font-medium">{a.label}</span>
                      <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
                        {KIND_LABEL[a.kind]} · {a.id}
                      </span>
                    </span>
                    {a.detail && (
                      <span className="block truncate text-[11px] text-[var(--color-text-dim)]">
                        {a.detail}
                      </span>
                    )}
                  </span>
                  {a.age && (
                    <span className="shrink-0 text-[11px] tabular-nums text-[var(--color-text-dim)]">
                      {a.age}
                    </span>
                  )}
                </button>
                {/* ◱ → peek the agent-identical projection in a side drawer,
                    without leaving the dashboard. */}
                {ref && <ViewButton target={ref} compact className="!px-1.5" />}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
