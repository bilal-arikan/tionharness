import { afterEach, describe, expect, it } from 'vitest'
import type { BoardColumnDef } from '@/types'
import { i18next } from '@/i18n'
import { canonicalizeBoardColumns, localizeBoardColumns } from './boardColumns'
import { getBuiltinViews, priorityLabel } from './views/boardViewTypes'
import { reviewGateBadge } from './reviewGate'

const canonical: BoardColumnDef[] = [
  { key: 'todo', label: 'Yapılacak', color: '' },
  { key: 'review', label: 'İnceleme', color: '#abcdef' },
]

afterEach(async () => {
  await i18next.changeLanguage('tr')
})

describe('localizeBoardColumns', () => {
  it('translates built-in display labels without changing their stored keys or colors', async () => {
    await i18next.changeLanguage('en')

    expect(localizeBoardColumns(canonical)).toEqual([
      { key: 'todo', label: 'To Do', color: '' },
      { key: 'review', label: 'Review', color: '#abcdef' },
    ])
  })

  it('preserves labels supplied by the user', async () => {
    await i18next.changeLanguage('en')
    const custom = [{ key: 'todo', label: 'Next Up', color: '' }]

    expect(localizeBoardColumns(custom)).toEqual(custom)
  })

  it('does not persist a translated display label over the canonical built-in value', async () => {
    await i18next.changeLanguage('en')
    const displayed = localizeBoardColumns(canonical)

    expect(canonicalizeBoardColumns(displayed, canonical)).toEqual(canonical)
    expect(
      canonicalizeBoardColumns([{ key: 'todo', label: 'Next Up', color: '' }], canonical),
    ).toEqual([{ key: 'todo', label: 'Next Up', color: '' }])
  })

  it('reads view, priority, and review labels from the active language at call time', async () => {
    await i18next.changeLanguage('en')
    expect(getBuiltinViews()[0].label).toBe('All')
    expect(priorityLabel('high')).toBe('High')
    expect(reviewGateBadge(1)?.title).toContain('returned from review 1 time')

    await i18next.changeLanguage('tr')
    expect(getBuiltinViews()[0].label).toBe('Tümü')
    expect(priorityLabel('high')).toBe('Yüksek')
    expect(reviewGateBadge(1)?.title).toContain('1 kez geri döndü')
  })
})
