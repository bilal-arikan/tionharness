import { beforeEach, expect, it, vi } from 'vitest'
import type { Message } from '@/types'
import {
  performRetry,
  performRewindTo,
  performRerunLast,
  type HistoryContext,
} from './chatStreamHistory'

const mocks = vi.hoisted(() => ({
  page: vi.fn(),
  remove: vi.fn(),
  rewind: vi.fn(),
  workspace: 'W1',
}))
vi.mock('@/api', () => ({
  api: { listMessagePage: mocks.page, deleteMessage: mocks.remove, rewindSession: mocks.rewind },
  getActiveWorkspace: () => mocks.workspace,
}))
const message = (id: string, role: Message['role']): Message => ({
  id,
  role,
  sessionId: 'S1',
  text: id,
  createdAt: 0,
})
function context(messages: Message[]): HistoryContext {
  const messagesRef = { current: messages }
  return {
    activeSessionIdRef: { current: 'S1' },
    messagesRef,
    sendMessageRef: { current: vi.fn() },
    setMessages: vi.fn((update) => {
      messagesRef.current = typeof update === 'function' ? update(messagesRef.current) : update
    }),
  }
}
beforeEach(() => {
  vi.resetAllMocks()
  mocks.workspace = 'W1'
})

it('finds a retry prompt across multiple history pages', async () => {
  const ctx = context([message('failed', 'assistant')])
  mocks.page
    .mockResolvedValueOnce({ items: [message('middle', 'assistant')], hasMore: true })
    .mockResolvedValueOnce({ items: [message('prompt', 'user')], hasMore: false })
  await performRetry(ctx, 'failed')
  expect(mocks.page.mock.calls.map((call) => call[1])).toEqual([
    { before: 'failed' },
    { before: 'middle' },
  ])
  expect(mocks.remove.mock.calls).toEqual([
    ['S1', 'failed'],
    ['S1', 'prompt'],
  ])
  expect(ctx.sendMessageRef.current).toHaveBeenCalledWith('prompt', 'S1', [])
})

it('keeps visible history and does not resend after a failed deletion', async () => {
  const messages = [message('prompt', 'user'), message('failed', 'assistant')]
  const ctx = context(messages)
  mocks.remove.mockRejectedValue(new Error('disk full'))
  await expect(performRetry(ctx, 'failed')).rejects.toThrow('disk full')
  expect(ctx.messagesRef.current).toBe(messages)
  expect(ctx.sendMessageRef.current).not.toHaveBeenCalled()
})

it('does not clear a rewind selection when persistence fails', async () => {
  const ctx = context([message('prompt', 'user')])
  const close = vi.fn()
  mocks.rewind.mockRejectedValue(new Error('disk full'))
  await expect(performRewindTo(ctx, close, 'prompt')).rejects.toThrow('disk full')
  expect(ctx.messagesRef.current).toHaveLength(1)
  expect(close).not.toHaveBeenCalled()
})

it('abandons a retry when its workspace changes during history lookup', async () => {
  const ctx = context([message('failed', 'assistant')])
  mocks.page.mockImplementation(async () => {
    mocks.workspace = 'W2'
    return { items: [message('prompt', 'user')], hasMore: false }
  })
  await performRetry(ctx, 'failed')
  expect(mocks.remove).not.toHaveBeenCalled()
  expect(ctx.sendMessageRef.current).not.toHaveBeenCalled()
})

it('reruns the actual latest reply while an older history page is visible', async () => {
  const ctx = context([message('old', 'assistant')])
  mocks.page.mockResolvedValue({ items: [message('latest', 'assistant')] })
  const retry = vi.fn()
  await performRerunLast(ctx, retry)
  expect(retry).toHaveBeenCalledWith('latest')
})
