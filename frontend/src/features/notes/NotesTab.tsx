import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Archive, GitBranch, Plus, Search, X } from 'lucide-react'
import { api } from '@/api'
import type { ListNotesParams } from '@/api/notes'
import { SIGNAL_NOTES } from '@/app/eventToRefreshSignals'
import { EmptyState, FilterToggle, toast, useFilterDisclosure } from '@/shared/components'
import { useDebouncedValue } from '@/shared/hooks/useDebouncedValue'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import type { Note, NoteConfidence, NoteExpansion, NoteScope, NoteWrite } from '@/types'
import { NoteDetail } from './NoteDetail'
import { NoteForm, type NoteFormMode } from './NoteForm'
import { NoteList } from './NoteList'
import {
  EMPTY_NOTE_FILTER,
  EMPTY_NOTE_WRITE,
  NOTE_CONFIDENCES,
  NOTE_KINDS,
  NOTE_SCOPES,
  applyNoteFilter,
  hasActiveNoteFilters,
  listParamsFor,
  noteToWrite,
  sortNotesNewest,
  toggleKind,
  type NoteFilter,
} from './notesHelpers'

interface Props {
  onError: (msg: string) => void
  // Selected note id (URL-synced by the parent: #/w/{ws}/notes/{id}).
  selectedId: string | null
  onSelectNote: (id: string | null) => void
  onOpenSession?: (sessionId: string) => void
}

interface FormState {
  mode: NoteFormMode
  initial: NoteWrite
  // The note being edited or corrected (absent for create).
  target?: Note
}

const selectCls =
  'rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs'

// NotesTab is the memory browser: a filter strip (kind chips, scope, confidence,
// archived / superseded toggles, free-text search) over a list + detail split.
// A non-empty search goes to /api/notes/search; otherwise the list comes from
// /api/notes with the same facets. Live SSE `notes` events re-fetch both.
export function NotesTab({ onError, selectedId, onSelectNote, onOpenSession }: Props) {
  const { t } = useTranslation('notes')
  const tick = useRefreshTrigger(SIGNAL_NOTES)
  const [filter, setFilter] = useState<NoteFilter>(EMPTY_NOTE_FILTER)
  const query = useDebouncedValue(filter.query.trim(), 250)
  // The page for the current request (facets + search + refresh tick). Loading
  // is derived: a page whose key differs from the live request is stale, so the
  // effect never has to flip a loading flag synchronously.
  const [page, setPage] = useState<{
    key: string
    notes: Note[]
    snippets: Record<string, string>
  }>({ key: '', notes: [], snippets: {} })
  // A deep-linked note that the current page does not contain (archived,
  // superseded or filtered out) is fetched on its own so the detail still opens.
  const [extraNote, setExtraNote] = useState<Note | null>(null)
  const [expansion, setExpansion] = useState<NoteExpansion | null>(null)
  const [form, setForm] = useState<FormState | null>(null)
  const [busy, setBusy] = useState(false)

  // Server-side facets as a stable string so the effect re-runs only when the
  // request would actually differ; the effect parses it back instead of
  // depending on the whole filter object.
  const paramsKey = JSON.stringify(listParamsFor(filter))
  const requestKey = `${tick}|${query}|${paramsKey}`

  useEffect(() => {
    const ctl = new AbortController()
    const params = JSON.parse(paramsKey) as ListNotesParams
    const run = query
      ? api.searchNotes(query, { ...params, limit: 100 }, ctl.signal).then((hits) => {
          const map: Record<string, string> = {}
          for (const h of hits) map[h.note.id] = h.snippet
          return { notes: hits.map((h) => h.note), snippets: map }
        })
      : api
          .listNotes({ ...params, limit: 500 }, ctl.signal)
          .then((notes) => ({ notes, snippets: {} }))
    run
      .then((res) => {
        if (!ctl.signal.aborted) setPage({ key: requestKey, ...res })
      })
      .catch((e) => {
        if (ctl.signal.aborted) return
        onError((e as Error).message)
        // Mark the request answered so the list does not spin forever.
        setPage((p) => ({ ...p, key: requestKey }))
      })
    return () => ctl.abort()
  }, [requestKey, query, paramsKey, onError])

  const loading = page.key !== requestKey
  const notes = page.notes
  const snippets = page.snippets

  const visible = sortNotesNewest(applyNoteFilter(notes, filter))
  const inList = selectedId ? visible.find((n) => n.id === selectedId) : undefined
  const active = inList ?? (extraNote && extraNote.id === selectedId ? extraNote : null)

  // Fetch the selected note when the list does not carry it.
  useEffect(() => {
    if (!selectedId || inList) return
    const ctl = new AbortController()
    api
      .getNote(selectedId)
      .then((n) => {
        if (!ctl.signal.aborted) setExtraNote(n)
      })
      .catch(() => {
        // A stale deep link (deleted note): drop the selection quietly.
        if (!ctl.signal.aborted) onSelectNote(null)
      })
    return () => ctl.abort()
  }, [selectedId, inList, tick, onSelectNote])

  // Neighbourhood (links / backlinks / correction chain) of the selected note.
  useEffect(() => {
    if (!selectedId) return
    const ctl = new AbortController()
    api
      .expandNote(selectedId, ctl.signal)
      .then((ex) => {
        if (!ctl.signal.aborted) setExpansion(ex)
      })
      .catch(() => {
        if (!ctl.signal.aborted) setExpansion(null)
      })
    return () => ctl.abort()
  }, [selectedId, tick])

  const patch = (p: Partial<NoteFilter>) => setFilter((f) => ({ ...f, ...p }))

  const runAction = useCallback(
    async (fn: () => Promise<void>) => {
      setBusy(true)
      try {
        await fn()
      } catch (e) {
        onError((e as Error).message)
      } finally {
        setBusy(false)
      }
    },
    [onError],
  )

  const submitForm = (write: NoteWrite) => {
    if (!form) return
    void runAction(async () => {
      if (form.mode === 'create') {
        const created = await api.createNote(write)
        toast.success(t('toast.created'))
        onSelectNote(created.id)
      } else if (form.mode === 'edit' && form.target) {
        const updated = await api.updateNote(form.target.id, write)
        toast.success(t('toast.updated'))
        setExtraNote(updated)
      } else if (form.mode === 'correct' && form.target) {
        const next = await api.correctNote(form.target.id, write)
        toast.success(t('toast.corrected'))
        onSelectNote(next.id)
      }
      setForm(null)
    })
  }

  const setArchived = (n: Note, archived: boolean) =>
    runAction(async () => {
      const updated = await api.setNoteArchived(n.id, archived)
      setExtraNote(updated)
      toast.success(archived ? t('toast.archived') : t('toast.restored'))
    })

  const remove = (n: Note) => {
    if (!window.confirm(t('confirm.delete', { title: n.title }))) return
    void runAction(async () => {
      // A linked note or a correction-chain member is refused (409, "archive it
      // instead"); the backend message surfaces through onError.
      await api.deleteNote(n.id)
      toast.success(t('toast.deleted'))
      onSelectNote(null)
    })
  }

  const filtered = hasActiveNoteFilters(filter)
  const [filtersOpen, toggleFilters] = useFilterDisclosure('notes')
  const facetCount =
    filter.kinds.length +
    (filter.scope ? 1 : 0) +
    (filter.confidence ? 1 : 0) +
    (filter.showArchived ? 1 : 0) +
    (filter.showSuperseded ? 1 : 0)

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Filter strip */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-3 py-2">
        <div className="relative">
          <Search
            size={13}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
          />
          <input
            data-testid="notes-search"
            value={filter.query}
            onChange={(e) => patch({ query: e.target.value })}
            placeholder={t('filters.searchPlaceholder')}
            className="w-56 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] py-1 pl-7 pr-7 text-xs outline-none focus:border-[var(--color-accent)]"
          />
          {filter.query && (
            <button
              type="button"
              onClick={() => patch({ query: '' })}
              title={t('filters.clear')}
              className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              <X size={13} />
            </button>
          )}
        </div>
        <FilterToggle
          open={filtersOpen}
          onToggle={toggleFilters}
          activeCount={facetCount}
          testId="notes-filters-toggle"
        />
        {filtersOpen && (
          <>
            <div className="flex flex-wrap gap-1">
              {NOTE_KINDS.map((k) => {
                const on = filter.kinds.includes(k)
                return (
                  <button
                    key={k}
                    type="button"
                    data-testid="notes-kind-chip"
                    data-kind={k}
                    data-active={on}
                    onClick={() => patch({ kinds: toggleKind(filter.kinds, k) })}
                    className={`rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
                      on
                        ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                        : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                    }`}
                  >
                    {t(`kind.${k}`)}
                  </button>
                )
              })}
            </div>
            <select
              className={selectCls}
              value={filter.scope}
              onChange={(e) => patch({ scope: e.target.value as NoteScope | '' })}
            >
              <option value="">{t('filters.allScopes')}</option>
              {NOTE_SCOPES.map((s) => (
                <option key={s} value={s}>
                  {t(`scope.${s}`)}
                </option>
              ))}
            </select>
            <select
              className={selectCls}
              value={filter.confidence}
              onChange={(e) => patch({ confidence: e.target.value as NoteConfidence | '' })}
            >
              <option value="">{t('filters.allConfidences')}</option>
              {NOTE_CONFIDENCES.map((c) => (
                <option key={c} value={c}>
                  {t(`confidence.${c}`)}
                </option>
              ))}
            </select>
            <FacetToggle
              on={filter.showArchived}
              onClick={() => patch({ showArchived: !filter.showArchived })}
              icon={<Archive size={12} />}
              label={t('filters.showArchived')}
            />
            <FacetToggle
              on={filter.showSuperseded}
              onClick={() => patch({ showSuperseded: !filter.showSuperseded })}
              icon={<GitBranch size={12} />}
              label={t('filters.showSuperseded')}
            />
          </>
        )}
        {filtered && (
          <button
            type="button"
            onClick={() => setFilter(EMPTY_NOTE_FILTER)}
            className="text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            {t('filters.clear')}
          </button>
        )}
        <span className="ml-auto text-xs text-[var(--color-text-dim)]">
          {t('list.count', { count: visible.length })}
        </span>
        <button
          type="button"
          data-testid="notes-new"
          onClick={() => setForm({ mode: 'create', initial: EMPTY_NOTE_WRITE })}
          className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-2.5 py-1 text-xs font-medium text-[var(--color-on-accent)] hover:opacity-90"
        >
          <Plus size={13} /> {t('actions.new')}
        </button>
      </div>

      {/* List + detail */}
      <div className="flex min-h-0 flex-1 flex-col md:flex-row">
        <div className="max-h-[40%] shrink-0 overflow-y-auto border-b border-[var(--color-border)] md:max-h-none md:w-80 md:border-b-0 md:border-r">
          <NoteList
            notes={visible}
            snippets={query ? snippets : undefined}
            activeId={selectedId}
            onSelect={onSelectNote}
            loading={loading}
            filtered={filtered}
          />
        </div>
        <div className="min-h-0 min-w-0 flex-1 overflow-y-auto">
          {active ? (
            <NoteDetail
              note={active}
              expansion={expansion?.note.id === active.id ? expansion : null}
              onSelectNote={onSelectNote}
              onEdit={() => setForm({ mode: 'edit', initial: noteToWrite(active), target: active })}
              onCorrect={() =>
                setForm({ mode: 'correct', initial: noteToWrite(active), target: active })
              }
              onSetArchived={(archived) => void setArchived(active, archived)}
              onDelete={() => remove(active)}
              onOpenSession={onOpenSession}
              busy={busy}
            />
          ) : (
            <EmptyState title={t('detail.selectPrompt')} className="h-full justify-center" />
          )}
        </div>
      </div>

      {form && (
        <NoteForm
          key={`${form.mode}:${form.target?.id ?? 'new'}`}
          mode={form.mode}
          initial={form.initial}
          submitting={busy}
          onSubmit={submitForm}
          onCancel={() => setForm(null)}
        />
      )}
    </div>
  )
}

function FacetToggle({
  on,
  onClick,
  icon,
  label,
}: {
  on: boolean
  onClick: () => void
  icon: ReactNode
  label: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      data-active={on}
      aria-pressed={on}
      className={`flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
        on
          ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
          : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
      }`}
    >
      {icon} {label}
    </button>
  )
}
