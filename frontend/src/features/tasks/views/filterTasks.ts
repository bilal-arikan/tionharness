// Board filtering + sorting. Pure functions over the task list the board already
// holds in state — there is no server-side query, because ListTasks returns the
// whole board and a workspace holds hundreds of cards, not millions.
//
// Facets combine with AND; values within one facet combine with OR.

import type { BoardFilter, BoardSort, Task } from '@/types'
import { PRIORITY_ORDER } from './boardViewTypes'

// todayISO returns the local date as YYYY-MM-DD, matching the format tasks store
// in startDate/dueDate. Deliberately local (not UTC): "due today" must mean the
// user's today, otherwise a card flips to overdue hours early or late.
export function todayISO(now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}

// addDaysISO shifts a YYYY-MM-DD date by n days, staying in local time.
function addDaysISO(iso: string, n: number): string {
  const [y, m, d] = iso.split('-').map(Number)
  const dt = new Date(y, m - 1, d + n)
  return todayISO(dt)
}

// foldForSearch normalises text for substring matching.
//
// Plain Turkish lowercasing is WRONG here: it maps 'I' to 'ı', so typing
// "LOGIN" would fail to match the title "Login". Plain invariant lowercasing is
// wrong the other way: 'İ' becomes 'i' + a combining dot, which fails to match
// "istanbul". So both dotted and dotless forms are folded onto plain 'i' — for
// search, the two should simply be interchangeable.
function foldForSearch(s: string): string {
  return s.replace(/[İI]/g, 'i').replace(/ı/g, 'i').toLowerCase().normalize('NFD').replace(/̇/g, '') // strip the combining dot above left by 'İ'
}

// Parse a task's dependencies JSON string into an array of task ids.
export function parseDeps(raw: string): string[] {
  try {
    const arr = JSON.parse(raw || '[]')
    return Array.isArray(arr) ? (arr as string[]) : []
  } catch {
    return []
  }
}

// dueBucket classifies a task's due date. ISO dates compare correctly as
// strings, so no Date objects are needed on the hot path.
function dueBucket(
  task: Task,
  today: string,
  weekEnd: string,
): 'overdue' | 'today' | 'week' | 'none' | 'later' {
  const due = task.dueDate
  if (!due) return 'none'
  if (due < today) return 'overdue'
  if (due === today) return 'today'
  if (due <= weekEnd) return 'week'
  return 'later'
}

// depState classifies a task's dependency situation against the full task list.
// A dependency id that no longer resolves is ignored rather than treated as
// unmet — a deleted blocker should not pin a card in "blocked" forever.
function depState(task: Task, byID: Map<string, Task>): 'none' | 'blocked' | 'ready' {
  const ids = parseDeps(task.dependencies)
  if (ids.length === 0) return 'none'
  let known = 0
  for (const id of ids) {
    const dep = byID.get(id)
    if (!dep) continue
    known++
    if (dep.boardState !== 'done') return 'blocked'
  }
  return known === 0 ? 'none' : 'ready'
}

// filterTasks applies every active facet. `all` is the unfiltered task list,
// needed because dependency state is relational (a card's blocked-ness depends
// on cards that the filter itself may have hidden).
export function filterTasks(all: Task[], filter: BoardFilter, now = new Date()): Task[] {
  const today = todayISO(now)
  const weekEnd = addDaysISO(today, 7)
  const byID = new Map(all.map((t) => [t.id, t]))
  const text = filter.text ? foldForSearch(filter.text.trim()) : ''

  return all.filter((t) => {
    if (text) {
      const hay = foldForSearch(`${t.title}\n${t.description}`)
      if (!hay.includes(text)) return false
    }
    if (filter.priorities?.length && !filter.priorities.includes((t.priority ?? '') as never)) {
      return false
    }
    if (filter.tags?.length) {
      const tags = t.tags ?? []
      if (!filter.tags.some((want) => tags.includes(want))) return false
    }
    if (filter.agentIds?.length) {
      // '-' is the unassigned sentinel; a task with no owner matches only it.
      const owner = t.ownerAgentId || '-'
      if (!filter.agentIds.includes(owner)) return false
    }
    if (filter.columns?.length && !filter.columns.includes(t.boardState)) {
      return false
    }
    if (filter.dues?.length) {
      const bucket = dueBucket(t, today, weekEnd)
      // 'week' is inclusive of today and overdue is separate, so selecting
      // "bu hafta" alone still surfaces something due tomorrow.
      const matches = filter.dues.some((want) =>
        want === 'week' ? bucket === 'today' || bucket === 'week' : bucket === want,
      )
      if (!matches) return false
    }
    if (filter.dep) {
      if (depState(t, byID) !== filter.dep) return false
    }
    return true
  })
}

// Topological levels so tasks with no blockers sort first (level 0). Cycles are
// broken by assigning level 0 to the repeated node. Mirrors the ordering the
// board's old "🔗 Sırala" toggle produced, now expressed as a sort option.
export function topoLevels(tasks: Task[]): Map<string, number> {
  const depsOf = new Map<string, string[]>()
  for (const t of tasks) depsOf.set(t.id, parseDeps(t.dependencies))
  const levels = new Map<string, number>()
  const visiting = new Set<string>()
  function level(id: string): number {
    const cached = levels.get(id)
    if (cached !== undefined) return cached
    if (visiting.has(id)) return 0
    visiting.add(id)
    const deps = (depsOf.get(id) ?? []).filter((d) => depsOf.has(d))
    const l = deps.length === 0 ? 0 : Math.max(...deps.map((d) => level(d) + 1))
    visiting.delete(id)
    levels.set(id, l)
    return l
  }
  for (const t of tasks) level(t.id)
  return levels
}

// priorityRank maps a priority to a sortable index; unset sorts last.
function priorityRank(p: string | undefined): number {
  const i = PRIORITY_ORDER.indexOf((p ?? '') as never)
  return i === -1 ? PRIORITY_ORDER.length : i
}

// sortTasks orders one column's cards. Every mode falls back to "most recently
// updated first" on ties, so the order is total and stable across renders.
export function sortTasks(
  tasks: Task[],
  sort: BoardSort,
  levels: Map<string, number> | null,
): Task[] {
  const out = [...tasks]
  out.sort((a, b) => {
    switch (sort) {
      case 'priority': {
        const d = priorityRank(a.priority) - priorityRank(b.priority)
        if (d !== 0) return d
        break
      }
      case 'due': {
        // Dated cards first, earliest deadline leading; undated sink to the end.
        const da = a.dueDate || '￿'
        const db = b.dueDate || '￿'
        if (da !== db) return da < db ? -1 : 1
        break
      }
      case 'deps': {
        if (levels) {
          const d = (levels.get(a.id) ?? 0) - (levels.get(b.id) ?? 0)
          if (d !== 0) return d
        }
        break
      }
      case 'title': {
        const d = a.title.localeCompare(b.title, 'tr')
        if (d !== 0) return d
        break
      }
      case 'updated':
        break
    }
    return b.updatedAt - a.updatedAt
  })
  return out
}
