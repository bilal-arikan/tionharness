// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Agent, AgentPatch } from '@/types'
import { i18next } from '@/i18n'

const listAgents = vi.fn()
const getWorkspaceSettings = vi.fn()
const updateAgent = vi.fn()
const assignCatalogAgent = vi.fn().mockResolvedValue({})
const detachCatalogAgent = vi.fn().mockResolvedValue({})

vi.mock('@/api/agentCatalog', () => ({
  catalogEditorApi: {},
  agentCatalogApi: {
    listAgentCatalog: () =>
      listAgents().then((agents: Agent[]) => ({
        agents: agents.map((agent) => ({
          agent,
          assignments: [{ workspaceId: 'WS1', workspaceName: 'Studio', agentId: agent.id }],
        })),
        workspaces: [
          { id: 'WS1', name: 'Studio' },
          { id: 'WS2', name: 'Research' },
        ],
      })),
    updateCatalogAgent: (id: string, patch: AgentPatch) => updateAgent(id, patch),
    assignCatalogAgent: (id: string, workspaceId: string) => assignCatalogAgent(id, workspaceId),
    detachCatalogAgent: (id: string, workspaceId: string) => detachCatalogAgent(id, workspaceId),
    deriveCatalogAgent: vi.fn(),
    restoreCatalogAgent: vi.fn(),
  },
}))

vi.mock('@/api', () => ({
  api: {
    listAgents: () => listAgents(),
    getWorkspaceSettings: () => getWorkspaceSettings(),
    updateAgent: (id: string, patch: AgentPatch) => updateAgent(id, patch),
    deriveAgent: vi.fn(),
    deleteAgent: vi.fn(),
    restoreDefaultAgent: vi.fn(),
  },
}))

// The settings form has its own tests; stub it down to the id of the agent it
// received so the assertions stay on the panel's own job: WHICH agents it lists
// and which one it hands to the form.
vi.mock('@/features/agents/AgentSettingsForm', () => ({
  AgentSettingsForm: ({ agent }: { agent: Agent }) => (
    <div data-testid="settings-form">{agent.id}</div>
  ),
}))
vi.mock('@/features/agents/SystemAgentStatusBadge', () => ({
  SystemAgentStatusBadge: () => null,
}))
vi.mock('@/shared/components/agents/AgentIdentity', () => ({
  AgentIdentity: ({ agent }: { agent: Agent }) => <span>{agent.name}</span>,
}))
vi.mock('@/shared/lib/catalog', () => ({
  useCatalog: () => [],
  resolveModelLabel: () => 'model',
  thinkingOptionsForModel: () => [],
}))
// The bulk bar and edit panel render for real; only the provider picker (which
// fetches provider instances) is stubbed to a button that picks a fixed pair.
vi.mock('@/shared/components/agents/ProviderInstanceModelSelect', () => ({
  ProviderInstanceModelSelect: ({
    onChange,
  }: {
    onChange: (kindId: string, instanceId: string, model: string) => void
  }) => (
    <button data-testid="pick-model" onClick={() => onChange('openai', 'PRV9', 'gpt-x')}>
      pick
    </button>
  ),
}))
vi.mock('@/shared/components', async () => {
  const { SelectionBar, SelectionBarButton } = await import('@/shared/components/SelectionBar')
  const { Button } = await import('@/shared/components/Button')
  return { LoadingState: () => null, SelectionBar, SelectionBarButton, Button }
})

const { SystemAgentsPanel } = await import('./SystemAgentsPanel')

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []

// Default for every test: the panel always reads the active workspace's name for
// its scope banner. vi.clearAllMocks() drops calls, not implementations, so this
// survives afterEach; a test that needs another answer overrides it.
getWorkspaceSettings.mockResolvedValue({ name: 'Atölye' })

const agent = (over: Partial<Agent> & Pick<Agent, 'id' | 'name'>): Agent =>
  ({ provider: 'claude-cli', model: 'sonnet', ...over }) as Agent

async function renderPanel(onError = vi.fn()) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const header = document.createElement('div')
  const content = document.createElement('div')
  container.append(header, content)
  const root = createRoot(content)
  roots.push(root)
  await act(async () => {
    root.render(<SystemAgentsPanel onError={onError} headerTarget={header} />)
  })
  return { container, header, onError }
}

afterEach(() => {
  act(() => roots.splice(0).forEach((root) => root.unmount()))
  document.body.innerHTML = ''
  vi.clearAllMocks()
})

function rosterIds(container: HTMLElement) {
  return [...container.querySelectorAll('[data-testid="system-agent-roster-item"]')].map((el) =>
    el.getAttribute('data-agent-id'),
  )
}

describe('SystemAgentsPanel', () => {
  it('assigns a central profile to another workspace without switching workspace', async () => {
    listAgents.mockResolvedValue([agent({ id: 'CAT1', name: 'Reviewer' })])
    const { container } = await renderPanel()
    const button = container.querySelector<HTMLButtonElement>('[aria-label="Assign to Research"]')!
    await act(async () => button.click())
    expect(assignCatalogAgent).toHaveBeenCalledWith('CAT1', 'WS2')
    expect(listAgents).toHaveBeenCalledTimes(2)
  })

  it('preserves an assignment when its removal is cancelled', async () => {
    listAgents.mockResolvedValue([agent({ id: 'CAT1', name: 'Reviewer' })])
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const { container } = await renderPanel()
    await act(async () =>
      container.querySelector<HTMLButtonElement>('[aria-label="Remove from Studio"]')!.click(),
    )
    expect(detachCatalogAgent).not.toHaveBeenCalled()
    confirm.mockRestore()
  })
  it('lists custom and system profiles together with category filters', async () => {
    listAgents.mockResolvedValue([
      agent({ id: 'AGT1', name: 'Ada' }),
      agent({ id: 'SYS1', name: 'Titler', system: true, locked: true, systemKey: 'titler' }),
      agent({
        id: 'SYS2',
        name: 'Worker: Coder',
        system: true,
        locked: true,
        systemKey: 'subagent-coder',
      }),
    ])
    const { container } = await renderPanel()

    expect(rosterIds(container)).toEqual(['AGT1', 'SYS1', 'SYS2'])
    expect(container.textContent).toContain('Services')
    expect(container.textContent).toContain('Workers')
  })

  it('selects the first system agent and switches on click', async () => {
    listAgents.mockResolvedValue([
      agent({ id: 'SYS1', name: 'Titler', system: true, locked: true, systemKey: 'titler' }),
      agent({ id: 'SYS2', name: 'Compactor', system: true, locked: true, systemKey: 'compaction' }),
    ])
    const { container } = await renderPanel()

    expect(container.querySelector('[data-testid="settings-form"]')?.textContent).toBe('SYS1')

    const second = container.querySelector<HTMLButtonElement>(
      '[data-testid="system-agent-roster-item"][data-agent-id="SYS2"]',
    )
    if (!second) throw new Error('SYS2 roster row not found')
    await act(async () => second.click())

    expect(container.querySelector('[data-testid="settings-form"]')?.textContent).toBe('SYS2')
  })

  it('places the count and working library controls in the page header', async () => {
    listAgents.mockResolvedValue([
      agent({ id: 'SYS1', name: 'Titler', system: true, locked: true, systemKey: 'titler' }),
    ])
    const { container, header } = await renderPanel()

    expect(header.textContent).toContain('1 agents')
    expect(container.textContent).not.toContain('One central profile.')
    const refresh = header.querySelector<HTMLButtonElement>('[aria-label="Refresh library"]')!
    await act(async () => refresh.click())
    expect(listAgents).toHaveBeenCalledTimes(2)

    const create = header.querySelector<HTMLButtonElement>('[aria-expanded]')!
    expect(create.textContent).toContain('New agent')
    expect(container.querySelector('form')).toBeNull()
    await act(async () => create.click())
    expect(create.getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelector('form')).not.toBeNull()
    expect(header.querySelector('form')).toBeNull()
    await act(async () => create.click())
    expect(container.querySelector('form')).toBeNull()
  })

  it('reports a load failure to the caller', async () => {
    listAgents.mockRejectedValue(new Error('boom'))
    const { onError } = await renderPanel()
    expect(onError).toHaveBeenCalledWith('boom')
  })
})

describe('SystemAgentsPanel bulk edit', () => {
  const roster = () => [
    agent({ id: 'SYS1', name: 'Titler', system: true, locked: true, systemKey: 'titler' }),
    agent({ id: 'SYS2', name: 'Compactor', system: true, locked: true, systemKey: 'compaction' }),
    agent({
      id: 'SYS3',
      name: 'Worker: Coder',
      system: true,
      locked: true,
      systemKey: 'subagent-coder',
    }),
    agent({ id: 'AGT1', name: 'Ada' }),
  ]

  const row = (container: HTMLElement, id: string) => {
    const el = container.querySelector<HTMLButtonElement>(
      `[data-testid="system-agent-roster-item"][data-agent-id="${id}"]`,
    )
    if (!el) throw new Error(`${id} roster row not found`)
    return el
  }

  const clickRow = (container: HTMLElement, id: string, mods: MouseEventInit = {}) =>
    act(async () => {
      row(container, id).dispatchEvent(new MouseEvent('click', { bubbles: true, ...mods }))
    })

  const buttonByText = (container: HTMLElement, text: string) => {
    const el = [...container.querySelectorAll('button')].find((b) => b.textContent?.trim() === text)
    if (!el) throw new Error(`button "${text}" not found`)
    return el
  }

  const byTestId = (container: HTMLElement, testId: string) => {
    const el = container.querySelector<HTMLElement>(`[data-testid="${testId}"]`)
    if (!el) throw new Error(`${testId} not found`)
    return el
  }

  const click = (el: HTMLElement) => act(async () => el.click())

  const formId = (container: HTMLElement) =>
    container.querySelector('[data-testid="settings-form"]')?.textContent

  beforeEach(async () => {
    await i18next.changeLanguage('tr')
    listAgents.mockResolvedValue(roster())
    updateAgent.mockResolvedValue({ agent: {} })
  })

  it('Ctrl+Click selects rows without leaving the open form; a plain click clears it', async () => {
    const { container } = await renderPanel()
    expect(formId(container)).toBe('SYS1')

    // The open row is folded into a fresh multi-selection.
    await clickRow(container, 'SYS3', { ctrlKey: true })
    expect(container.textContent).toContain('2 seçili')
    expect(formId(container)).toBe('SYS1')

    await clickRow(container, 'SYS2')
    expect(container.textContent).not.toContain('seçili')
    expect(formId(container)).toBe('SYS2')
  })

  it('writes the picked provider and model to every selected system agent', async () => {
    const { container } = await renderPanel()
    await clickRow(container, 'SYS2', { ctrlKey: true })
    await click(buttonByText(container, 'Düzenle'))

    expect(byTestId(container, 'agent-bulk-edit-panel').getAttribute('data-scope')).toBe('system')

    await click(byTestId(container, 'pick-model'))
    await click(byTestId(container, 'agent-bulk-edit-apply'))

    expect(updateAgent.mock.calls).toEqual([
      ['SYS1', { provider: 'PRV9', model: 'gpt-x' }],
      ['SYS2', { provider: 'PRV9', model: 'gpt-x' }],
    ])
    // Success closes the panel, clears the selection and re-fetches the roster.
    expect(container.querySelector('[data-testid="agent-bulk-edit-panel"]')).toBeNull()
    expect(container.textContent).not.toContain('seçili')
    expect(listAgents).toHaveBeenCalledTimes(2)
  })

  it('keeps the selection, refreshes and reports when a write fails', async () => {
    updateAgent.mockImplementation((id: string) =>
      id === 'SYS2' ? Promise.reject(new Error('SYS2 failed')) : Promise.resolve({ agent: {} }),
    )
    const { container, onError } = await renderPanel()
    await clickRow(container, 'SYS2', { ctrlKey: true })
    await click(buttonByText(container, 'Düzenle'))
    await click(byTestId(container, 'agent-bulk-edit-apply'))

    expect(onError).toHaveBeenCalledWith('SYS2 failed')
    expect(listAgents).toHaveBeenCalledTimes(2)
    expect(container.querySelector('[data-testid="agent-bulk-edit-panel"]')).not.toBeNull()
    expect(container.textContent).toContain('2 seçili')
  })

  it('closes the edit panel with the selection, so a new selection starts closed', async () => {
    const { container } = await renderPanel()
    await clickRow(container, 'SYS2', { ctrlKey: true })
    await click(buttonByText(container, 'Düzenle'))
    expect(container.querySelector('[data-testid="agent-bulk-edit-panel"]')).not.toBeNull()

    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    })
    expect(container.querySelector('[data-testid="agent-bulk-edit-panel"]')).toBeNull()

    await clickRow(container, 'SYS3', { ctrlKey: true })
    expect(container.textContent).toContain('2 seçili')
    expect(container.querySelector('[data-testid="agent-bulk-edit-panel"]')).toBeNull()
  })

  it('selects every visible agent from the bar, including custom profiles', async () => {
    const { container } = await renderPanel()
    await clickRow(container, 'SYS2', { ctrlKey: true })
    await click(buttonByText(container, 'Tümü'))

    expect(container.textContent).toContain('4 seçili')
  })
})
