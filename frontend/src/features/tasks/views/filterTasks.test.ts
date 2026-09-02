import { describe, expect, it } from 'vitest'
import type { Task } from '@/types'
import { filterTasks, sortTasks, topoLevels } from './filterTasks'

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

describe('filterTasks', () => {
  const tasks = [
    task({ id: 'a', title: 'Login akışı', priority: 'high', tags: ['ui'], ownerAgentId: 'ag1' }),
    task({ id: 'b', title: 'Veritabanı şeması', priority: 'low', tags: ['db', 'ui'] }),
    task({ id: 'c', title: 'Rapor', boardState: 'done' }),
    task({ id: 'd', title: 'Deploy', dependencies: '["c"]' }),
    task({ id: 'e', title: 'Test', dependencies: '["d"]' }),
  ]
  const ids = (f: Parameters<typeof filterTasks>[1]) => filterTasks(tasks, f).map((t) => t.id)

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
    const find = (text: string) => filterTasks(tr, { text }).map((t) => t.id)
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
    expect(filterTasks(orphan, { dep: 'blocked' })).toEqual([])
    expect(filterTasks(orphan, { dep: 'ready' })).toEqual([])
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
})

describe('filterTasks · review facet', () => {
  const tasks = [
    task({ id: 'clean' }),
    task({ id: 'once', reviewBounces: 1 }),
    task({ id: 'spent', reviewBounces: 3 }),
    task({ id: 'over', reviewBounces: 5 }),
  ]

  it('is inactive when unset', () => {
    expect(filterTasks(tasks, {}).map((t) => t.id)).toEqual(['clean', 'once', 'spent', 'over'])
  })

  it('bounced keeps every card that came back from review at least once', () => {
    expect(filterTasks(tasks, { review: 'bounced' }).map((t) => t.id)).toEqual([
      'once',
      'spent',
      'over',
    ])
  })

  it('exhausted keeps only cards at or past the round budget', () => {
    expect(filterTasks(tasks, { review: 'exhausted' }).map((t) => t.id)).toEqual(['spent', 'over'])
  })
})
