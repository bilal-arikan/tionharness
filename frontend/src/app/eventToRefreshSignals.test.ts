import { describe, expect, it } from 'vitest'
import type { AppEvent } from '@/types'
import { SIGNAL_EXPLORER, SIGNAL_EXPLORER_LIVE, signalsForEvent } from './eventToRefreshSignals'

const event = (type: string, target?: Record<string, string>): AppEvent => ({
  type,
  level: 'info',
  workspaceId: 'WS1',
  title: '',
  body: '',
  target,
  time: 0,
})

// The map re-pulls its whole structure only when a node or edge can have
// appeared or vanished; state-only events touch the light live layer so the
// physics never re-settles for a glow or a ring.
describe('explorer refresh routing', () => {
  it('sends turn and digest events to the live layer only', () => {
    for (const e of [
      event('chat'),
      event('worker'),
      event('task'),
      event('awareness_digest'),
      event('interaction', { op: 'open', sessionId: 'SES1' }),
    ]) {
      const keys = signalsForEvent(e)
      expect(keys, e.type).toContain(SIGNAL_EXPLORER_LIVE)
      expect(keys, e.type).not.toContain(SIGNAL_EXPLORER)
    }
  })

  it('re-pulls the structure for events that add or remove nodes', () => {
    for (const e of [
      event('session', { op: 'create' }),
      event('session', { op: 'delete' }),
      event('board', { op: 'create' }),
      event('notes'),
      event('spawned'),
    ]) {
      expect(signalsForEvent(e), e.type).toContain(SIGNAL_EXPLORER)
    }
  })

  it('treats metadata-only session ops as live-layer changes', () => {
    const keys = signalsForEvent(event('session', { op: 'message_activity' }))
    expect(keys).toContain(SIGNAL_EXPLORER_LIVE)
    expect(keys).not.toContain(SIGNAL_EXPLORER)
  })
})
