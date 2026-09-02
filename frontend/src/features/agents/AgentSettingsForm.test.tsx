// @vitest-environment jsdom

import { act, type ButtonHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
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
vi.mock('@/shared/components', () => ({
  Button: (props: ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props} />,
  PromptEditor: ({
    value,
    onChange,
    ...props
  }: Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
    value: string
    onChange: (value: string) => void
  }) => (
    <textarea {...props} value={value} onChange={(event) => onChange(event.currentTarget.value)} />
  ),
}))

import { AgentSettingsForm } from './AgentSettingsForm'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []
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
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<AgentSettingsForm agent={agent} onSave={onSave} />))
  return { container, onSave }
}

function option(container: HTMLElement, group: string, value: string) {
  const element = container.querySelector<HTMLButtonElement>(
    `[data-testid="${group}-option"][data-value="${value}"]`,
  )
  if (!element) throw new Error(`Option not found: ${group}/${value}`)
  return element
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

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
