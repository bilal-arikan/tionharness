import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { shouldDiscardFreshSession } from '@/app/freshSessionCleanup'

const values = new Map<string, string>()
const storage = {
  get length() {
    return values.size
  },
  clear: () => values.clear(),
  getItem: (key: string) => values.get(key) ?? null,
  key: (index: number) => [...values.keys()][index] ?? null,
  removeItem: (key: string) => values.delete(key),
  setItem: (key: string, value: string) => values.set(key, value),
} satisfies Storage

let setActiveWorkspace: (id: string) => void
let readSessionDraftState: typeof import('./sessionDrafts').readSessionDraftState
let writeSessionDraft: (sessionId: string | undefined, text: string) => void

beforeAll(async () => {
  vi.stubGlobal('localStorage', storage)
  ;({ setActiveWorkspace } = await import('@/api'))
  ;({ readSessionDraftState, writeSessionDraft } = await import('./sessionDrafts'))
})

beforeEach(() => storage.clear())

describe('workspace-scoped session drafts', () => {
  it('does not preserve a fresh session using another workspace draft', () => {
    setActiveWorkspace('WS1')
    writeSessionDraft('SES1', 'unfinished prompt')

    setActiveWorkspace('WS2')
    expect(
      shouldDiscardFreshSession({
        freshSessionId: 'SES1',
        leavingSessionId: 'SES1',
        messageCount: 0,
        draft: readSessionDraftState('SES1'),
      }),
    ).toBe(true)
  })

  it('reports an unreadable draft instead of treating it as empty', () => {
    setActiveWorkspace('WS1')
    const getItem = vi.spyOn(storage, 'getItem').mockImplementationOnce(() => {
      throw new Error('storage unavailable')
    })

    expect(readSessionDraftState('SES1')).toEqual({ ok: false, value: '' })
    getItem.mockRestore()
  })
})
