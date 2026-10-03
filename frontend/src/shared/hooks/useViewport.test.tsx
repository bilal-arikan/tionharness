// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useIsMobile } from './useMediaQuery'
import { useViewport } from './useViewport'

const reactEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
reactEnvironment.IS_REACT_ACT_ENVIRONMENT = true

let root: ReturnType<typeof createRoot>
let container: HTMLDivElement

function Probe() {
  const mobile = useIsMobile()
  const viewport = useViewport()
  return <span>{JSON.stringify({ mobile, ...viewport })}</span>
}

function resize(width: number, height: number) {
  Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { configurable: true, value: height })
  act(() => {
    window.dispatchEvent(new Event('resize'))
    vi.advanceTimersByTime(1)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) =>
    setTimeout(() => callback(0), 0),
  )
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(() => {
  act(() => root.unmount())
  container.remove()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

it('keeps the mobile branch aligned with width tiers across the md boundary and aspect changes', () => {
  resize(767, 1000)
  act(() => root.render(<Probe />))
  expect(JSON.parse(container.textContent!)).toEqual({
    mobile: true,
    tier: 'narrow',
    aspect: 'portrait',
  })
  resize(767, 400)
  expect(JSON.parse(container.textContent!)).toEqual({
    mobile: true,
    tier: 'narrow',
    aspect: 'landscape',
  })
  resize(768, 1000)
  expect(JSON.parse(container.textContent!)).toEqual({
    mobile: false,
    tier: 'square',
    aspect: 'portrait',
  })
  resize(1280, 800)
  expect(JSON.parse(container.textContent!)).toEqual({
    mobile: false,
    tier: 'wide',
    aspect: 'landscape',
  })
  resize(767, 1000)
  expect(JSON.parse(container.textContent!).mobile).toBe(true)
})
