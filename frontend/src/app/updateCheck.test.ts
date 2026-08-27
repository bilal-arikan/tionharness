import { beforeEach, describe, expect, it } from 'vitest'
import type { UpdateStatus } from '@/types'
import {
  UPDATE_DISMISS_KEY,
  readDismissedVersion,
  shouldShowUpdate,
  writeDismissedVersion,
} from './updateCheck'

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

const ok: UpdateStatus = {
  state: 'ok',
  current: '0.1.0',
  latest: '0.2.0',
  updateAvailable: true,
  notesUrl: 'https://example.test/notes',
  releasedAt: '2026-08-01T00:00:00Z',
  checkedAt: '2026-08-27T00:00:00Z',
  downloadUrl: 'https://example.test/download/tionharness_0.2.0_windows_amd64.zip',
  downloadFile: 'tionharness_0.2.0_windows_amd64.zip',
}

describe('shouldShowUpdate', () => {
  it.each([
    ['a fresh available update', ok, '', true],
    ['no status yet', null, '', false],
    ['dev build (skipped check)', { ...ok, state: 'skipped' as const }, '', false],
    ['unreachable feed', { ...ok, state: 'unknown' as const }, '', false],
    ['already up to date', { ...ok, updateAvailable: false }, '', false],
    ['dismissed this exact version', ok, '0.2.0', false],
    // The point of per-version dismissal: an older dismissal must not hide a
    // newer release.
    ['dismissed an older version', ok, '0.1.5', true],
  ] satisfies ReadonlyArray<[string, UpdateStatus | null, string, boolean]>)(
    '%s',
    (_, status, dismissed, expected) => {
      expect(shouldShowUpdate(status, dismissed)).toBe(expected)
    },
  )
})

describe('dismissal persistence', () => {
  beforeEach(() => localStorage.clear())

  it('round-trips the dismissed version', () => {
    expect(readDismissedVersion()).toBe('')
    writeDismissedVersion('0.2.0')
    expect(localStorage.getItem(UPDATE_DISMISS_KEY)).toBe('0.2.0')
    expect(readDismissedVersion()).toBe('0.2.0')
  })

  it('hides only the dismissed version, not a later one', () => {
    writeDismissedVersion('0.2.0')
    const dismissed = readDismissedVersion()
    expect(shouldShowUpdate(ok, dismissed)).toBe(false)
    expect(shouldShowUpdate({ ...ok, latest: '0.3.0' }, dismissed)).toBe(true)
  })
})
