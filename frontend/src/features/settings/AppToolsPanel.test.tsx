// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppSettings } from '@/types'
import { i18next } from '@/i18n'
import { ExecutionPanel } from './ExecutionPanel'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('ExecutionPanel spawn limits', () => {
  it('renders no wall-clock hard-cap controls', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    const draft = {} as AppSettings

    act(() => root.render(<ExecutionPanel draft={draft} set={vi.fn()} setDraft={vi.fn()} />))

    expect(container.textContent).toContain('Productive work has no total duration cap.')
    expect(container.textContent).not.toContain('spawnTimeoutMin')
    expect(container.textContent).not.toContain('Spawn süresi — üst sınır')
    expect(container.textContent).not.toContain('Zamanlama süresi (dk)')
  })

  it('renders execution settings in Turkish when the UI locale changes', async () => {
    await i18next.changeLanguage('tr')
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)

    act(() =>
      root.render(<ExecutionPanel draft={{} as AppSettings} set={vi.fn()} setDraft={vi.fn()} />),
    )

    expect(container.textContent).toContain('Üretken işlerin toplam süre sınırı yoktur.')
    expect(container.textContent).toContain('Arka plan çalışmaları ve boşta kalma sınırları')
    expect(container.textContent).not.toContain('Productive work has no total duration cap.')
  })
})
