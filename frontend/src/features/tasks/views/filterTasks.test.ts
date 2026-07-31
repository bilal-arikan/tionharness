import { describe, expect, it } from 'vitest'
import type { Task } from '@/types'
import { filterTasks, sortTasks, todayISO, topoLevels } from './filterTasks'

// A fixed "now" so date-bucket assertions are stable regardless of when the
// suite runs.
const NOW = new Date(2026, 6, 31) // 2026-07-31, local time
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

describe('todayISO', () => {
  it('formats the local date, not UTC', () => {
    // 23:30 local on the 31st must still read as the 31st; a UTC-based
    // implementation would roll over to the 1st for positive offsets.
    expect(todayISO(new Date(2026, 6, 31, 23, 30))).toBe('2026-07-31')
  })
})

describe('filterTasks', () => {
  const tasks = [
    task({ id: 'a', title: 'Login akışı', priority: 'high', tags: ['ui'], ownerAgentId: 'ag1' }),
    task({ id: 'b', title: 'Veritabanı şeması', priority: 'low', tags: ['db', 'ui'] }),
    task({ id: 'c', title: 'Rapor', boardState: 'done', dueDate: '2026-07-01' }),
    task({ id: 'd', title: 'Deploy', dueDate: TODAY, dependencies: '["c"]' }),
    task({ id: 'e', title: 'Test', dependencies: '["d"]' }),
  ]
  const ids = (f: Parameters<typeof filterTasks>[1]) => filterTasks(tasks, f, NOW).map((t) => t.id)

  it('returns everything for an empty filter', () => {
    expect(ids({})).toEqual(['a', 'b', 'c', 'd', 'e'])
  })

  it('matches text case-insensitively over title and description', () => {
    expect(ids({ text: 'LOGIN' })).toEqual(['a'])
    expect(ids({ text: 'şema' })).toEqual(['b'])
  })

  it('treats dotted and dotless I as interchangeable', () => {
    // Turkish lowercasing would turn 'I' into 'ı' and lose the match; invariant
    // lowercasing would turn 'İ' into 'i'+combining dot and lose the other one.
    const tr = [task({ id: 'i1', title: 'İstanbul' }), task({ id: 'i2', title: 'Ilık' })]
    const find = (text: string) => filterTasks(tr, { text }, NOW).map((t) => t.id)
    expect(find('istanbul')).toEqual(['i1'])
    expect(find('İSTANBUL')).toEqual(['i1'])
    expect(find('ilik')).toEqual(['i2'])
    expect(find('ILIK')).toEqual(['i2'])
  })

  it('ORs within a facet and ANDs across facets', () => {
    expect(ids({ priorities: ['high', 'low'] })).toEqual(['a', 'b'])
    expect(ids({ priorities: ['high', 'low'], tags: ['db'] })).toEqual(['b'])
  })

  it('treats the "-" agent sentinel as unassigned', () => {
    expect(ids({ agentIds: ['-'] })).toEqual(['b', 'c', 'd', 'e'])
    expect(ids({ agentIds: ['ag1'] })).toEqual(['a'])
  })

  it('buckets due dates against the local today', () => {
    expect(ids({ dues: ['overdue'] })).toEqual(['c'])
    expect(ids({ dues: ['today'] })).toEqual(['d'])
    expect(ids({ dues: ['overdue', 'today'] })).toEqual(['c', 'd'])
    expect(ids({ dues: ['none'] })).toEqual(['a', 'b', 'e'])
  })

  it('classifies dependency state against the full list', () => {
    // d depends on c, which is done → ready. e depends on d, which is not → blocked.
    expect(ids({ dep: 'ready' })).toEqual(['d'])
    expect(ids({ dep: 'blocked' })).toEqual(['e'])
  })

  it('keeps blocked-ness correct when the blocker is itself filtered out', () => {
    // Filtering to the "todo" column hides d, but e must still read as blocked.
    expect(ids({ columns: ['todo'], dep: 'blocked' })).toEqual(['e'])
  })

  it('ignores a dependency id that no longer resolves', () => {
    const orphan = [task({ id: 'x', dependencies: '["gone"]' })]
    expect(filterTasks(orphan, { dep: 'blocked' }, NOW)).toEqual([])
    expect(filterTasks(orphan, { dep: 'ready' }, NOW)).toEqual([])
  })
})

describe('topoLevels', () => {
  it('places unblocked tasks at level 0 and dependents above', () => {
    const lv = topoLevels([
      task({ id: 'a' }),
      task({ id: 'b', dependencies: '["a"]' }),
      task({ id: 'c', dependencies: '["b"]' }),
    ])
    expect([lv.get('a'), lv.get('b'), lv.get('c')]).toEqual([0, 1, 2])
  })

  it('terminates on a dependency cycle', () => {
    const lv = topoLevels([
      task({ id: 'a', dependencies: '["b"]' }),
      task({ id: 'b', dependencies: '["a"]' }),
    ])
    expect(lv.size).toBe(2)
  })
})

describe('sortTasks', () => {
  it('orders by priority with unset last, breaking ties by recency', () => {
    const out = sortTasks(
      [
        task({ id: 'none', updatedAt: 5 }),
        task({ id: 'low', priority: 'low', updatedAt: 1 }),
        task({ id: 'crit', priority: 'critical', updatedAt: 1 }),
      ],
      'priority',
      null,
    )
    expect(out.map((t) => t.id)).toEqual(['crit', 'low', 'none'])
  })

  it('sinks undated tasks to the end of a due sort', () => {
    const out = sortTasks(
      [
        task({ id: 'undated' }),
        task({ id: 'late', dueDate: '2026-12-01' }),
        task({ id: 'soon', dueDate: '2026-08-01' }),
      ],
      'due',
      null,
    )
    expect(out.map((t) => t.id)).toEqual(['soon', 'late', 'undated'])
  })
})
