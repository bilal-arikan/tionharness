// Column derivation: turn a grouping axis into the column list the board
// renders, plus the rules for assigning a card to a column and for what a drag
// between columns should write.
//
// This is what makes one board serve four questions. The render code never
// changes — only the columns it is handed, and the field a drop mutates.

import type { Agent, BoardColumnDef, BoardGroupBy, BoardDueFilter, Task, TaskPatch } from '@/types'
import { DUE_LABELS, DUE_ORDER, PRIORITY_LABELS, PRIORITY_ORDER } from './boardViewTypes'
import { todayISO } from './filterTasks'
import { compareText } from '@/shared/lib/intl'

// Column key used for "this card has no value on the current axis".
export const NONE_KEY = '__none__'

// A derived board column: the same shape the kanban already renders, plus the
// value a card must carry to belong here.
export interface DerivedColumn extends BoardColumnDef {
  // True when this column collects cards with no value on the axis (no agent,
  // no priority, no tag, no due date). Dropping onto it CLEARS the field.
  isNone?: boolean
}

const PRIORITY_COLORS: Record<string, string> = {
  critical: '#ef4444',
  high: '#f59e0b',
  medium: '#3b82f6',
  low: '#6b7280',
}

const DUE_COLORS: Record<BoardDueFilter, string> = {
  overdue: '#ef4444',
  today: '#f59e0b',
  week: '#3b82f6',
  none: '',
}

// dueKeyOf classifies a task into one of the date columns. Unlike the filter's
// bucket this is exhaustive: anything past the week window lands in 'week' so no
// card can silently fall out of the board.
function dueKeyOf(task: Task, today: string): string {
  if (!task.dueDate) return NONE_KEY
  if (task.dueDate < today) return 'overdue'
  if (task.dueDate === today) return 'today'
  return 'week'
}

// columnKeyOf reports which derived column a task belongs to, for a given axis.
// Under the 'tag' axis a task can belong to several columns, hence the array.
export function columnKeysOf(task: Task, groupBy: BoardGroupBy, today: string): string[] {
  switch (groupBy) {
    case 'status':
      return [task.boardState]
    case 'agent':
      return [task.ownerAgentId || NONE_KEY]
    case 'priority':
      return [task.priority || NONE_KEY]
    case 'due':
      return [dueKeyOf(task, today)]
    case 'tag': {
      const tags = task.tags ?? []
      return tags.length > 0 ? tags : [NONE_KEY]
    }
  }
}

// deriveColumns builds the column list for an axis.
//
// 'status' returns the workspace's configured columns verbatim (so custom
// columns and their colors survive). The others are derived from the data, with
// a trailing "none" column when any card lacks a value — dropped otherwise, so
// an axis where everything is set stays clean.
export function deriveColumns(
  groupBy: BoardGroupBy,
  tasks: Task[],
  agents: Agent[],
  boardColumns: BoardColumnDef[],
  now = new Date(),
): DerivedColumn[] {
  const today = todayISO(now)
  const needsNone = tasks.some((t) => columnKeysOf(t, groupBy, today).includes(NONE_KEY))
  const noneCol = (label: string): DerivedColumn => ({
    key: NONE_KEY,
    label,
    color: '',
    isNone: true,
  })

  switch (groupBy) {
    case 'status':
      return boardColumns

    case 'agent': {
      // Only agents that actually own a card, so a workspace with 30 agents and
      // 4 active ones does not render 26 empty columns.
      const used = new Set(tasks.map((t) => t.ownerAgentId).filter(Boolean))
      const cols: DerivedColumn[] = agents
        .filter((a) => used.has(a.id))
        .map((a) => ({ key: a.id, label: a.name, color: a.color ?? '' }))
      // Cards owned by an agent that no longer exists would otherwise vanish.
      for (const id of used) {
        if (!cols.some((c) => c.key === id)) {
          cols.push({ key: id, label: `(silinmiş ajan) ${id.slice(0, 8)}`, color: '' })
        }
      }
      if (needsNone) cols.push(noneCol('Atanmamış'))
      return cols
    }

    case 'priority': {
      const cols: DerivedColumn[] = PRIORITY_ORDER.map((p) => ({
        key: p,
        label: PRIORITY_LABELS[p],
        color: PRIORITY_COLORS[p] ?? '',
      }))
      if (needsNone) cols.push(noneCol(PRIORITY_LABELS['']))
      return cols
    }

    case 'due': {
      const cols: DerivedColumn[] = DUE_ORDER.filter((d) => d !== 'none').map((d) => ({
        key: d,
        label: DUE_LABELS[d],
        color: DUE_COLORS[d],
      }))
      if (needsNone) cols.push(noneCol(DUE_LABELS.none))
      return cols
    }

    case 'tag': {
      const counts = new Map<string, number>()
      for (const t of tasks) {
        for (const tag of t.tags ?? []) counts.set(tag, (counts.get(tag) ?? 0) + 1)
      }
      const cols: DerivedColumn[] = [...counts.keys()]
        .sort((a, b) => compareText(a, b))
        .map((tag) => ({ key: tag, label: `#${tag}`, color: '' }))
      if (needsNone) cols.push(noneCol('Etiketsiz'))
      return cols
    }
  }
}

// dropPatch is the task update a drag onto `columnKey` should produce, for the
// current axis. Returns null when the axis cannot express a drop, so the caller
// can refuse the gesture instead of silently doing nothing.
//
// The 'due' axis returns null deliberately: "make this due today" is a real
// edit, but "make this due sometime this week" is not a well-defined date, and
// guessing one would quietly falsify a deadline.
export function dropPatch(groupBy: BoardGroupBy, columnKey: string, task: Task): TaskPatch | null {
  const isNone = columnKey === NONE_KEY
  switch (groupBy) {
    case 'status':
      return { boardState: columnKey }
    case 'agent':
      return { ownerAgentId: isNone ? '' : columnKey }
    case 'priority':
      return { priority: isNone ? '' : (columnKey as Task['priority']) }
    case 'tag': {
      if (isNone) return { tags: [] }
      const tags = task.tags ?? []
      return tags.includes(columnKey) ? null : { tags: [...tags, columnKey] }
    }
    case 'due':
      return null
  }
}

// Human-readable reason a drop is refused, shown as a transient hint.
export const DROP_REFUSED_REASON: Partial<Record<BoardGroupBy, string>> = {
  due: 'Tarihe göre gruplandırmada kart sürüklenemez — bitiş tarihini kart üzerinden düzenleyin.',
}
