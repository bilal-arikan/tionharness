// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setLocale } from '@/i18n'
import type { InsightFinding } from '@/types'

vi.mock('@/api', () => ({ api: {} }))

import { FindingsTab } from './FindingsTab'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

beforeEach(async () => {
  await setLocale('tr')
  localStorage.clear()
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

function renderTab(findings: InsightFinding[]) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(
      <FindingsTab
        findings={findings}
        lenses={[]}
        reload={() => {}}
        onOpenSession={() => {}}
        onError={() => {}}
        onNote={() => {}}
      />,
    ),
  )
  return container
}

const finding = {
  id: 'F1',
  title: 'Slow tool call',
  channel: 'app-fix',
  status: 'new',
  occurrences: 1,
} as InsightFinding

describe('FindingsTab', () => {
  it('shows one empty state instead of an empty board when nothing was found', () => {
    const view = renderTab([])
    expect(view.textContent).toContain('Henüz bulgu yok')
    // No counters, filters or lifecycle columns for a board with nothing on it.
    expect(view.querySelector('select')).toBeNull()
    expect(view.textContent).not.toContain('Uygulandı')
  })

  it('renders the board once there are findings', () => {
    const view = renderTab([finding])
    expect(view.textContent).toContain('Slow tool call')
    expect(view.textContent).toContain('Uygulandı')
    expect(view.textContent).not.toContain('Henüz bulgu yok')
  })
})
