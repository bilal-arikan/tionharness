// @vitest-environment jsdom
//
// The soul editor of a SYSTEM agent offers "Koddaki prompta dön": it pulls the
// prompt compiled into the binary for the agent's role and stages it as an
// ordinary unsaved edit, so a customisation that has drifted can be put back
// without deleting the row (which would also drop its model/tool choices).

import { act, type ButtonHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Agent, AgentPatch } from '@/types'

const agentBuiltinPrompt = vi.fn()

vi.mock('@/api', () => ({ api: { agentBuiltinPrompt: (id: string) => agentBuiltinPrompt(id) } }))
vi.mock('@/shared/lib/dirtySignals', () => ({ useRegisterDirty: () => undefined }))
vi.mock('@/shared/components/agents/AgentAvatar', () => ({ AgentAvatar: () => null }))
vi.mock('@/shared/components/EmojiField', () => ({ EmojiField: () => null }))
vi.mock('@/shared/components/agents/ProviderInstanceModelSelect', () => ({
  ProviderInstanceModelSelect: () => null,
}))
vi.mock('./AgentToolsSection', () => ({ AgentToolsSection: () => null }))
vi.mock('./AgentSkillsSection', () => ({ AgentSkillsSection: () => null }))
vi.mock('./AgentContextModal', () => ({ AgentContextModal: () => null }))
vi.mock('@/shared/components/CoordinatorWorkflowPicker', () => ({
  CoordinatorWorkflowPicker: () => null,
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

const BUILTIN_SOUL = 'koddaki gomulu prompt'

const parent = {
  id: 'AGT1',
  name: 'Titler',
  soul: BUILTIN_SOUL,
  provider: 'claude-cli',
  model: 'haiku',
  permissionMode: 'auto',
  system: true,
  systemKey: 'titler',
  locked: true,
} as Agent
// The workspace customisation of the titler role, drifted from the shipped text.
const customization = {
  ...parent,
  id: 'AGT2',
  name: 'Titler (özel)',
  soul: 'elle degistirilmis prompt',
  locked: false,
  parentId: parent.id,
  overrides: ['soul'],
} as Agent

const roots: Root[] = []

function renderForm(agent: Agent, extra: Record<string, unknown> = {}) {
  const onSave = vi.fn(async (_patch: AgentPatch) => {})
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(<AgentSettingsForm agent={agent} onSave={onSave} parent={parent} {...extra} />),
  )
  return { container, onSave }
}

function q<T extends Element>(container: HTMLElement, selector: string): T {
  const element = container.querySelector<T>(selector)
  if (!element) throw new Error(`Not found: ${selector}`)
  return element
}

// React installs its own `value` setter on the element, so assigning directly
// bypasses onChange. Go through the prototype setter the way a real edit does.
function typeInto(el: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')?.set
  setter?.call(el, value)
  el.dispatchEvent(new Event('input', { bubbles: true }))
}

const revertButton = (container: HTMLElement) =>
  q<HTMLButtonElement>(container, '[data-testid="agent-soul-revert-builtin"]')
const soulTextarea = (container: HTMLElement) =>
  q<HTMLTextAreaElement>(container, '[data-testid="agent-soul-textarea"]')

beforeEach(() => {
  agentBuiltinPrompt.mockReset()
  agentBuiltinPrompt.mockResolvedValue({ systemKey: 'titler', soul: BUILTIN_SOUL })
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('AgentSettingsForm built-in prompt revert', () => {
  it('restores the compiled-in prompt into the soul editor', async () => {
    const { container } = renderForm(customization)
    expect(soulTextarea(container).value).toBe('elle degistirilmis prompt')

    await act(async () => revertButton(container).click())

    expect(agentBuiltinPrompt).toHaveBeenCalledWith('AGT2')
    expect(soulTextarea(container).value).toBe(BUILTIN_SOUL)
  })

  it('stages the revert as an unsaved edit rather than applying it immediately', async () => {
    const { container, onSave } = renderForm(customization)
    await act(async () => revertButton(container).click())
    expect(onSave).not.toHaveBeenCalled()

    const save = q<HTMLButtonElement>(container, '[data-testid="agent-save"]')
    await act(async () => save.click())
    expect(onSave).toHaveBeenCalledTimes(1)
    // The revert keeps soul PINNED: the customisation deliberately serves the
    // role with this text, it does not fall back to inheriting the parent.
    expect(onSave.mock.calls[0][0]).toMatchObject({ soul: BUILTIN_SOUL })
  })

  it('disables the button once the editor already holds the built-in text', async () => {
    const { container } = renderForm(customization)
    await act(async () => revertButton(container).click())
    expect(revertButton(container).disabled).toBe(true)
  })

  it('fetches the built-in prompt only once across repeated reverts', async () => {
    const { container } = renderForm(customization)
    await act(async () => revertButton(container).click())

    await act(async () => typeInto(soulTextarea(container), 'tekrar degistirildi'))
    expect(revertButton(container).disabled).toBe(false)

    await act(async () => revertButton(container).click())
    expect(soulTextarea(container).value).toBe(BUILTIN_SOUL)
    expect(agentBuiltinPrompt).toHaveBeenCalledTimes(1)
  })

  it('offers no revert on an agent with no system role', () => {
    const plain = { ...customization, system: false, systemKey: undefined } as Agent
    const { container } = renderForm(plain)
    expect(container.querySelector('[data-testid="agent-soul-revert-builtin"]')).toBeNull()
  })

  // A built-in's soul is editable now, so reverting it to the compiled prompt is
  // exactly as meaningful there as on a customisation.
  it('offers the revert on a locked built-in, whose soul is editable', () => {
    const { container } = renderForm(parent)
    expect(container.querySelector('[data-testid="agent-soul-revert-builtin"]')).not.toBeNull()
  })
})
