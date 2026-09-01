// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppSettings } from '@/types'
import { ToolsPanel } from './AppToolsPanel'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('ToolsPanel deprecated timeouts', () => {
  it('explains legacy fields without rendering mutable hard-cap controls', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    const draft = {
      spawnTimeoutMin: 20,
      scheduleTimeoutMin: 60,
    } as AppSettings

    act(() => root.render(<ToolsPanel draft={draft} set={vi.fn()} setDraft={vi.fn()} />))

    expect(container.textContent).toContain('spawnTimeoutMin')
    expect(container.textContent).toContain('scheduleTimeoutMin')
    expect(container.textContent).toContain('deprecated ve etkisizdir')
    expect(container.textContent).toContain('0 = disabled')
    expect(container.textContent).not.toContain('Spawn süresi — üst sınır')
    expect(container.textContent).not.toContain('Zamanlama süresi (dk)')
  })
})
