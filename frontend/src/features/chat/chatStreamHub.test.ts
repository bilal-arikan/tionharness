import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Message } from '@/types'

// The module graph reaches api/client, which reads localStorage at import time.
// The suite runs on the plain node environment (no jsdom dependency), so the
// stub has to exist BEFORE the imports below — hence vi.hoisted.
vi.hoisted(() => {
  const store = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (k: string) => store.get(k) ?? null,
      setItem: (k: string, v: string) => void store.set(k, v),
      removeItem: (k: string) => void store.delete(k),
      clear: () => store.clear(),
    },
  })
})

import { HubKind } from '@/api/sessionStream'
import { makeHubHandlers, type HubApplyCtx } from './chatStreamHub'

const SID = 'SES1'
const GHOST = `live-hub-${SID}`

// harness wires makeHubHandlers to a plain messages array so a test can drive
// hub events and read back exactly what the transcript would render.
function harness() {
  let messages: Message[] = []
  const ctx: HubApplyCtx = {
    sid: SID,
    activeSessionIdRef: { current: SID },
    setMessages: (u) => {
      messages = typeof u === 'function' ? (u as (p: Message[]) => Message[])(messages) : u
    },
    setStreamingSessions: () => {},
    setPendingSessions: () => {},
    setPendingAsks: () => {},
    setQueued: () => {},
    setPresence: () => {},
    setTyping: () => {},
    reload: () => {},
    notifyEnabled: { current: false },
    bumpMeter: () => {},
  }
  const h = makeHubHandlers(ctx)
  let seq = 0
  const send = (kind: string, payload: unknown) =>
    h.onEvent({ kind, payload, seq: ++seq, time: 1000 } as never)
  return {
    h,
    send,
    ghost: () => messages.find((m) => m.id === GHOST),
    all: () => messages,
  }
}

const delta = (text: string) => ({ kind: 'text', text })

describe('chatStreamHub streaming coalescer', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('paints the first delta immediately (no added latency on turn start)', () => {
    const { send, ghost } = harness()
    send(HubKind.Delta, delta('Mer'))
    expect(ghost()?.text).toBe('Mer')
  })

  it('coalesces a burst into one frame instead of one render per token', () => {
    const { send, ghost } = harness()
    send(HubKind.Delta, delta('a')) // leading edge paints
    send(HubKind.Delta, delta('b')) // collapse into the open window
    send(HubKind.Delta, delta('c'))
    expect(ghost()?.text).toBe('a')
    vi.advanceTimersByTime(50)
    expect(ghost()?.text).toBe('abc') // trailing flush carries everything
  })

  it('does not lose the tail when the burst ends mid-window', () => {
    const { send, ghost } = harness()
    send(HubKind.Delta, delta('a'))
    send(HubKind.Delta, delta('b'))
    vi.advanceTimersByTime(500)
    expect(ghost()?.text).toBe('ab')
  })

  it('a trailing flush cannot resurrect a ghost the reply already dropped', () => {
    const { send, ghost, all } = harness()
    send(HubKind.Delta, delta('kismi'))
    send(HubKind.Delta, delta(' devam')) // leaves a pending flush armed
    send(HubKind.Reply, { id: 'M1', sessionId: SID, role: 'assistant', text: 'tam yanit' })
    expect(ghost()).toBeUndefined()
    // The armed timer must not re-append the stale live bubble.
    vi.advanceTimersByTime(500)
    expect(ghost()).toBeUndefined()
    expect(all().map((m) => m.id)).toEqual(['M1'])
  })

  it('turn_done also disarms the pending flush', () => {
    const { send, ghost } = harness()
    send(HubKind.Delta, delta('x'))
    send(HubKind.Delta, delta('y'))
    send(HubKind.TurnDone, {})
    vi.advanceTimersByTime(500)
    expect(ghost()).toBeUndefined()
  })

  it('onClose flushes pending deltas rather than dropping them', () => {
    const { h, send, ghost } = harness()
    send(HubKind.Delta, delta('a'))
    send(HubKind.Delta, delta('b')) // pending, not yet painted
    expect(ghost()?.text).toBe('a')
    h.onClose?.()
    expect(ghost()?.text).toBe('ab')
    // and no timer survives the subscription
    vi.advanceTimersByTime(500)
    expect(ghost()?.text).toBe('ab')
  })

  it('onClose after a finished turn does not conjure a ghost', () => {
    const { h, send, ghost } = harness()
    send(HubKind.Delta, delta('x'))
    send(HubKind.TurnDone, {})
    h.onClose?.()
    expect(ghost()).toBeUndefined()
  })

  it('keeps the turn-start stamp stable across coalesced frames', () => {
    const { send, ghost } = harness()
    send(HubKind.AgentStart, { agentId: 'AGT1', index: 0 })
    const started = ghost()?.createdAt
    send(HubKind.Delta, delta('a'))
    vi.advanceTimersByTime(50)
    send(HubKind.Delta, delta('b'))
    vi.advanceTimersByTime(50)
    expect(ghost()?.createdAt).toBe(started)
  })

  it('ignores events once the window moved to another session', () => {
    let messages: Message[] = []
    const ref = { current: SID as string | null }
    const h = makeHubHandlers({
      sid: SID,
      activeSessionIdRef: ref,
      setMessages: (u) => {
        messages = typeof u === 'function' ? (u as (p: Message[]) => Message[])(messages) : u
      },
      setStreamingSessions: () => {},
      setPendingSessions: () => {},
      setPendingAsks: () => {},
      setQueued: () => {},
      setPresence: () => {},
      setTyping: () => {},
      reload: () => {},
      notifyEnabled: { current: false },
      bumpMeter: () => {},
    })
    h.onEvent({ kind: HubKind.Delta, payload: delta('a'), seq: 1, time: 1 } as never)
    expect(messages).toHaveLength(1)
    ref.current = 'SES2' // user switched away mid-turn
    h.onEvent({ kind: HubKind.Delta, payload: delta('b'), seq: 2, time: 1 } as never)
    vi.advanceTimersByTime(500)
    expect(messages[0].text).toBe('a') // stale flush must not touch the new session
  })
})

describe('tool_delta live output', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('renders streamed tool output, merging chunks by call id', () => {
    const { send, ghost } = harness()
    send(HubKind.ToolDelta, { kind: 'tool_delta', id: 'call-1', tool: 'shell', output: 'satir1\n' })
    send(HubKind.ToolDelta, { kind: 'tool_delta', id: 'call-1', tool: 'shell', output: 'satir2\n' })
    vi.advanceTimersByTime(50)
    const steps = JSON.parse(ghost()?.steps ?? '[]')
    expect(steps).toHaveLength(1) // merged into ONE card, not one per chunk
    expect(steps[0].output).toBe('satir1\nsatir2\n')
  })

  it('keeps concurrent tools on separate cards', () => {
    const { send, ghost } = harness()
    send(HubKind.ToolDelta, { kind: 'tool_delta', id: 'a', tool: 'shell', output: 'A' })
    send(HubKind.ToolDelta, { kind: 'tool_delta', id: 'b', tool: 'grep', output: 'B' })
    vi.advanceTimersByTime(50)
    const steps = JSON.parse(ghost()?.steps ?? '[]')
    expect(steps.map((s: { id: string }) => s.id)).toEqual(['a', 'b'])
  })
})

describe('generic live cards', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('merges append frames into the matching card', () => {
    const { send, ghost } = harness()
    send(HubKind.Step, { kind: 'tool', id: 'call-1', tool: 'Bash', running: true })
    send(HubKind.Step, {
      kind: 'tool',
      id: 'call-1',
      tool: 'Bash',
      output: 'one',
      running: true,
      append: true,
    })
    send(HubKind.Step, {
      kind: 'tool',
      id: 'call-1',
      tool: 'Bash',
      output: 'two',
      running: true,
      append: true,
    })
    vi.advanceTimersByTime(50)

    const steps = JSON.parse(ghost()?.steps ?? '[]')
    expect(steps).toHaveLength(1)
    expect(steps[0].output).toBe('onetwo')
  })

  it('replaces a matching id in place', () => {
    const { send, ghost } = harness()
    send(HubKind.Step, { kind: 'tool', id: 'call-1', tool: 'Bash', running: true })
    send(HubKind.Step, { kind: 'diff', id: 'call-1', tool: 'Write', path: 'result.txt' })
    vi.advanceTimersByTime(50)

    const steps = JSON.parse(ghost()?.steps ?? '[]')
    expect(steps).toEqual([{ kind: 'diff', id: 'call-1', tool: 'Write', path: 'result.txt' }])
  })
})

describe('subagent live card', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('replaces the running card with the final one by call id', () => {
    const { send, ghost } = harness()
    send(HubKind.Step, {
      kind: 'subagent',
      id: 'call-1',
      tool: 'run_subagent',
      running: true,
      subSteps: [{ kind: 'tool', tool: 'Read' }],
    })
    send(HubKind.Step, {
      kind: 'subagent',
      id: 'call-1',
      tool: 'run_subagent',
      subSteps: [
        { kind: 'tool', tool: 'Read' },
        { kind: 'tool', tool: 'Edit' },
      ],
    })
    vi.advanceTimersByTime(50)
    const steps = JSON.parse(ghost()?.steps ?? '[]')
    expect(steps).toHaveLength(1) // one card, grown in place
    expect(steps[0].running).toBeUndefined()
    expect(steps[0].subSteps).toHaveLength(2)
  })
})
