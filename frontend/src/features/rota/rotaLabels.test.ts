import { describe, expect, it } from 'vitest'
import { laneLiveLabel, laneOriginGlyph } from './rotaLabels'

describe('rota labels', () => {
  it('renders liveness with queue depth', () => {
    expect(laneLiveLabel(undefined)).toBe('')
    expect(laneLiveLabel({ state: 'running' })).toBe('çalışıyor')
    expect(laneLiveLabel({ state: 'queued', waiting: 2 })).toBe('kuyrukta +2')
    expect(laneLiveLabel({ state: 'odd' })).toBe('odd')
  })

  it('picks the origin glyph', () => {
    const base = {
      id: 'S',
      kind: 'chat',
      state: 'active',
      rootSessionId: '',
      createdAt: 1,
      updatedAt: 1,
    }
    expect(laneOriginGlyph(base)).toBe('●')
    expect(laneOriginGlyph({ ...base, coordinator: true })).toBe('◎')
    expect(laneOriginGlyph({ ...base, origin: { kind: 'coordinator', at: 1 } })).toBe('↳')
    expect(laneOriginGlyph({ ...base, origin: { kind: 'automation', at: 1 } })).toBe('⚡')
  })
})
