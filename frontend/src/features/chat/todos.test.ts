import { describe, expect, it } from 'vitest'
import type { Message, TodoItem, TurnStep } from '@/types'
import {
  dismissCompletedTodo,
  isCompletedTodoDismissed,
  latestTodos,
  MAX_TODO_DISMISSALS,
  TODO_DISMISSALS_STORAGE_KEY,
} from './todos'

const completed: TodoItem[] = [{ content: 'Ship it', status: 'completed' }]
const pending: TodoItem[] = [{ content: 'Keep working', status: 'pending' }]

function message(
  role: Message['role'],
  steps: TurnStep[] = [],
  id = `${role}-${Math.random()}`,
): Message {
  return {
    id,
    sessionId: 'SES1',
    role,
    text: '',
    steps: JSON.stringify(steps),
    createdAt: 1,
  }
}

function todoStep(todos: TodoItem[]): TurnStep {
  return { kind: 'todo', todos }
}

function memoryStorage(initial: string | null = null) {
  let value = initial
  return {
    getItem: () => value,
    setItem: (_key: string, next: string) => {
      value = next
    },
    value: () => value,
  }
}

describe('latestTodos', () => {
  it('keeps a completed list after a later user message', () => {
    expect(
      latestTodos([message('assistant', [todoStep(completed)]), message('user')])?.todos,
    ).toEqual(completed)
  })

  it('returns the newest todo list', () => {
    expect(
      latestTodos([
        message('assistant', [todoStep(pending)]),
        message('assistant', [todoStep(completed)]),
      ])?.todos,
    ).toEqual(completed)
  })

  it('reads legacy todo_write input', () => {
    expect(
      latestTodos([
        message('assistant', [{ kind: 'tool', tool: 'todo_write', input: { todos: pending } }]),
      ])?.todos,
    ).toEqual(pending)
  })

  it('returns a stable identity per todo_write occurrence', () => {
    const first = latestTodos([message('assistant', [todoStep(completed)], 'MSG1')])
    const reloaded = latestTodos([message('assistant', [todoStep(completed)], 'MSG1')])
    const second = latestTodos([
      message('assistant', [todoStep(completed)], 'MSG1'),
      message('assistant', [todoStep(completed)], 'MSG2'),
    ])

    expect(reloaded?.occurrenceId).toBe(first?.occurrenceId)
    expect(second?.occurrenceId).not.toBe(first?.occurrenceId)
  })

  it('shows a new identical todo list after the earlier occurrence was dismissed', () => {
    const storage = memoryStorage()
    const first = latestTodos([message('assistant', [todoStep(completed)], 'MSG1')])
    const second = latestTodos([
      message('assistant', [todoStep(completed)], 'MSG1'),
      message('assistant', [todoStep(completed)], 'MSG2'),
    ])
    if (!first || !second) throw new Error('Expected todo occurrences')

    dismissCompletedTodo(storage, 'SES1', first.occurrenceId, first.todos)

    expect(isCompletedTodoDismissed(storage, 'SES1', second.occurrenceId, second.todos)).toBe(false)
  })
})

describe('completed todo dismissals', () => {
  it('isolates dismissals by session and list signature', () => {
    const storage = memoryStorage()
    dismissCompletedTodo(storage, 'SES1', 'OCC1', completed)

    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC1', completed)).toBe(true)
    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC2', completed)).toBe(false)
    expect(isCompletedTodoDismissed(storage, 'SES2', 'OCC1', completed)).toBe(false)
    expect(
      isCompletedTodoDismissed(storage, 'SES1', 'OCC1', [
        { content: 'Different', status: 'completed' },
      ]),
    ).toBe(false)
    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC1', pending)).toBe(false)
    expect(storage.value()).toContain('SES1')
  })

  it('treats malformed storage as empty and replaces it safely', () => {
    const storage = memoryStorage('{broken')
    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC1', completed)).toBe(false)

    dismissCompletedTodo(storage, 'SES1', 'OCC1', completed)
    expect(() => JSON.parse(storage.value() ?? '')).not.toThrow()
    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC1', completed)).toBe(true)
  })

  it('survives storage access failures', () => {
    const storage = {
      getItem: () => {
        throw new Error('blocked')
      },
      setItem: () => {
        throw new Error('blocked')
      },
    }
    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC1', completed)).toBe(false)
    expect(() => dismissCompletedTodo(storage, 'SES1', 'OCC1', completed)).not.toThrow()
  })

  it('retains only the newest dismissal records', () => {
    const storage = memoryStorage()
    for (let index = 0; index <= MAX_TODO_DISMISSALS; index++) {
      dismissCompletedTodo(storage, 'SES1', `OCC${index}`, completed)
    }

    const stored = JSON.parse(storage.value() ?? '[]') as string[]
    expect(stored).toHaveLength(MAX_TODO_DISMISSALS)
    expect(isCompletedTodoDismissed(storage, 'SES1', 'OCC0', completed)).toBe(false)
    expect(isCompletedTodoDismissed(storage, 'SES1', `OCC${MAX_TODO_DISMISSALS}`, completed)).toBe(
      true,
    )
  })

  it('uses the dedicated storage key', () => {
    expect(TODO_DISMISSALS_STORAGE_KEY).toBe('tionharness.completedTodoDismissals')
  })
})
