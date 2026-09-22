import { describe, expect, it } from 'vitest'
import {
  kindMeta,
  sessionChipKey,
  sessionMatchesChips,
  SUBAGENT_CHIP,
  WORKER_CHIP,
} from './sessionKindMeta'
import { originLabel } from '@/shared/lib/sessionOrigin'

// TSK1005: a session an agent opens with spawn_session is an ordinary chat; only
// real delegations (run_subagent, spawn_worker) carry a delegation label.
describe('spawn_session child labelling', () => {
  const spawnChild = {
    kind: 'chat',
    category: 'chat',
    executionType: 'interactive',
    isWorker: false,
    isArchived: false,
  }

  it('files a spawn_session child under the Sohbet chip', () => {
    expect(sessionChipKey(spawnChild)).toBe('chat')
    expect(kindMeta(sessionChipKey(spawnChild)).label).toBe('Sohbet')
    expect(sessionMatchesChips(spawnChild, new Set(['chat']))).toBe(true)
  })

  it('keeps run_subagent and spawn_worker children on their own chips', () => {
    expect(
      sessionChipKey({ kind: 'subagent', category: 'subagent', executionType: 'subagent' }),
    ).toBe(SUBAGENT_CHIP)
    expect(sessionChipKey({ kind: 'worker', category: 'worker', executionType: 'worker' })).toBe(
      WORKER_CHIP,
    )
  })

  it('still attributes the spawn to the session that started it', () => {
    const label = originLabel({ kind: 'spawn', triggerSessionId: 'SES7', at: 1 })
    expect(label?.sessionId).toBe('SES7')
    expect(label?.text).toContain('SES7')
  })
})
