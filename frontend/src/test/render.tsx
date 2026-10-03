import { act, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach } from 'vitest'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const mounted: { root: ReturnType<typeof createRoot>; container: HTMLDivElement }[] = []

export function render(element: ReactNode) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  mounted.push({ root, container })
  act(() => root.render(element))
  return { container, rerender: (next: ReactNode) => act(() => root.render(next)) }
}

export function query<T extends Element>(container: HTMLElement, selector: string): T {
  const element = container.querySelector<T>(selector)
  if (!element) throw new Error(`Element not found: ${selector}`)
  return element
}

afterEach(() => {
  for (const { root, container } of mounted.splice(0)) {
    act(() => root.unmount())
    container.remove()
  }
})
