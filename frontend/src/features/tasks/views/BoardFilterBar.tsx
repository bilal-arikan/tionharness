// BoardFilterBar — the single row under the board header that owns search, the
// facet dropdowns, the grouping axis, the sort order and the saved-view menu.
//
// Everything here mutates the board's LIVE view; nothing is persisted until the
// user presses Kaydet.

import { useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Search, X } from 'lucide-react'
import type {
  Agent,
  BoardColumnDef,
  BoardDepFilter,
  BoardGroupBy,
  BoardReviewFilter,
  BoardSort,
  Task,
} from '@/types'
import { FacetDropdown, type FacetOption } from './FacetDropdown'
import { SavedViewMenu } from './SavedViewMenu'
import {
  dependencyFilterLabel,
  reviewFilterLabel,
  GROUP_BY_VALUES,
  SORT_VALUES,
  PRIORITY_ORDER,
  groupByLabel,
  priorityLabel,
  sortLabel,
  countActiveFacets,
  isBuiltinId,
} from './boardViewTypes'
import type { BoardViewState } from './useBoardView'
import { compareText } from '@/shared/lib/intl'
import { FilterToggle, useFilterDisclosure } from '@/shared/components'

interface Props {
  view: BoardViewState
  /** Every task in the workspace — facet option counts are computed from these. */
  tasks: Task[]
  /** Tasks surviving the current filter, for the "12 / 87" counter. */
  visibleCount: number
  agents: Agent[]
  boardColumns: BoardColumnDef[]
}

export function BoardFilterBar({ view, tasks, visibleCount, agents, boardColumns }: Props) {
  const { t } = useTranslation('tasks')
  const { live, setFilter } = view
  const f = live.filter
  const searchRef = useRef<HTMLInputElement>(null)
  const [facetsOpen, toggleFacets] = useFilterDisclosure('board')

  // "/" focuses search, the way every list-heavy tool does it. Ignored while the
  // user is already typing somewhere, so it cannot hijack a card title.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return
      const el = document.activeElement
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return
      if (el instanceof HTMLElement && el.isContentEditable) return
      e.preventDefault()
      searchRef.current?.focus()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const patch = (part: Partial<typeof f>) => setFilter({ ...f, ...part })

  // Facet option counts come from the UNFILTERED list: a count that shrank
  // because of another facet would make the menu look broken.
  const countBy = <K extends string>(key: (t: Task) => K[] | K): Map<K, number> => {
    const m = new Map<K, number>()
    for (const t of tasks) {
      const ks = key(t)
      for (const k of Array.isArray(ks) ? ks : [ks]) m.set(k, (m.get(k) ?? 0) + 1)
    }
    return m
  }

  const priorityCounts = countBy((t) => (t.priority ?? '') as string)
  const tagCounts = countBy((t) => t.tags ?? [])
  const agentCounts = countBy((t) => t.ownerAgentId || '-')
  const columnCounts = countBy((t) => t.boardState)

  const priorityOptions: FacetOption[] = useMemo(
    () =>
      [...PRIORITY_ORDER, '' as const].map((p) => ({
        value: p,
        label: priorityLabel(p),
        count: priorityCounts.get(p) ?? 0,
      })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [tasks],
  )

  const tagOptions: FacetOption[] = useMemo(
    () =>
      [...tagCounts.keys()]
        .sort((a, b) => compareText(a, b))
        .map((tag) => ({ value: tag, label: `#${tag}`, count: tagCounts.get(tag) })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [tasks],
  )

  const agentOptions: FacetOption[] = useMemo(() => {
    const opts: FacetOption[] = agents
      .filter((a) => (agentCounts.get(a.id) ?? 0) > 0)
      .map((a) => ({ value: a.id, label: a.name, color: a.color, count: agentCounts.get(a.id) }))
    if ((agentCounts.get('-') ?? 0) > 0) {
      opts.push({ value: '-', label: t('filters.unassigned'), count: agentCounts.get('-') })
    }
    return opts
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tasks, agents, t])

  const columnOptions: FacetOption[] = boardColumns.map((c) => ({
    value: c.key,
    label: c.label,
    color: c.color || undefined,
    count: columnCounts.get(c.key) ?? 0,
  }))

  const depOptions: FacetOption[] = (['blocked', 'ready'] as const).map((d) => ({
    value: d,
    label: dependencyFilterLabel(d),
  }))

  const reviewOptions: FacetOption[] = (['bounced', 'exhausted'] as const).map((r) => ({
    value: r,
    label: reviewFilterLabel(r),
  }))

  const activeFacets = countActiveFacets(f)
  const filtering = activeFacets > 0

  const save = async () => {
    // Overwriting a built-in is impossible, so an edited built-in always becomes
    // a new view — which is also what the user means by "save" in that case.
    if (isBuiltinId(view.selectedId)) {
      const label = prompt(t('views.newNamePrompt'))
      if (label === null) return
      await view.saveAsNew(label)
      return
    }
    const choice = confirm(t('views.overwriteConfirm'))
    if (choice) {
      await view.saveOverwrite()
      return
    }
    const label = prompt(t('views.newNamePrompt'))
    if (label === null) return
    await view.saveAsNew(label)
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5 border-b border-[var(--color-border)] px-4 py-2">
      {/* Search */}
      <div className="relative">
        <Search
          size={12}
          className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
        />
        <input
          ref={searchRef}
          value={f.text ?? ''}
          onChange={(e) => patch({ text: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              patch({ text: '' })
              e.currentTarget.blur()
            }
          }}
          placeholder={t('filters.searchPlaceholder')}
          data-testid="board-filter-search"
          className="w-40 rounded border border-[var(--color-border)] bg-[var(--color-bg)] py-1 pl-6 pr-2 text-xs outline-none transition focus:border-[var(--color-accent)]"
        />
      </div>

      <SavedViewMenu
        allViews={view.allViews}
        selectedId={view.selectedId}
        dirty={view.dirty}
        onSelect={view.selectView}
        onRename={view.renameView}
        onDelete={view.deleteView}
      />

      <span className="mx-0.5 h-4 w-px bg-[var(--color-border)]" />

      <FilterToggle
        open={facetsOpen}
        onToggle={toggleFacets}
        activeCount={activeFacets - (f.text ? 1 : 0)}
        testId="board-filters-toggle"
      />
      {facetsOpen && (
        <>
          <FacetDropdown
            label={t('filters.priority')}
            options={priorityOptions}
            selected={f.priorities ?? []}
            onChange={(v) => patch({ priorities: v as never })}
          />
          <FacetDropdown
            label={t('filters.tag')}
            options={tagOptions}
            selected={f.tags ?? []}
            onChange={(v) => patch({ tags: v })}
            emptyHint={t('filters.noTags')}
          />
          <FacetDropdown
            label={t('filters.agent')}
            options={agentOptions}
            selected={f.agentIds ?? []}
            onChange={(v) => patch({ agentIds: v })}
          />
          <FacetDropdown
            label={t('filters.dependency')}
            options={depOptions}
            selected={f.dep ? [f.dep] : []}
            onChange={(v) => patch({ dep: (v[0] ?? '') as BoardDepFilter })}
            mode="single"
          />
          <FacetDropdown
            label={t('filters.reviewLabel')}
            options={reviewOptions}
            selected={f.review ? [f.review] : []}
            onChange={(v) => patch({ review: (v[0] ?? '') as BoardReviewFilter })}
            mode="single"
          />
          <FacetDropdown
            label={t('filters.column')}
            options={columnOptions}
            selected={f.columns ?? []}
            onChange={(v) => patch({ columns: v })}
          />
        </>
      )}

      <span className="mx-0.5 h-4 w-px bg-[var(--color-border)]" />

      {/* Grouping axis + sort. */}
      <select
        value={live.groupBy}
        onChange={(e) => view.setGroupBy(e.target.value as BoardGroupBy)}
        title={t('views.groupByTitle')}
        data-testid="board-group-by"
        className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
      >
        {GROUP_BY_VALUES.map((g) => (
          <option key={g} value={g}>
            ⊞ {groupByLabel(g)}
          </option>
        ))}
      </select>
      <select
        value={live.sort}
        onChange={(e) => view.setSort(e.target.value as BoardSort)}
        title={t('views.sortTitle')}
        data-testid="board-sort"
        className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
      >
        {SORT_VALUES.map((s) => (
          <option key={s} value={s}>
            ↕ {sortLabel(s)}
          </option>
        ))}
      </select>

      <div className="ml-auto flex items-center gap-1.5">
        {/* The subset counter. Always shows the total when filtering, so a
            filtered board can never be mistaken for the whole board. */}
        <span
          data-testid="board-filter-count"
          className={`text-xs ${filtering ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
        >
          {filtering
            ? `${visibleCount} / ${tasks.length}`
            : t('taskCount', { count: tasks.length })}
        </span>
        {filtering && (
          <button
            onClick={view.clearFilter}
            title={t('filters.clearAll')}
            className="flex items-center gap-1 rounded border border-[var(--color-accent)] bg-[var(--color-accent-soft)] px-1.5 py-1 text-xs text-[var(--color-accent)]"
          >
            {t('filters.activeCount', { count: activeFacets })} <X size={11} />
          </button>
        )}
        {view.dirty && (
          <>
            <button
              onClick={view.revert}
              title={t('views.revert')}
              className="rounded border border-[var(--color-border)] px-1.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              ↺
            </button>
            <button
              onClick={() => void save()}
              data-testid="board-view-save"
              className="rounded border border-[var(--color-accent)] bg-[var(--color-accent-soft)] px-2 py-1 text-xs text-[var(--color-accent)]"
            >
              {t('actions.save')}
            </button>
          </>
        )}
      </div>
    </div>
  )
}
