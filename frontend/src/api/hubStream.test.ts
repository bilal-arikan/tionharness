// @vitest-environment jsdom
import { beforeEach, expect, it, vi } from 'vitest'
import { subscribeHubStream } from './hubStream'
import { setActiveWorkspace } from './client'

const transport = vi.hoisted(() => ({
  spec: null as null | (() => { key: string; workspaceId: string; since: number; epoch: string }),
  onFrame: null as null | ((frame: string) => void),
  reconnect: vi.fn(),
}))
vi.mock('./liveConnection', () => ({
  reconnectLiveConnection: transport.reconnect,
  subscribeLiveChannel: (
    spec: typeof transport.spec,
    handlers: { onFrame: (frame: string) => void },
  ) => {
    transport.spec = spec
    transport.onFrame = handlers.onFrame
    return vi.fn()
  },
}))

beforeEach(() => {
  transport.reconnect.mockClear()
  setActiveWorkspace('WS30')
})

function send(event: string, data: unknown) {
  transport.onFrame!(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`)
}

it('keeps its workspace and cursor scoped, dedupes durable replay, and preserves ephemeral deltas', () => {
  const onEvent = vi.fn()
  subscribeHubStream('/api/sessions/SES36/stream', { onEvent })
  send('hello', { epoch: 'boot1', head: 8 })
  send('hub', { seq: 9, kind: 'step' })
  send('hub', { seq: 9, kind: 'step' })
  send('hub', { seq: 0, kind: 'delta' })
  send('hub', { seq: 0, kind: 'delta' })
  setActiveWorkspace('WS1')
  expect(onEvent).toHaveBeenCalledTimes(3)
  expect(transport.spec!()).toMatchObject({ workspaceId: 'WS30', since: 9, epoch: 'boot1' })
  send('hub', { seq: 11, kind: 'step' })
  expect(transport.reconnect).toHaveBeenCalledTimes(1)
  expect(transport.spec!().since).toBe(9)
})

it('resets a previous boot cursor before accepting restarted-server events', () => {
  const onEvent = vi.fn()
  const onReset = vi.fn()
  subscribeHubStream('/api/workspace/stream', { onEvent, onReset })
  send('hello', { epoch: 'old', head: 99 })
  send('hub', { seq: 100 })
  send('hello', { epoch: 'new', head: 1 })
  send('reset', { head: 1 })
  send('hub', { seq: 2 })
  expect(onReset).toHaveBeenCalledExactlyOnceWith({ head: 1 })
  expect(onEvent).toHaveBeenCalledTimes(2)
  expect(transport.spec!()).toMatchObject({ since: 2, epoch: 'new' })
})
