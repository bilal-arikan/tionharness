// Derives the agent's current working checklist for a session from its message
// trace. The todo_write tool emits a 'todo' step each time it runs (the full
// list, with updated statuses); the most recent one is the live list. Steps are
// persisted on messages, so this survives a page reload.
import type { Message, TodoItem, TurnStep } from '../types'
import { parseSteps } from '../components/chat/TurnSteps'

// readStepTodos pulls checklist items from a 'todo' step (or a legacy
// 'todo_write' tool step whose JSON input still carries them).
function readStepTodos(step: TurnStep): TodoItem[] {
  if (step.kind !== 'todo' && step.tool !== 'todo_write') return []
  if (step.todos?.length) return step.todos
  const input = step.input
  if (input && typeof input === 'object') {
    const todos = (input as { todos?: unknown }).todos
    if (Array.isArray(todos)) {
      return todos.filter(
        (t): t is TodoItem => !!t && typeof (t as TodoItem).content === 'string',
      )
    }
  }
  return []
}

// latestTodos returns the most recent checklist in the session — the agent's
// current todo list — by scanning messages (and their steps) newest-first.
export function latestTodos(messages: Message[]): TodoItem[] {
  for (let i = messages.length - 1; i >= 0; i--) {
    const steps = parseSteps(messages[i].steps)
    for (let j = steps.length - 1; j >= 0; j--) {
      const todos = readStepTodos(steps[j])
      if (todos.length) return todos
    }
  }
  return []
}
