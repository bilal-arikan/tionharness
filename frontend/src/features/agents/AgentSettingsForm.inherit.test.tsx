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

const roots: Root[] = []
const parent = {
  id: 'AGT1',
  name: 'Titler',
  soul: 'parent soul',
  identity: 'parent identity',
  provider: 'claude-cli',
  model: 'haiku',
  permissionMode: 'auto',
  system: true,
  systemKey: 'titler',
  locked: true,
} as Agent
// The server serves a child RESOLVED: soul below is the child's own value
// (overridden), everything else is the parent's.
const child = {
  ...parent,
  id: 'AGT2',
  name: 'Titler (özel)',
  soul: 'child soul',
  locked: false,
  parentId: parent.id,
  overrides: ['soul'],
} as Agent

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

async function clickSave(container: HTMLElement) {
  const save = q<HTMLButtonElement>(container, '[data-testid="agent-save"]')
  await act(async () => save.click())
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('AgentSettingsForm inheritance', () => {
  it('marks overridden vs inherited fields on a derived agent', () => {
    const { container } = renderForm(child)
    expect(
      container.querySelector('[data-testid="agent-field-overridden"][data-field="soul"]'),
    ).not.toBeNull()
    expect(
      container.querySelector('[data-testid="agent-field-inherited"][data-field="identity"]'),
    ).not.toBeNull()
    expect(container.querySelector('[data-testid="agent-field-inherited"]')?.textContent).toContain(
      'Titler',
    )
  })

  it('saves only the pinned fields, so untouched fields keep inheriting', async () => {
    const { container, onSave } = renderForm(child)
    await clickSave(container)
    expect(onSave).toHaveBeenCalledTimes(1)
    const patch = onSave.mock.calls[0][0]
    expect(patch).toEqual({ name: 'Titler (özel)', soul: 'child soul' })
    expect(patch).not.toHaveProperty('identity')
    expect(patch).not.toHaveProperty('model')
  })

  it('pins a field once it is edited', async () => {
    const { container, onSave } = renderForm(child)
    const identity = q<HTMLTextAreaElement>(container, '[data-testid="agent-identity-textarea"]')
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value')?.set
      setter?.call(identity, 'own identity')
      identity.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(
      container.querySelector('[data-testid="agent-field-overridden"][data-field="identity"]'),
    ).not.toBeNull()
    await clickSave(container)
    expect(onSave.mock.calls[0][0]).toEqual(
      expect.objectContaining({ soul: 'child soul', identity: 'own identity' }),
    )
  })

  it('releases an override: shows the parent value and sends resetFields', async () => {
    const { container, onSave } = renderForm(child)
    const reset = q<HTMLButtonElement>(
      container,
      '[data-testid="agent-field-reset"][data-field="soul"]',
    )
    act(() => reset.click())
    expect(q<HTMLTextAreaElement>(container, '[data-testid="agent-soul-textarea"]').value).toBe(
      'parent soul',
    )
    expect(
      container.querySelector('[data-testid="agent-field-inherited"][data-field="soul"]'),
    ).not.toBeNull()
    await clickSave(container)
    expect(onSave.mock.calls[0][0]).toEqual({ name: 'Titler (özel)', resetFields: ['soul'] })
  })

  it('renders a locked built-in read-only with a customise action', () => {
    const onDerive = vi.fn(async () => 'AGT9')
    const { container } = renderForm(parent, { onDerive })
    expect(container.querySelector('[data-testid="agent-save"]')).toBeNull()
    expect(container.querySelector('[data-testid="agent-locked-note"]')).not.toBeNull()
    // Inputs are disabled through the enclosing <fieldset disabled>; the
    // textarea's own attribute stays untouched.
    expect(
      q<HTMLTextAreaElement>(container, '[data-testid="agent-soul-textarea"]').closest(
        'fieldset[disabled]',
      ),
    ).not.toBeNull()
    act(() => q<HTMLButtonElement>(container, '[data-testid="agent-customize"]').click())
    expect(onDerive).toHaveBeenCalledWith({ bindRole: true })
  })

  it('points at the existing customisation instead of offering a second one', () => {
    const onSelectAgent = vi.fn()
    const { container } = renderForm(parent, {
      onDerive: vi.fn(),
      roleCustomization: child,
      onSelectAgent,
    })
    expect(container.querySelector('[data-testid="agent-customize"]')).toBeNull()
    act(() => q<HTMLButtonElement>(container, '[data-testid="agent-open-customization"]').click())
    expect(onSelectAgent).toHaveBeenCalledWith(child.id)
  })
})
