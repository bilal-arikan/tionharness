// @vitest-environment jsdom

import { act } from 'react'
import { render, query } from '@/test/render'
import { describe, expect, it, vi } from 'vitest'
import type { Agent, AgentPatch } from '@/types'

vi.mock('@/shared/lib/dirtySignals', () => ({ useRegisterDirty: () => undefined }))
vi.mock('@/shared/components/agents/AgentAvatar', () => ({ AgentAvatar: () => null }))
vi.mock('@/shared/components/EmojiField', () => ({ EmojiField: () => null }))
vi.mock('@/shared/components/agents/ProviderInstanceModelSelect', () => ({
  ProviderInstanceModelSelect: () => null,
}))
vi.mock('./AgentToolsSection', () => ({ AgentToolsSection: () => null }))
vi.mock('./AgentSkillsSection', () => ({ AgentSkillsSection: () => null }))
vi.mock('./AgentContextModal', () => ({ AgentContextModal: () => null }))
vi.mock('./SystemAgentStatusBadge', () => ({ SystemAgentStatusBadge: () => null }))
vi.mock('@/shared/components/CoordinatorWorkflowPicker', () => ({
  CoordinatorWorkflowPicker: () => <div data-testid="agent-coordinator-workflow" />,
}))
vi.mock('@/shared/lib/catalog', () => ({
  useCatalog: () => [],
  thinkingOptionsForModel: (available: unknown) => available,
}))
vi.mock('@/shared/components', async () => import('@/test/formStubs'))

import { AgentSettingsForm } from './AgentSettingsForm'

const baseAgent = {
  id: 'AGT1',
  name: 'Agent',
  provider: 'claude-cli',
  model: 'sonnet',
  permissionMode: 'auto',
  coordinatorMode: true,
  coordinatorWorkflow: 'coordinator-wf-plan-dev-test',
  coordinatorPrompt: 'İşi workerlara böl.',
} as Agent

function renderForm(agent: Agent = baseAgent, onSave = vi.fn(async (_patch: AgentPatch) => {})) {
  const { container } = render(<AgentSettingsForm agent={agent} onSave={onSave} />)
  return { container, onSave }
}

function option(container: HTMLElement, group: string, value: string) {
  return query<HTMLButtonElement>(
    container,
    `[data-testid="${group}-option"][data-value="${value}"]`,
  )
}

describe('AgentSettingsForm boolean option pills', () => {
  it('defaults an unstored native web search setting to Açık', () => {
    const { container } = renderForm({ ...baseAgent, nativeWebSearch: undefined })

    expect(option(container, 'agent-native-web-search', 'on').getAttribute('aria-checked')).toBe(
      'true',
    )
    expect(
      container.querySelector('[data-testid="agent-native-web-search"]')?.getAttribute('role'),
    ).toBe('radiogroup')
  })

  it('renders an explicit false native web search setting as Kapalı', () => {
    const { container } = renderForm({ ...baseAgent, nativeWebSearch: false })

    expect(option(container, 'agent-native-web-search', 'off').getAttribute('aria-checked')).toBe(
      'true',
    )
  })

  it('defaults an unstored native shell setting to Kapalı (opt-in)', () => {
    const { container } = renderForm({ ...baseAgent, nativeShell: undefined })

    expect(option(container, 'agent-native-shell', 'off').getAttribute('aria-checked')).toBe('true')
  })

  it('renders an explicit true native shell setting as Açık', () => {
    const { container } = renderForm({ ...baseAgent, nativeShell: true })

    expect(option(container, 'agent-native-shell', 'on').getAttribute('aria-checked')).toBe('true')
  })

  it('defaults an unstored coordinator setting to Kapalı and hides coordinator fields', () => {
    const { container } = renderForm({ ...baseAgent, coordinatorMode: undefined })

    expect(option(container, 'agent-coordinator-mode', 'off').getAttribute('aria-checked')).toBe(
      'true',
    )
    expect(container.querySelector('[data-testid="agent-coordinator-workflow"]')).toBeNull()
    expect(container.querySelector('[data-testid="agent-coordinator-prompt-textarea"]')).toBeNull()
  })

  it('saves booleans and clears coordinator-only fields when Kapalı is selected', async () => {
    const { container, onSave } = renderForm({ ...baseAgent, nativeWebSearch: true })

    expect(container.querySelector('[data-testid="agent-coordinator-workflow"]')).not.toBeNull()
    expect(
      container.querySelector('[data-testid="agent-coordinator-prompt-textarea"]'),
    ).not.toBeNull()

    act(() => option(container, 'agent-native-web-search', 'off').click())
    act(() => option(container, 'agent-coordinator-mode', 'off').click())

    expect(container.querySelector('[data-testid="agent-coordinator-workflow"]')).toBeNull()
    expect(container.querySelector('[data-testid="agent-coordinator-prompt-textarea"]')).toBeNull()

    const save = container.querySelector<HTMLButtonElement>('[data-testid="agent-save"]')
    if (!save) throw new Error('Save button not found')
    await act(async () => save.click())

    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        nativeWebSearch: false,
        coordinatorMode: false,
        coordinatorWorkflow: '',
        coordinatorPrompt: '',
      }),
    )
  })
})
