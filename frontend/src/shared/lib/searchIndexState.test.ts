import { describe, expect, it } from 'vitest'
import type { SearchIndexStatus } from '@/types'
import {
  canActOn,
  formatIndexTime,
  isAnyIndexing,
  phaseLook,
  refreshVerb,
} from './searchIndexState'

function row(patch: Partial<SearchIndexStatus> = {}): SearchIndexStatus {
  return { tool: 'zg', root: '/repo', phase: 'ready', usable: true, ...patch }
}

describe('phaseLook', () => {
  it('gives a failed index its own tone, never the ready one', () => {
    expect(phaseLook('failed').tone).not.toBe(phaseLook('ready').tone)
  })

  it('shows an unknown phase raw instead of guessing it is ready', () => {
    const look = phaseLook('quantum')
    expect(look.label).toBe('quantum')
    expect(look.tone).not.toBe(phaseLook('ready').tone)
  })
})

describe('isAnyIndexing', () => {
  it('is the poll condition: true only while a run is in flight', () => {
    expect(isAnyIndexing([row(), row({ phase: 'stale' })])).toBe(false)
    expect(isAnyIndexing([row(), row({ phase: 'indexing' })])).toBe(true)
    expect(isAnyIndexing([])).toBe(false)
  })
})

describe('canActOn', () => {
  it('blocks actions on a row that is already indexing', () => {
    // The backend answers 409 for a second run, so offering the button would
    // only produce an error the user cannot act on.
    expect(canActOn(row({ phase: 'indexing' }))).toBe(false)
  })

  it('allows acting on every settled phase, including failed', () => {
    for (const phase of ['ready', 'stale', 'failed', 'missing'] as const) {
      expect(canActOn(row({ phase }))).toBe(true)
    }
  })
})

describe('refreshVerb', () => {
  it('calls a refresh of a missing index what it really is: a create', () => {
    expect(refreshVerb(row({ phase: 'missing' }))).toBe('Create')
    expect(refreshVerb(row({ phase: 'stale' }))).toBe('Refresh')
  })
})

describe('formatIndexTime', () => {
  it('renders an em dash rather than "Invalid Date" for missing or broken input', () => {
    expect(formatIndexTime(undefined)).toBe('—')
    expect(formatIndexTime('')).toBe('—')
    expect(formatIndexTime('not a date')).toBe('—')
  })

  it('formats a real RFC3339 timestamp', () => {
    const out = formatIndexTime('2026-09-22T10:30:00Z', 'en-US')
    expect(out).not.toBe('—')
    expect(out).toMatch(/Sep/)
  })
})
