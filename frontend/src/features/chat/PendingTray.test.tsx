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
    expect(button?.title).toContain('mesaj sırada kalır')
    act(() => button?.click())
    expect(onSteerNow).not.toHaveBeenCalled()
  })

  // The gap this closes: the action used to be gated on "a turn is streaming"
  // alone, so on a turn whose provider/mode has no steer boundary (claude-cli in
  // auto/read-only, any codex-cli turn) it stayed enabled and the backend could
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
    expect(button?.title).toContain('desteklemiyor')
    expect(button?.title).not.toContain('çalışan bir tur yok')
    act(() => button?.click())
    expect(onSteerNow).not.toHaveBeenCalled()
  })

  it('offers no steer action for a non-queue item', () => {
    const container = renderTray({
      items: [{ id: 'd-1', text: 'gönderiliyor', kind: 'dispatching', sid: 'S1' }],
      onRemove: vi.fn(),
      onSteerNow: vi.fn(),
      canSteer: true,
    })

    expect(steerButton(container, 'd-1')).toBeNull()
  })
})
