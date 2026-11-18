// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Lightbulb } from 'lucide-react'

const updateWorkspaceSettings = vi.fn(async (_patch: unknown) => ({}))

vi.mock('@/api', () => ({
  api: { updateWorkspaceSettings: (patch: unknown) => updateWorkspaceSettings(patch) },
}))

vi.mock('./recommendations', () => ({
  fetchRecommendationData: async () => ({ ws: { ignoredRecommendations: [] } }),
  runRules: () => [
    {
      key: 'sample',
      icon: Lightbulb,
      title: 'Sample recommendation',
      desc: 'Something worth doing.',
      actionLabel: 'Open',
      variant: 'info',
      act: async () => {},
    },
  ],
}))

import { WorkspaceRecommendations } from './WorkspaceRecommendations'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

async function renderCards() {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  await act(async () => {
    root.render(
      <WorkspaceRecommendations
        trigger={1}
        onNavigateView={() => {}}
        onNavigateSettings={() => {}}
      />,
    )
  })
  return container
}

const card = (c: HTMLElement) => c.querySelector('[data-testid="workspace-rec-sample"]')
const click = (c: HTMLElement, testId: string) =>
  act(async () => {
    c.querySelector<HTMLButtonElement>(`[data-testid="${testId}"]`)!.click()
  })

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.innerHTML = ''
  updateWorkspaceSettings.mockClear()
})

describe('WorkspaceRecommendations', () => {
  it('closes a card for now without persisting an ignore', async () => {
    const c = await renderCards()
    expect(card(c)).not.toBeNull()

    await click(c, 'workspace-rec-close-sample')

    expect(card(c)).toBeNull()
    expect(updateWorkspaceSettings).not.toHaveBeenCalled()
  })

  it('persists the ignore when "do not show again" is chosen', async () => {
    const c = await renderCards()

    await click(c, 'workspace-rec-ignore-sample')

    expect(card(c)).toBeNull()
    expect(updateWorkspaceSettings).toHaveBeenCalledWith({ ignoredRecommendations: ['sample'] })
  })
})
