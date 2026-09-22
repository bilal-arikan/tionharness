import { describe, expect, it } from 'vitest'
import { archiveSide, archivedCount } from './archive'

describe('archive helpers', () => {
  const items = [{ id: 'a' }, { id: 'b', archived: true }, { id: 'c', archived: false }]

  it('splits a list into its live and archived sides, never mixed', () => {
    expect(archiveSide(items, false).map((i) => i.id)).toEqual(['a', 'c'])
    expect(archiveSide(items, true).map((i) => i.id)).toEqual(['b'])
  })

  it('counts archived items', () => {
    expect(archivedCount(items)).toBe(1)
    expect(archivedCount([])).toBe(0)
  })
})
