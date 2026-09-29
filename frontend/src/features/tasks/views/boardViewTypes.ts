// Board view model: the built-in presets, the labels shared by the filter bar,
// and the small helpers used to compare / normalise a view.
//
// A "view" is a filter + a grouping axis + a sort order. Built-in views are
// defined here and never persisted; user-created ones round-trip through
// WorkspaceSettings.boardViews (see internal/db/models_board_view.go).

import type {
  BoardDepFilter,
  BoardReviewFilter,
  BoardFilter,
  BoardGroupBy,
  BoardSort,
  BoardViewDef,
  TaskPriority,
} from '@/types'
import { i18next } from '@/i18n'

// A view as the board actually runs it: groupBy/sort resolved to concrete values.
export interface ResolvedView {
  filter: BoardFilter
  groupBy: BoardGroupBy
  sort: BoardSort
}

const EMPTY_FILTER: BoardFilter = {}

const DEFAULT_VIEW: ResolvedView = {
  filter: EMPTY_FILTER,
  groupBy: 'status',
  sort: 'updated',
}

// Built-in view ids are prefixed so they can never collide with a saved view's
// id (the backend slug pattern forbids ':').
const BUILTIN_PREFIX = 'builtin:'
export const DEFAULT_BUILTIN_VIEW_ID = `${BUILTIN_PREFIX}all`

export function isBuiltinId(id: string): boolean {
  return id.startsWith(BUILTIN_PREFIX)
}

// The always-available presets. Deliberately few: each answers one question the
// unfiltered board answers badly once the card count grows.
export function getBuiltinViews(): BoardViewDef[] {
  return [
    {
      id: DEFAULT_BUILTIN_VIEW_ID,
      label: i18next.t('views.builtins.all', { ns: 'tasks' }),
      icon: '▦',
      filter: {},
      groupBy: 'status',
      sort: 'updated',
    },
    {
      id: `${BUILTIN_PREFIX}blocked`,
      label: i18next.t('views.builtins.blocked', { ns: 'tasks' }),
      icon: '⛔',
      filter: { dep: 'blocked' },
      groupBy: 'status',
      sort: 'deps',
    },
    {
      id: `${BUILTIN_PREFIX}unassigned`,
      label: i18next.t('views.builtins.unassigned', { ns: 'tasks' }),
      icon: '○',
      filter: { agentIds: ['-'] },
      groupBy: 'priority',
      sort: 'priority',
    },
  ]
}

export const GROUP_BY_VALUES: BoardGroupBy[] = ['status', 'agent', 'priority', 'tag']
export const SORT_VALUES: BoardSort[] = ['updated', 'priority', 'deps', 'title']

export function groupByLabel(value: BoardGroupBy): string {
  return i18next.t(`views.groupBy.${value}`, { ns: 'tasks' })
}

export function sortLabel(value: BoardSort): string {
  return i18next.t(`views.sort.${value}`, { ns: 'tasks' })
}

export const PRIORITY_ORDER: Exclude<TaskPriority, ''>[] = ['critical', 'high', 'medium', 'low']

export function priorityLabel(value: TaskPriority): string {
  return i18next.t(value ? `priorities.${value}` : 'priorities.none', { ns: 'tasks' })
}

export function dependencyFilterLabel(value: Exclude<BoardDepFilter, ''>): string {
  return i18next.t(`filters.dependencies.${value}`, { ns: 'tasks' })
}

export function reviewFilterLabel(value: Exclude<BoardReviewFilter, ''>): string {
  return i18next.t(`filters.review.${value}`, { ns: 'tasks' })
}

// resolveView fills in the optional groupBy/sort so downstream code never has
// to branch on undefined.
export function resolveView(v: BoardViewDef | null): ResolvedView {
  if (!v) return DEFAULT_VIEW
  return {
    filter: v.filter ?? EMPTY_FILTER,
    groupBy: v.groupBy ?? 'status',
    sort: v.sort ?? 'updated',
  }
}

// countActiveFacets reports how many facets are narrowing the board. Drives the
// "3 filtre ✕" chip — the user must always be able to see that they are looking
// at a subset.
export function countActiveFacets(f: BoardFilter): number {
  let n = 0
  if (f.text) n++
  if (f.priorities?.length) n++
  if (f.tags?.length) n++
  if (f.agentIds?.length) n++
  if (f.columns?.length) n++
  if (f.dep) n++
  if (f.review) n++
  return n
}

// sameView compares two resolved views by value, so the bar can show a "unsaved
// changes" dot when the live state drifts from the selected preset. Array facets
// are compared order-insensitively since facet order carries no meaning.
export function sameView(a: ResolvedView, b: ResolvedView): boolean {
  if (a.groupBy !== b.groupBy || a.sort !== b.sort) return false
  const fa = a.filter
  const fb = b.filter
  const set = (xs?: string[]) => [...(xs ?? [])].sort().join('\u0000')
  return (
    (fa.text ?? '') === (fb.text ?? '') &&
    (fa.dep ?? '') === (fb.dep ?? '') &&
    (fa.review ?? '') === (fb.review ?? '') &&
    set(fa.priorities) === set(fb.priorities) &&
    set(fa.tags) === set(fb.tags) &&
    set(fa.agentIds) === set(fb.agentIds) &&
    set(fa.columns) === set(fb.columns)
  )
}

// slugifyViewLabel derives a backend-acceptable id from a user-typed name.
// The backend enforces ^[a-z0-9][a-z0-9_-]{0,63}$, so anything else collapses to
// a dash and a non-conforming result falls back to a timestamp-free prefix that
// the caller de-duplicates.
export function slugifyViewLabel(label: string): string {
  const map: Record<string, string> = {
    ç: 'c',
    ğ: 'g',
    ı: 'i',
    ö: 'o',
    ş: 's',
    ü: 'u',
    Ç: 'c',
    Ğ: 'g',
    İ: 'i',
    Ö: 'o',
    Ş: 's',
    Ü: 'u',
  }
  const ascii = [...label].map((ch) => map[ch] ?? ch).join('')
  const slug = ascii
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64)
  return /^[a-z0-9]/.test(slug) ? slug : `view-${slug}`.slice(0, 64)
}

// uniqueViewID appends -2, -3, … until the id is free among `taken`.
export function uniqueViewID(base: string, taken: Set<string>): string {
  if (!taken.has(base)) return base
  for (let i = 2; ; i++) {
    const candidate = `${base}-${i}`.slice(0, 64)
    if (!taken.has(candidate)) return candidate
  }
}
