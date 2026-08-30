// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { NavRail } from './NavRail'
import { WorkspaceView } from '@/features/workspace/WorkspaceView'
import { i18next } from '@/i18n'

vi.mock('@/api', () => ({
  api: { getWorkspaceSettings: vi.fn(() => new Promise(() => {})) },
}))

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

function render(element: React.ReactNode) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(element))
  return container
}

function renderNavigation() {
  const nav = render(
    <NavRail
      view="dashboard"
      onSelectView={() => {}}
      workspaces={[]}
      activeWorkspaceId={null}
      unreadWorkspaceIds={new Set()}
      onSwitchWorkspace={() => {}}
      onCreateWorkspace={() => {}}
    />,
  )
  const workspace = render(<WorkspaceView onError={() => {}} tab="logs" />)
  return { nav, workspace }
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  localStorage.clear()
})

describe.each([
  ['en', 'Prompts', 'Logs'],
  ['tr', 'Promptlar', 'Loglar'],
] as const)('navigation labels in %s', (locale, prompts, logs) => {
  it('renders top-level Prompts and Workspace Logs from the catalog', async () => {
    await act(() => i18next.changeLanguage(locale))
    const { nav, workspace } = renderNavigation()

    expect(nav.querySelector('[data-testid="nav-prompts"]')?.textContent).toBe(prompts)
    expect(
      [...workspace.querySelectorAll('button')].some((button) => button.textContent === logs),
    ).toBe(true)
  })
})
