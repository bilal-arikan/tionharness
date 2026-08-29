// Derives the agent's current working checklist for a session from its message
// trace. The todo_write tool emits a 'todo' step each time it runs (the full
// list, with updated statuses); the most recent one is the live list. Steps are
// persisted on messages, so this survives a page reload.
import type { Message, TodoItem, TurnStep } from '@/types'
import { parseSteps } from './turnStepUtils'

// readStepTodos pulls checklist items from a 'todo' step (or a legacy
// 'todo_write' tool step whose JSON input still carries them).
function readStepTodos(step: TurnStep): TodoItem[] {
  if (step.kind !== 'todo' && step.tool !== 'todo_write') return []
  if (step.todos?.length) return step.todos
  const input = step.input
  if (input && typeof input === 'object') {
    const todos = (input as { todos?: unknown }).todos
    if (Array.isArray(todos)) {
      return todos.filter((t): t is TodoItem => !!t && typeof (t as TodoItem).content === 'string')
    }
  }
  return []
}

// latestTodos returns the most recent checklist in the session — the agent's
// current todo list — by scanning messages (and their steps) newest-first.
export interface TodoOccurrence {
  todos: TodoItem[]
  occurrenceId: string
}

export function latestTodos(messages: Message[]): TodoOccurrence | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    const steps = parseSteps(messages[i].steps)
    for (let j = steps.length - 1; j >= 0; j--) {
      const todos = readStepTodos(steps[j])
      if (!todos.length) continue
      const todoOrdinal =
        steps.slice(0, j + 1).filter((step) => readStepTodos(step).length).length - 1
      return {
        todos,
        occurrenceId: JSON.stringify([messages[i].id, steps[j].id ?? todoOrdinal]),
      }
    }
  }
  return null
}

export const TODO_DISMISSALS_STORAGE_KEY = 'tionharness.completedTodoDismissals'
export const MAX_TODO_DISMISSALS = 100

interface TodoDismissalStorage {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
}

// Status is included so an agent reopening an item produces a different, visible list.
export function todoListSignature(todos: TodoItem[]): string {
  return JSON.stringify(todos.map((todo) => [todo.content, todo.status]))
}

export function todoDismissalKey(
  sessionId: string,
  occurrenceId: string,
  todos: TodoItem[],
): string {
  return JSON.stringify([sessionId, occurrenceId, todoListSignature(todos)])
}

function readDismissals(storage: TodoDismissalStorage): Set<string> {
  try {
    const raw = storage.getItem(TODO_DISMISSALS_STORAGE_KEY)
    if (!raw) return new Set()
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed) || !parsed.every((value) => typeof value === 'string')) {
      return new Set()
    }
    return new Set(parsed)
  } catch {
    return new Set()
  }
}

export function isCompletedTodoDismissed(
  storage: TodoDismissalStorage,
  sessionId: string,
  occurrenceId: string,
  todos: TodoItem[],
): boolean {
  if (!todos.length || !todos.every((todo) => todo.status === 'completed')) return false
  return readDismissals(storage).has(todoDismissalKey(sessionId, occurrenceId, todos))
}

export function dismissCompletedTodo(
  storage: TodoDismissalStorage,
  sessionId: string,
  occurrenceId: string,
  todos: TodoItem[],
): void {
  if (!todos.length || !todos.every((todo) => todo.status === 'completed')) return
  const key = todoDismissalKey(sessionId, occurrenceId, todos)
  const dismissals = [...readDismissals(storage)].filter((dismissal) => dismissal !== key)
  dismissals.push(key)
  const retainedDismissals = dismissals.slice(-MAX_TODO_DISMISSALS)
  try {
    storage.setItem(TODO_DISMISSALS_STORAGE_KEY, JSON.stringify(retainedDismissals))
  } catch {
    // Storage can be unavailable (privacy mode/quota). Keep panel usable in-memory.
  }
}
