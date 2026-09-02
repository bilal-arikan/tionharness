import { describe, expect, it } from 'vitest'
import { originLabel } from './sessionOrigin'

describe('originLabel', () => {
  it('says nothing for a user-started session', () => {
    expect(originLabel(undefined)).toBeNull()
    expect(originLabel({ kind: 'user', at: 1 })).toBeNull()
  })

  it('names the automation and offers the trigger session as the jump target', () => {
    const l = originLabel({ kind: 'automation', entityId: 'AUT4', triggerSessionId: 'SES9', at: 1 })
    expect(l?.text).toBe('Otomasyon AUT4 tetikledi')
    expect(l?.sessionId).toBe('SES9')
    expect(l?.entityId).toBe('AUT4')
    expect(l?.title).toContain('SES9')
  })

  it('renders flow run and node ids', () => {
    const l = originLabel({ kind: 'flow', entityId: 'FLW1', runId: 'RUN7', nodeId: 'n2', at: 1 })
    expect(l?.text).toBe('Akış FLW1 · koşu RUN7 · düğüm n2')
  })

  it('points a worker at its coordinator', () => {
    const l = originLabel({
      kind: 'coordinator',
      triggerSessionId: 'SES1',
      rootSessionId: 'SES1',
      at: 1,
    })
    expect(l?.text).toBe('Koordinatör SES1 açtı')
    expect(l?.sessionId).toBe('SES1')
  })

  it('handles a schedule without a trigger session', () => {
    const l = originLabel({ kind: 'schedule', entityId: 'SCH2', at: 1 })
    expect(l?.text).toBe('Zamanlayıcı SCH2 başlattı')
    expect(l?.sessionId).toBeUndefined()
  })
})
