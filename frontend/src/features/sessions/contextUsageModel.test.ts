import { describe, expect, it } from 'vitest'
import type { SessionInfo } from '@/types'
import {
  buildContextRows,
  contextToneColor,
  contextToneOf,
  OTHER_ROW_KEY,
} from './contextUsageModel'

type Filler = SessionInfo['fillers'][number]

function filler(role: string, tokens: number, calibrated = false): Filler {
  return { role, label: role, tokens, count: 1, calibrated }
}

describe('contextToneOf', () => {
  it('groups buckets into three tones', () => {
    expect(contextToneOf('cli-harness')).toBe('overhead')
    expect(contextToneOf('tools')).toBe('overhead')
    expect(contextToneOf('user')).toBe('conversation')
    expect(contextToneOf('assistant')).toBe('conversation')
    expect(contextToneOf('tool-activity')).toBe('traffic')
    expect(contextToneOf('unknown-role')).toBe('traffic')
  })

  it('keeps machine-injected prompts off the accent', () => {
    expect(contextToneOf('auto-prompt')).toBe('traffic')
  })

  it('draws at most three distinct colours', () => {
    const roles = ['cli-harness', 'system', 'tools', 'skills', 'lazy-tools', 'user', 'assistant']
    const colours = new Set(
      [...roles, 'summary', 'auto-prompt', 'tool', 'tool-activity', 'artifacts'].map((role) =>
        contextToneColor(contextToneOf(role)),
      ),
    )
    expect(colours.size).toBe(3)
  })
})

describe('buildContextRows', () => {
  it('folds every bucket under 1% of the window into one trailing row', () => {
    const rows = buildContextRows(
      [
        filler('cli-harness', 28_400),
        filler('tools', 10_400),
        filler('system', 3_400),
        filler('skills', 1_200),
        filler('lazy-tools', 879),
        filler('tool-activity', 735),
        filler('user', 531),
        filler('assistant', 229),
      ],
      350_000,
    )
    expect(rows.map((row) => row.key)).toEqual(['cli-harness', 'tools', 'system', OTHER_ROW_KEY])
    const other = rows.at(-1)!
    expect(other.tokens).toBe(1_200 + 879 + 735 + 531 + 229)
    expect(other.count).toBe(5)
    expect(other.folded?.map((f) => f.role)).toEqual([
      'skills',
      'lazy-tools',
      'tool-activity',
      'user',
      'assistant',
    ])
  })

  it('keeps a lone thin bucket under its own name', () => {
    const rows = buildContextRows([filler('system', 500), filler('user', 3)], 1_000)
    expect(rows.map((row) => row.key)).toEqual(['system', 'user'])
  })

  it('orders rows by tone and keeps the backend order within a tone', () => {
    const rows = buildContextRows(
      [
        filler('tool', 100),
        filler('user', 100),
        filler('system', 100),
        filler('assistant', 100),
        filler('tools', 100),
      ],
      1_000,
    )
    expect(rows.map((row) => row.key)).toEqual(['system', 'tools', 'user', 'assistant', 'tool'])
  })

  it('marks the folded row measured only when every folded bucket is', () => {
    const mixed = buildContextRows([filler('user', 1, true), filler('assistant', 1)], 1_000)
    expect(mixed.at(-1)?.calibrated).toBe(false)
    const measured = buildContextRows(
      [filler('user', 1, true), filler('assistant', 1, true)],
      1_000,
    )
    expect(measured.at(-1)?.calibrated).toBe(true)
  })
})
