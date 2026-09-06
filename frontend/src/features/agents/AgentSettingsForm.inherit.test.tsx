// @vitest-environment jsdom

import { act, type ButtonHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Agent, AgentOverrideKey, AgentPatch } from '@/types'

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

  // A built-in is edited IN PLACE now: its edit is stored installation-wide, so
  // there is nothing to copy and no "Özelleştir" action to offer.
  it('renders a locked built-in as editable, with no customise copy action', () => {
    const { container } = renderForm(parent, { onDerive: vi.fn() })
    expect(container.querySelector('[data-testid="agent-save"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="agent-locked-note"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="agent-customize"]')).toBeNull()
    // The fields are live: no enclosing disabled fieldset.
    expect(
      q<HTMLTextAreaElement>(container, '[data-testid="agent-soul-textarea"]').closest(
        'fieldset[disabled]',
      ),
    ).toBeNull()
  })

  // Editing a built-in reaches every workspace, so the note has to say so.
  it('tells the user a built-in edit applies to every workspace', () => {
    const { container } = renderForm(parent)
    const note = container.querySelector('[data-testid="agent-locked-note"]')?.textContent ?? ''
    expect(note).toContain('tüm workspace')
  })

  // What a built-in still cannot be: deleted or disabled.
  it('offers no delete or disable action on a built-in', () => {
    const { container } = renderForm(parent, {
      onDelete: vi.fn(),
      onToggleDisabled: vi.fn(),
    })
    expect(container.querySelector('[data-testid="agent-delete"]')).toBeNull()
    expect(container.querySelector('[data-testid="agent-toggle-disabled"]')).toBeNull()
  })

  it('renders an editable agent read-only when the screen only inspects it', () => {
    const { container, onSave } = renderForm(child, {
      readOnly: true,
      readOnlyNote: 'Ayarlar ekranını kullan',
      onDelete: vi.fn(),
      onToggleDisabled: vi.fn(),
      onDuplicate: vi.fn(),
    })
    // No mutating action, and every field disabled through the fieldset.
    for (const id of ['agent-save', 'agent-delete', 'agent-toggle-disabled', 'agent-duplicate'])
      expect(container.querySelector(`[data-testid="${id}"]`)).toBeNull()
    expect(
      q<HTMLTextAreaElement>(container, '[data-testid="agent-soul-textarea"]').closest(
        'fieldset[disabled]',
      ),
    ).not.toBeNull()
    expect(onSave).not.toHaveBeenCalled()
    // The read-only note points elsewhere; the built-in note does NOT appear,
    // because this agent is a customisation, not a locked built-in.
    expect(container.querySelector('[data-testid="agent-readonly-note"]')?.textContent).toContain(
      'Ayarlar',
    )
    expect(container.querySelector('[data-testid="agent-locked-note"]')).toBeNull()
  })

  it('offers no customise action on a read-only agent that is not a built-in', () => {
    const { container } = renderForm(child, { readOnly: true, onDerive: vi.fn() })
    expect(container.querySelector('[data-testid="agent-customize"]')).toBeNull()
    expect(container.querySelector('[data-testid="agent-derive"]')).toBeNull()
  })

  it('hides the free derive action when disallowed', () => {
    const { container } = renderForm(parent, { onDerive: vi.fn(), allowFreeDerive: false })
    expect(container.querySelector('[data-testid="agent-derive"]')).toBeNull()
  })

  // "Restore defaults" on a built-in drops the installation-wide customisation.
  it('offers a reset on a built-in that carries customised fields', () => {
    const onRestoreDefault = vi.fn()
    const customised = { ...parent, overrides: ['soul'] as AgentOverrideKey[] }
    const { container } = renderForm(customised, { onRestoreDefault })
    const button = q<HTMLButtonElement>(container, '[data-testid="agent-restore-default"]')
    expect(button.disabled).toBe(false)
    act(() => button.click())
    expect(onRestoreDefault).toHaveBeenCalled()
  })
})
