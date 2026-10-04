import { useTranslation } from 'react-i18next'
import {
  Archive,
  ArchiveRestore,
  ArrowRight,
  ExternalLink,
  GitBranch,
  Link2,
  Pencil,
  Trash2,
} from 'lucide-react'
import { Badge, Button, KeyValueRow } from '@/shared/components'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { formatDateTime } from '@/shared/lib/intl'
import type { Note, NoteExpansion } from '@/types'
import { ConfidenceBadge, KindBadge, ScopeBadge } from './noteBadges'
import { isRetired, reachOf } from './notesHelpers'

interface Props {
  note: Note
  expansion: NoteExpansion | null
  onSelectNote: (id: string) => void
  onEdit: () => void
  onCorrect: () => void
  onSetArchived: (archived: boolean) => void
  onDelete: () => void
  onOpenSession?: (sessionId: string) => void
  busy: boolean
}

// NoteDetail is the right pane of the Notes tab: the frontmatter facts, the
// Markdown body and the neighbourhood from /expand (links, backlinks, the
// correction chain), plus the edit / correct / archive / delete actions.
export function NoteDetail({
  note,
  expansion,
  onSelectNote,
  onEdit,
  onCorrect,
  onSetArchived,
  onDelete,
  onOpenSession,
  busy,
}: Props) {
  const { t } = useTranslation('notes')
  const retired = isRetired(note)
  const reach = reachOf(note)
  const when = (unix: number) => formatDateTime(unix * 1000)

  const chain = [...(expansion?.predecessors ?? []), note, ...(expansion?.successors ?? [])]
  const hasChain = chain.length > 1

  return (
    <div className="flex flex-col gap-4 p-4" data-testid="note-detail">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 flex-1">
          <h2 className={`text-base font-semibold ${retired ? 'line-through' : ''}`}>
            {note.title}
          </h2>
          <div className="mt-1 flex flex-wrap items-center gap-1">
            <KindBadge kind={note.kind} />
            <ConfidenceBadge confidence={note.confidence} />
            <ScopeBadge note={note} />
            {retired && <Badge tone="danger">{t('list.superseded')}</Badge>}
            {note.archived && <Badge tone="muted">{t('list.archived')}</Badge>}
            {note.private && <Badge tone="accent">{t('list.private')}</Badge>}
            <code className="text-[10px] text-[var(--color-text-dim)]">{note.id}</code>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <Button size="sm" variant="secondary" onClick={onEdit} disabled={busy}>
            <Pencil size={13} /> {t('actions.edit')}
          </Button>
          {!retired && (
            <Button
              size="sm"
              variant="secondary"
              onClick={onCorrect}
              disabled={busy}
              title={t('actions.correctTitle')}
            >
              <GitBranch size={13} /> {t('actions.correct')}
            </Button>
          )}
          <Button
            size="sm"
            variant="secondary"
            onClick={() => onSetArchived(!note.archived)}
            disabled={busy}
          >
            {note.archived ? <ArchiveRestore size={13} /> : <Archive size={13} />}{' '}
            {note.archived ? t('actions.restore') : t('actions.archive')}
          </Button>
          <Button
            size="sm"
            variant="danger"
            onClick={onDelete}
            disabled={busy}
            title={t('actions.deleteTitle')}
          >
            <Trash2 size={13} /> {t('actions.delete')}
          </Button>
        </div>
      </div>

      {retired && note.supersededBy && (
        <button
          type="button"
          onClick={() => onSelectNote(note.supersededBy!)}
          className="flex items-center gap-2 rounded-md border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_8%,transparent)] px-3 py-2 text-left text-xs"
        >
          <ArrowRight size={13} className="shrink-0 text-[var(--color-danger)]" />
          <span>
            {t('detail.supersededBy')} <code>{note.supersededBy}</code>
          </span>
        </button>
      )}

      {/* Frontmatter facts */}
      <div className="grid grid-cols-1 gap-x-6 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 sm:grid-cols-2">
        <KeyValueRow label={t('detail.scope')} value={t(`scope.${note.scope}`)} />
        {reach.length > 0 && (
          <KeyValueRow
            label={note.scope === 'agent' ? t('form.agents') : t('form.projects')}
            value={reach.join(', ')}
          />
        )}
        <KeyValueRow label={t('detail.confidence')} value={t(`confidence.${note.confidence}`)} />
        {note.verification && (
          <KeyValueRow label={t('detail.verification')} value={note.verification} />
        )}
        <KeyValueRow label={t('detail.created')} value={when(note.created)} />
        <KeyValueRow label={t('detail.updated')} value={when(note.updated)} />
        {note.source && (
          <KeyValueRow
            label={t('detail.source')}
            value={t(`source.${note.source}`, { defaultValue: note.source })}
          />
        )}
        {note.sourceAgent && (
          <KeyValueRow label={t('detail.sourceAgent')} value={note.sourceAgent} />
        )}
        {(note.occurrences ?? 0) > 0 && (
          <KeyValueRow
            label={t('detail.occurrences')}
            value={t('list.seen', { count: note.occurrences ?? 0 })}
          />
        )}
        {note.signature && <KeyValueRow label={t('detail.signature')} value={note.signature} />}
        {note.supersedes && (
          <div className="flex items-center justify-between py-0.5 text-xs">
            <span className="text-[var(--color-text-dim)]">{t('detail.supersedes')}</span>
            <button
              type="button"
              onClick={() => onSelectNote(note.supersedes!)}
              className="font-mono text-[var(--color-accent)] hover:underline"
            >
              {note.supersedes}
            </button>
          </div>
        )}
        {note.sourceSession && (
          <div className="flex items-center justify-between py-0.5 text-xs">
            <span className="text-[var(--color-text-dim)]">{t('detail.sourceSession')}</span>
            {onOpenSession ? (
              <button
                type="button"
                onClick={() => onOpenSession(note.sourceSession!)}
                className="flex items-center gap-1 font-mono text-[var(--color-accent)] hover:underline"
                title={t('actions.openSession')}
              >
                {note.sourceSession} <ExternalLink size={11} />
              </button>
            ) : (
              <code>{note.sourceSession}</code>
            )}
          </div>
        )}
        {(note.tags?.length ?? 0) > 0 && (
          <div className="flex items-center justify-between gap-2 py-0.5 text-xs sm:col-span-2">
            <span className="text-[var(--color-text-dim)]">{t('detail.tags')}</span>
            <span className="flex flex-wrap justify-end gap-1">
              {note.tags!.map((tag) => (
                <Badge key={tag}>{tag}</Badge>
              ))}
            </span>
          </div>
        )}
      </div>

      {/* Body */}
      <div className="rounded-lg border border-[var(--color-border)] p-4">
        <Markdown>{note.body}</Markdown>
      </div>

      {/* Neighbourhood */}
      {expansion && (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          <NoteRefs
            title={t('detail.links')}
            notes={expansion.links}
            unresolved={expansion.unresolved}
            onSelect={onSelectNote}
            emptyLabel={t('detail.noLinks')}
          />
          <NoteRefs
            title={t('detail.backlinks')}
            notes={expansion.backlinks}
            onSelect={onSelectNote}
            emptyLabel={t('detail.noBacklinks')}
          />
          {hasChain && (
            <div className="md:col-span-2">
              <h3 className="mb-1 flex items-center gap-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                <GitBranch size={12} /> {t('detail.chain')}
              </h3>
              <ol className="flex flex-wrap items-center gap-1 text-xs">
                {chain.map((n, i) => (
                  <li key={n.id} className="flex items-center gap-1">
                    {i > 0 && <ArrowRight size={11} className="text-[var(--color-text-dim)]" />}
                    {n.id === note.id ? (
                      <span className="rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 font-medium text-[var(--color-accent)]">
                        {n.title}
                      </span>
                    ) : (
                      <button
                        type="button"
                        onClick={() => onSelectNote(n.id)}
                        className="rounded px-1.5 py-0.5 hover:bg-[var(--color-surface-2)] hover:underline"
                      >
                        {n.title}
                      </button>
                    )}
                  </li>
                ))}
              </ol>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function NoteRefs({
  title,
  notes,
  unresolved,
  onSelect,
  emptyLabel,
}: {
  title: string
  notes: Note[] | null | undefined
  unresolved?: string[] | null
  onSelect: (id: string) => void
  emptyLabel: string
}) {
  const { t } = useTranslation('notes')
  // A neighbourhood the backend had nothing to put in may arrive as null.
  const list = notes ?? []
  return (
    <div>
      <h3 className="mb-1 flex items-center gap-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <Link2 size={12} /> {title}
        <span className="font-normal">· {list.length}</span>
      </h3>
      {list.length === 0 && (unresolved?.length ?? 0) === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">{emptyLabel}</p>
      ) : (
        <ul className="flex flex-col gap-0.5 text-xs">
          {list.map((n) => (
            <li key={n.id}>
              <button
                type="button"
                onClick={() => onSelect(n.id)}
                className="flex w-full items-center gap-2 rounded px-1.5 py-1 text-left hover:bg-[var(--color-surface-2)]"
              >
                <KindBadge kind={n.kind} />
                <span className="truncate">{n.title}</span>
              </button>
            </li>
          ))}
          {unresolved?.map((target) => (
            <li
              key={`unresolved:${target}`}
              className="flex items-center gap-2 px-1.5 py-1 text-[var(--color-text-dim)]"
              title={t('detail.unresolved')}
            >
              <Badge tone="warning">{t('detail.unresolvedBadge')}</Badge>
              <span className="truncate">{target}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
