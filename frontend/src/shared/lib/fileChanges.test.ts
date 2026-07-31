import { describe, it, expect } from 'vitest'
import type { TurnStep } from '@/types'
import { extractFileChanges, groupByPath, hasFileChanges, changeTotals } from './fileChanges'
import { parseDiff, foldableRanges, diffRows } from './diff'

const diffStep = (path: string, patch: string, extra: Partial<TurnStep> = {}): TurnStep => ({
  kind: 'diff',
  tool: 'Write',
  path,
  patch,
  added: 1,
  removed: 0,
  ...extra,
})

const cliEdit = (
  path: string,
  oldS: string,
  newS: string,
  extra: Partial<TurnStep> = {},
): TurnStep => ({
  kind: 'tool',
  tool: 'Edit',
  input: { file_path: path, old_string: oldS, new_string: newS },
  ...extra,
})

describe('extractFileChanges', () => {
  it('takes a recorded patch off a diff step as-is', () => {
    const [c] = extractFileChanges([diffStep('/a.go', '+hello')])
    expect(c.path).toBe('/a.go')
    expect(c.patch).toBe('+hello')
    expect(c.synthesized).toBe(false)
  })

  it('synthesizes a patch for the claude-cli path, where none was recorded', () => {
    const [c] = extractFileChanges([cliEdit('/a.go', 'old', 'new')])
    expect(c.path).toBe('/a.go')
    expect(c.patch).toBe('-old\n+new')
    expect(c.added).toBe(1)
    expect(c.removed).toBe(1)
    expect(c.synthesized).toBe(true)
  })

  it('skips errored edits — nothing reached disk', () => {
    expect(extractFileChanges([cliEdit('/a.go', 'old', 'new', { isError: true })])).toHaveLength(0)
    expect(extractFileChanges([diffStep('/a.go', '+x', { isError: true })])).toHaveLength(0)
  })

  it('skips non-mutating tools', () => {
    const read: TurnStep = { kind: 'tool', tool: 'Read', input: { file_path: '/a.go' } }
    expect(extractFileChanges([read])).toHaveLength(0)
  })

  it('descends into a subagent trace and flags what it finds', () => {
    const parent: TurnStep = {
      kind: 'subagent',
      tool: 'run_subagent',
      subSteps: [diffStep('/nested.go', '+x')],
    }
    const out = extractFileChanges([parent])
    expect(out).toHaveLength(1)
    expect(out[0].nested).toBe(true)
  })

  it('treats a cut input as truncated: the synthesized counts undercount', () => {
    const [c] = extractFileChanges([cliEdit('/a.go', 'old', 'new', { inputTruncated: true })])
    expect(c.truncated).toBe(true)
  })

  it('drops a step with neither a path nor a patch', () => {
    const empty: TurnStep = { kind: 'tool', tool: 'Edit', input: {} }
    expect(extractFileChanges([empty])).toHaveLength(0)
  })
})

describe('hasFileChanges', () => {
  it('agrees with extractFileChanges', () => {
    expect(hasFileChanges([diffStep('/a.go', '+x')])).toBe(true)
    expect(hasFileChanges([{ kind: 'text', text: 'hi' }])).toBe(false)
  })

  it('still finds edits an errored subagent applied before it failed', () => {
    const parent: TurnStep = {
      kind: 'subagent',
      tool: 'run_subagent',
      isError: true,
      subSteps: [diffStep('/nested.go', '+x')],
    }
    expect(hasFileChanges([parent])).toBe(true)
  })
})

describe('groupByPath', () => {
  it('sums repeated edits to one file and keeps first-touch order', () => {
    const changes = extractFileChanges([
      diffStep('/b.go', '+1'),
      diffStep('/a.go', '+1'),
      diffStep('/b.go', '+1'),
    ])
    const groups = groupByPath(changes)
    expect(groups.map((g) => g.path)).toEqual(['/b.go', '/a.go'])
    expect(groups[0].changes).toHaveLength(2)
    expect(groups[0].added).toBe(2)
  })
})

describe('changeTotals', () => {
  it('counts distinct files, not changes', () => {
    const changes = extractFileChanges([diffStep('/a.go', '+1'), diffStep('/a.go', '+1')])
    expect(changeTotals(changes).files).toBe(1)
    expect(changeTotals(changes).added).toBe(2)
  })
})

describe('context folding', () => {
  const patch = (ctx: number) =>
    ['+added', ...Array.from({ length: ctx }, (_, i) => ` ctx${i}`), '-removed'].join('\n')

  it('folds a long unchanged run, keeping context on both sides', () => {
    const { lines } = parseDiff(patch(20))
    const [range] = foldableRanges(lines)
    // Line 0 is the addition; the context run is 1..20 inclusive, so 3 lines are
    // kept at each edge and the middle 14 hide.
    expect(range).toEqual({ start: 4, end: 18 })
  })

  it('leaves a short run alone — an expander would cost more than it saves', () => {
    expect(foldableRanges(parseDiff(patch(6)).lines)).toHaveLength(0)
  })

  it('collapses a folded run to one row and restores it when expanded', () => {
    const { lines } = parseDiff(patch(20))
    const ranges = foldableRanges(lines)
    const collapsed = diffRows(lines.length, ranges, new Set())
    // 22 lines − 14 hidden + 1 marker.
    expect(collapsed).toHaveLength(9)
    expect(collapsed.filter((r) => r.kind === 'fold')).toHaveLength(1)
    expect(diffRows(lines.length, ranges, new Set([0]))).toHaveLength(lines.length)
  })
})
