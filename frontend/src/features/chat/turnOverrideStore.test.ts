import { beforeEach, describe, expect, it } from 'vitest'
import { readSessionOverride, writeSessionOverride } from './turnOverrideStore'

const KEY = 'test.override'

// The suite runs in a node environment, so stand in a minimal localStorage.
const store = new Map<string, string>()
globalThis.localStorage = {
  getItem: (k: string) => store.get(k) ?? null,
  setItem: (k: string, v: string) => void store.set(k, v),
  removeItem: (k: string) => void store.delete(k),
  clear: () => store.clear(),
  key: (i: number) => [...store.keys()][i] ?? null,
  get length() {
    return store.size
  },
} as Storage

describe('turnOverrideStore', () => {
  beforeEach(() => localStorage.clear())

  it('returns empty for an unknown session', () => {
    expect(readSessionOverride(KEY, 'SES1')).toBe('')
  })

  it('keeps each session independent', () => {
    writeSessionOverride(KEY, 'SES1', 'high')
    writeSessionOverride(KEY, 'SES2', 'low')
    expect(readSessionOverride(KEY, 'SES1')).toBe('high')
    expect(readSessionOverride(KEY, 'SES2')).toBe('low')
    expect(readSessionOverride(KEY, 'SES3')).toBe('')
  })

  it('ignores a blank session id', () => {
    writeSessionOverride(KEY, '', 'high')
    expect(localStorage.getItem(KEY)).toBeNull()
    expect(readSessionOverride(KEY, '')).toBe('')
  })

  it('recovers from a corrupted entry', () => {
    localStorage.setItem(KEY, 'not json')
    expect(readSessionOverride(KEY, 'SES1')).toBe('')
  })

  it('prunes the oldest sessions past the cap', () => {
    for (let i = 0; i < 205; i++) writeSessionOverride(KEY, `SES${i}`, 'high')
    expect(readSessionOverride(KEY, 'SES0')).toBe('')
    expect(readSessionOverride(KEY, 'SES204')).toBe('high')
  })
})
