import { Archive, EyeOff, NotebookPen, Repeat } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge, EmptyState, LoadingState } from '@/shared/components'
import { SELECTED_ITEM_CLS } from '@/shared/components/SidebarChrome'
import { relativeTime } from '@/shared/lib/time'
import type { Note } from '@/types'
import { ConfidenceBadge, KindBadge, ScopeBadge } from './noteBadges'
import { isRetired } from './notesHelpers'

interface Props {
  notes: Note[]
  // Search snippets keyed by note id (only when the list came from /search).
  snippets?: Record<string, string>
  activeId: string | null
  onSelect: (id: string) => void
  loading: boolean
  // True when any filter / search narrows the list (drives the empty copy).
  filtered: boolean
}

// NoteList is the Notes tab's left column: one row per note with its kind,
// confidence and reach badges, the age, how often a machine-written note has
// recurred, who wrote it and the archived / superseded / private markers.
export function NoteList({ notes, snippets, activeId, onSelect, loading, filtered }: Props) {
  const { t } = useTranslation('notes')
  if (loading && notes.length === 0) return <LoadingState label={t('list.loading')} />
  if (notes.length === 0) {
    return (
      <EmptyState icon={NotebookPen} title={filtered ? t('list.noMatch') : t('list.empty')}>
        {!filtered && t('list.emptyHint')}
      </EmptyState>
    )
  }
  return (
    <ul className="flex flex-col gap-1 p-2" data-testid="note-list">
      {notes.map((n) => {
        const active = n.id === activeId
        const retired = isRetired(n)
        return (
          <li key={n.id}>
            <button
              type="button"
              data-testid="note-row"
              data-note-id={n.id}
              onClick={() => onSelect(n.id)}
              aria-current={active ? 'true' : undefined}
              className={`w-full rounded-lg px-3 py-2 text-left transition ${
                active
                  ? SELECTED_ITEM_CLS
                  : 'hover:bg-[var(--color-surface-2)] text-[var(--color-text)]'
              } ${retired || n.archived ? 'opacity-70' : ''}`}
            >
              <div className="flex items-start gap-2">
                <span
                  className={`min-w-0 flex-1 truncate text-sm font-medium ${
                    retired ? 'line-through' : ''
                  }`}
                  title={n.title}
                >
                  {n.title}
                </span>
                <span className="shrink-0 text-[10px] text-[var(--color-text-dim)]">
                  {relativeTime(n.updated)}
                </span>
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-1">
                <KindBadge kind={n.kind} />
                <ConfidenceBadge confidence={n.confidence} />
                <ScopeBadge note={n} />
                {(n.occurrences ?? 0) > 1 && (
                  <Badge tone="muted" className="gap-0.5">
                    <Repeat size={10} /> {t('list.seen', { count: n.occurrences ?? 0 })}
                  </Badge>
                )}
                {n.source && (
                  <span className="text-[10px] text-[var(--color-text-dim)]">
                    {t(`source.${n.source}`, { defaultValue: n.source })}
                  </span>
                )}
                {retired && (
                  <Badge tone="danger" className="gap-0.5">
                    {t('list.superseded')}
                  </Badge>
                )}
                {n.archived && (
                  <Badge tone="muted" className="gap-0.5">
                    <Archive size={10} /> {t('list.archived')}
                  </Badge>
                )}
                {n.private && (
                  <Badge tone="accent" className="gap-0.5">
                    <EyeOff size={10} /> {t('list.private')}
                  </Badge>
                )}
              </div>
              {snippets?.[n.id] && (
                <p className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
                  {snippets[n.id]}
                </p>
              )}
            </button>
          </li>
        )
      })}
    </ul>
  )
}
