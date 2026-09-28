// @vitest-environment jsdom
import { act, useRef } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import type { Message } from '@/types'
import { useTranscriptWindow } from './useTranscriptWindow'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

it('releases a paging anchor on scrollbar or programmatic navigation', async () => {
  vi.stubGlobal('CSS', { escape: (value: string) => value })
  const messages: Message[] = [
    { id: 'M1', sessionId: 'S1', role: 'user', text: 'Prompt', createdAt: 1 },
  ]
  let window!: ReturnType<typeof useTranscriptWindow>
  const container = document.createElement('div')
  const root = createRoot(container)
  function Harness() {
    const ref = useRef<HTMLDivElement>(null)
    window = useTranscriptWindow(messages, ref)
    return (
      <div ref={ref}>
        <div data-msg-id="M1" />
      </div>
    )
  }
  try {
    await act(async () => root.render(<Harness />))
    const scroll = container.firstElementChild as HTMLDivElement
    const row = scroll.firstElementChild as HTMLDivElement
    scroll.getBoundingClientRect = () => ({ top: 0 }) as DOMRect
    row.getBoundingClientRect = () =>
      ({ top: 80 - scroll.scrollTop, bottom: 160 - scroll.scrollTop }) as DOMRect
    window.captureAnchor()
    await act(async () => {
      scroll.scrollTop = 100
      window.onScroll()
    })
    expect(scroll.scrollTop).toBe(100)
  } finally {
    await act(async () => root.unmount())
    vi.unstubAllGlobals()
  }
})
