// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Agent } from '@/types'

const listAgents = vi.fn()
const getWorkspaceSettings = vi.fn()

vi.mock('@/api', () => ({
  api: {
    listAgents: () => listAgents(),
    getWorkspaceSettings: () => getWorkspaceSettings(),
    updateAgent: vi.fn(),
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
}))
vi.mock('@/shared/components', () => ({ LoadingState: () => null }))

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
  const root = createRoot(container)
  roots.push(root)
  await act(async () => {
    root.render(<SystemAgentsPanel onError={onError} />)
  })
  return { container, onError }
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
  it('lists only system agents, split into services and workers', async () => {
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

    expect(rosterIds(container)).toEqual(['SYS1', 'SYS2'])
    expect(container.textContent).toContain('Servisler')
    expect(container.textContent).toContain("Worker'lar")
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

  // Built-ins are edited in place and the edit applies installation-wide, so the
  // scope note must say that rather than promising a workspace-local copy.
  it('says an edit applies to every workspace and makes no copy', async () => {
    listAgents.mockResolvedValue([
      agent({ id: 'SYS1', name: 'Titler', system: true, locked: true, systemKey: 'titler' }),
    ])
    const { container } = await renderPanel()

    const note = container.querySelector('[data-testid="system-agents-scope-note"]')?.textContent
    expect(note).toContain('tüm workspace')
    expect(note).toContain('kopya oluşmaz')
  })

  it('reports a load failure to the caller', async () => {
    listAgents.mockRejectedValue(new Error('boom'))
    const { onError } = await renderPanel()
    expect(onError).toHaveBeenCalledWith('boom')
  })
})
