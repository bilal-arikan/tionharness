// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, expect, it, vi } from 'vitest'
import { WorkspaceDataFolder } from './WorkspaceDataFolder'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []
afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

it('shows the server-owned folder as read-only and copies its path', async () => {
  const path = 'C:\\Users\\Bilal\\AppData\\TionHarness\\workspaces\\WS1'
  const writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  })
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)

  await act(async () => root.render(<WorkspaceDataFolder path={path} />))
  const input = container.querySelector<HTMLInputElement>('[data-testid="workspace-data-folder"]')!
  expect(input.value).toBe(path)
  expect(input.readOnly).toBe(true)

  const button = container.querySelector<HTMLButtonElement>(
    '[data-testid="workspace-data-folder-copy"]',
  )!
  await act(async () => button.click())
  expect(writeText).toHaveBeenCalledExactlyOnceWith(path)
})
