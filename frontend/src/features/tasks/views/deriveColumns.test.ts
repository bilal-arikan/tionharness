import { describe, expect, it } from 'vitest'
import type { Agent, BoardColumnDef, Task } from '@/types'
import { NONE_KEY, columnKeysOf, deriveColumns, dropPatch } from './deriveColumns'

const NOW = new Date(2026, 6, 31)
const TODAY = '2026-07-31'

function task(over: Partial<Task> & { id: string }): Task {
  return {
    title: '',
    description: '',
    prompt: '',
    ownerAgentId: '',
    flowId: '',
    boardState: 'todo',
    dependencies: '[]',
    lastRunId: '',
    lastRunStatus: '',
    lastRunAt: 0,
    createdAt: 0,
    updatedAt: 0,
    ...over,
  }
}

const agents = [
  { id: 'ag1', name: 'Planlayıcı' },
  { id: 'ag2', name: 'Boşta' },
] as Agent[]

const statusCols: BoardColumnDef[] = [
  { key: 'todo', label: 'Yapılacak', color: '' },
  { key: 'done', label: 'Bitti', color: '' },
]

describe('deriveColumns', () => {
  it('returns the workspace columns verbatim on the status axis', () => {
    expect(deriveColumns('status', [], agents, statusCols, NOW)).toBe(statusCols)
  })

  it('only lists agents that own a card', () => {
    const cols = deriveColumns(
      'agent',
      [task({ id: 'a', ownerAgentId: 'ag1' })],
      agents,
      statusCols,
      NOW,
    )
    expect(cols.map((c) => c.key)).toEqual(['ag1'])
  })

  it('keeps cards owned by a deleted agent visible in their own column', () => {
    const cols = deriveColumns(
      'agent',
      [task({ id: 'a', ownerAgentId: 'ghost' })],
      agents,
      statusCols,
      NOW,
    )
    expect(cols.map((c) => c.key)).toEqual(['ghost'])
  })

  it('appends the none column only when some card lacks a value', () => {
    const withNone = deriveColumns('priority', [task({ id: 'a' })], agents, statusCols, NOW)
    expect(withNone.at(-1)?.key).toBe(NONE_KEY)
    const withoutNone = deriveColumns(
      'priority',
      [task({ id: 'a', priority: 'high' })],
      agents,
      statusCols,
      NOW,
    )
    expect(withoutNone.some((c) => c.key === NONE_KEY)).toBe(false)
  })
})

describe('columnKeysOf', () => {
  it('puts a multi-tagged card in every tag column', () => {
    expect(columnKeysOf(task({ id: 'a', tags: ['ui', 'db'] }), 'tag', TODAY)).toEqual(['ui', 'db'])
  })

  it('routes anything past this week into the week column, never off-board', () => {
    expect(columnKeysOf(task({ id: 'a', dueDate: '2027-01-01' }), 'due', TODAY)).toEqual(['week'])
  })
})

describe('dropPatch', () => {
  it('writes the field the axis names', () => {
    const t = task({ id: 'a' })
    expect(dropPatch('status', 'done', t)).toEqual({ boardState: 'done' })
    expect(dropPatch('agent', 'ag1', t)).toEqual({ ownerAgentId: 'ag1' })
    expect(dropPatch('priority', 'high', t)).toEqual({ priority: 'high' })
  })

  it('clears the field when dropped on the none column', () => {
    const t = task({ id: 'a', ownerAgentId: 'ag1', priority: 'high', tags: ['ui'] })
    expect(dropPatch('agent', NONE_KEY, t)).toEqual({ ownerAgentId: '' })
    expect(dropPatch('priority', NONE_KEY, t)).toEqual({ priority: '' })
    expect(dropPatch('tag', NONE_KEY, t)).toEqual({ tags: [] })
  })

  it('appends a tag but no-ops when the card already carries it', () => {
    expect(dropPatch('tag', 'db', task({ id: 'a', tags: ['ui'] }))).toEqual({ tags: ['ui', 'db'] })
    expect(dropPatch('tag', 'ui', task({ id: 'a', tags: ['ui'] }))).toBeNull()
  })

  it('refuses a drop on the date axis rather than inventing a deadline', () => {
    expect(dropPatch('due', 'week', task({ id: 'a' }))).toBeNull()
  })
})
