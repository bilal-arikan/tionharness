// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { expect, it, vi } from 'vitest'
import { AgentsView } from './AgentsView'

const listAgents = vi.hoisted(() => vi.fn().mockResolvedValue([]))
vi.mock('@/api', () => ({ api: { listAgents }, getActiveWorkspace: () => 'WS5' }))
vi.mock('@/shared/lib/catalog', () => ({ useCatalog: () => [], resolveModelLabel: () => '' }))
vi.mock('./AgentSettingsForm', () => ({ AgentSettingsForm: () => null }))
vi.mock('./AgentBulkEditPanel', () => ({ AgentBulkEditPanel: () => null }))
vi.mock('./AgentActivityPanel', () => ({ AgentActivityPanel: () => null }))

it('does not request the archive until the user opens it', async () => {
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  try {
    await act(async () =>
      root.render(
        <AgentsView
          agents={[]}
          defaultAgentId={null}
          defaultAgentSaveState="idle"
          onSetDefault={vi.fn()}
          onCreateAgent={vi.fn()}
          onUpdateAgent={vi.fn()}
          onDuplicateAgent={vi.fn()}
          onDeleteAgent={vi.fn()}
        />,
      ),
    )
    expect(listAgents).not.toHaveBeenCalled()
    await act(async () =>
      container.querySelector<HTMLButtonElement>('[data-testid="agents-archived-toggle"]')!.click(),
    )
    expect(listAgents).toHaveBeenCalledExactlyOnceWith(true)
    await act(async () =>
      container.querySelector<HTMLButtonElement>('[data-testid="agents-archived-toggle"]')!.click(),
    )
    expect(listAgents).toHaveBeenCalledTimes(1)
  } finally {
    act(() => root.unmount())
    container.remove()
  }
})
