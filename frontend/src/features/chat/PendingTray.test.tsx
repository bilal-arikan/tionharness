// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PendingTray } from './PendingTray'
import type { PendingItem } from './PendingTray'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function renderTray(props: Parameters<typeof PendingTray>[0]) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<PendingTray {...props} />))
  return container
}

function steerButton(container: HTMLElement, id: string) {
  return container.querySelector<HTMLButtonElement>(`[data-testid="pending-steer-${id}"]`)
}

const queued: PendingItem[] = [
  { id: 'm-1', text: 'check the other file', kind: 'queue', sid: 'S1' },
]

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('PendingTray steer-now action', () => {
  it('converts a queued message when a turn is streaming', () => {
    const onSteerNow = vi.fn()
    const container = renderTray({
      items: queued,
      onRemove: vi.fn(),
      onSteerNow,
      canSteer: true,
    })

    const button = steerButton(container, 'm-1')
    expect(button).not.toBeNull()
    expect(button?.disabled).toBe(false)
    act(() => button?.click())
    expect(onSteerNow).toHaveBeenCalledWith('m-1')
  })

  it('disables the action with a reason when no turn is running', () => {
    const onSteerNow = vi.fn()
    const container = renderTray({
      items: queued,
      onRemove: vi.fn(),
      onSteerNow,
      canSteer: false,
    })

    const button = steerButton(container, 'm-1')
    expect(button?.disabled).toBe(true)
    expect(button?.title).toContain('remains queued')
    act(() => button?.click())
    expect(onSteerNow).not.toHaveBeenCalled()
  })

  // The gap this closes: the action used to be gated on "a turn is streaming"
  // alone, so on a turn without a delivery channel it stayed enabled and the backend could
  // only answer "unsupported". canSteer now carries the server's verdict.
  it('disables the action when a turn runs but cannot carry a steer', () => {
    const onSteerNow = vi.fn()
    const container = renderTray({
      items: queued,
      onRemove: vi.fn(),
      onSteerNow,
      canSteer: false,
      turnRunning: true,
    })

    const button = steerButton(container, 'm-1')
    expect(button?.disabled).toBe(true)
    // The reason must name the real cause, not the absent-turn one: a user who
    // sees "no running turn" while a turn is plainly streaming learns nothing.
    expect(button?.title).toContain('unavailable')
    expect(button?.title).not.toContain('No running turn')
    act(() => button?.click())
    expect(onSteerNow).not.toHaveBeenCalled()
  })

  it('offers no steer action for a non-queue item', () => {
    const container = renderTray({
      items: [{ id: 'h-1', text: 'worker bildirimi', kind: 'holding', sid: 'S1' }],
      onRemove: vi.fn(),
      onSteerNow: vi.fn(),
      canSteer: true,
    })

    expect(steerButton(container, 'h-1')).toBeNull()
  })
})

// A dispatched message has already left for the server and becomes the user's own
// bubble in the transcript. Rendering it here as well showed the same text twice,
// in a row the user cannot cancel or act on.
describe('PendingTray dispatched head', () => {
  const dispatching: PendingItem = {
    id: 'd-1',
    text: 'gönderilmiş mesaj',
    kind: 'dispatching',
    sid: 'S1',
  }

  it('does not render a dispatching item', () => {
    const container = renderTray({
      items: [...queued, dispatching],
      onRemove: vi.fn(),
    })

    expect(container.textContent).not.toContain('Gönderiliyor')
    expect(container.textContent).not.toContain('gönderilmiş mesaj')
    // The genuinely waiting message is untouched.
    expect(container.textContent).toContain('check the other file')
  })

  it('renders nothing at all when the dispatched head is the only item', () => {
    const container = renderTray({ items: [dispatching], onRemove: vi.fn() })

    // Not merely an empty card: the header must not render either, or the user
    // sees a "Bekleyenler" shell listing nothing.
    expect(container.textContent).toBe('')
    expect(container.textContent).not.toContain('Bekleyenler')
  })

  // The failed row is the opposite case: its turn never started, so the text is
  // NOT in the transcript and this row is the only copy left on screen.
  it('renders a failed head with its text, unlike a dispatching one', () => {
    const failed: PendingItem = {
      id: 'd-1',
      text: 'başlatılamayan mesaj',
      kind: 'failed',
      sid: 'S1',
    }
    const container = renderTray({ items: [failed], onRemove: vi.fn() })

    expect(container.textContent).toContain('başlatılamayan mesaj')
    expect(container.textContent).toContain('Gönderilemedi')
  })

  it('dismisses a failed row through onRemove', () => {
    const onRemove = vi.fn()
    const failed: PendingItem = {
      id: 'd-1',
      text: 'başlatılamayan mesaj',
      kind: 'failed',
      sid: 'S1',
    }
    const container = renderTray({ items: [failed], onRemove })

    const btn = container.querySelector<HTMLButtonElement>('[data-testid="pending-remove-d-1"]')
    expect(btn).not.toBeNull()
    btn!.click()
    expect(onRemove).toHaveBeenCalledWith('d-1')
  })

  it('numbers queue positions ignoring the hidden head', () => {
    const container = renderTray({
      items: [
        dispatching,
        { id: 'm-1', text: 'first waiting', kind: 'queue', sid: 'S1' },
        { id: 'm-2', text: 'second waiting', kind: 'queue', sid: 'S1' },
      ],
      onRemove: vi.fn(),
    })

    expect(container.textContent).toContain('Sırada #1')
    expect(container.textContent).toContain('Sırada #2')
    expect(container.textContent).not.toContain('Sırada #3')
  })

  it('still renders steer and holding items', () => {
    const container = renderTray({
      items: [
        { id: 'h-1', text: 'worker bildirimi', kind: 'holding', sid: 'S1' },
        dispatching,
        { id: 's-1', text: 'yönlendirme', kind: 'steer', sid: 'S1' },
      ],
      onRemove: vi.fn(),
    })

    expect(container.textContent).toContain('Şu an')
    expect(container.textContent).toContain('worker bildirimi')
    expect(container.textContent).toContain('Yönlendir')
    expect(container.textContent).not.toContain('Gönderiliyor')
  })

  it('counts only visible queue items in the clear-all action', () => {
    const container = renderTray({
      items: [
        dispatching,
        { id: 'm-1', text: 'first waiting', kind: 'queue', sid: 'S1' },
        { id: 'm-2', text: 'second waiting', kind: 'queue', sid: 'S1' },
      ],
      onRemove: vi.fn(),
      onClear: vi.fn(),
    })

    expect(container.textContent).toContain('Kuyruğu temizle (2)')
  })
})
