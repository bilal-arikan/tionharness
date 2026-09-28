import { describe, expect, it } from 'vitest'
import type { Message } from '@/types'
import { mergeLiveTranscriptPage } from './transcriptPageMerge'

const message = (id: string, text = id): Message => ({
  id,
  text,
  sessionId: 'S',
  role: 'assistant',
  createdAt: 0,
})

describe('HTTP snapshot and live stream ordering', () => {
  it('keeps an active ghost across a persisted refresh', () => {
    const saved = message('saved'),
      ghost = message('live-hub-S', 'streaming')
    expect(mergeLiveTranscriptPage([saved], [saved, ghost], [saved, ghost])).toEqual([saved, ghost])
  })
  it('keeps a reply and user message arriving after the request began', () => {
    const first = message('first'),
      user = message('user'),
      reply = message('reply')
    expect(mergeLiveTranscriptPage([first], [first], [first, user, reply])).toEqual([
      first,
      user,
      reply,
    ])
  })
  it('does not resurrect a deletion or replace a newer edit', () => {
    const first = message('first'),
      second = message('second'),
      edited = message('second', 'changed')
    expect(mergeLiveTranscriptPage([first, second], [first, second], [edited])).toEqual([edited])
  })
})
