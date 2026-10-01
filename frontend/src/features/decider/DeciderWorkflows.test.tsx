// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { i18next } from '@/i18n'
import type {
  DeciderAuthority,
  DeciderConfig,
  DeciderModelInstance,
  DeciderView,
} from '@/types/decider'
import { DeciderWorkflows } from './DeciderWorkflows'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const authority = (id: string, group: string): DeciderAuthority => ({
  id,
  group,
  label: id,
  description: `Configure ${id}`,
  pattern: 'select',
  modes: ['off', 'shadow', 'on'],
  defaultMode: 'off',
  defaultThreshold: 0.8,
  thresholdHint: '',
  explicit: false,
})
const authorities = [
  authority('tool-risk', 'safety'),
  authority('worker-review', 'collaboration'),
  authority('compact-retention', 'context'),
  authority('session-setup', 'session'),
  authority('model-router', 'session'),
]
const draft: DeciderConfig = { enabled: true, defaultModel: 'DM1', authorities: {} }
const view: DeciderView = {
  authorities,
  groups: ['safety', 'session', 'context', 'collaboration'],
  config: draft,
  status: { enabled: true, ready: true },
  models: ['DM1', 'DM2'].map((id) => ({ id, label: id, enabled: true }) as DeciderModelInstance),
  backends: [],
  providerCandidates: {},
  stats: [],
  recent: [],
  statsDays: 7,
}

let root: Root
let container: HTMLDivElement
const onChange = vi.fn()
const select = (id: string) =>
  container.querySelector<HTMLButtonElement>(`[data-testid="decider-workflow-select-${id}"]`)!
const render = (nextView = view, nextDraft = draft) => {
  act(() => root.render(<DeciderWorkflows view={nextView} draft={nextDraft} onChange={onChange} />))
}

beforeEach(async () => {
  await i18next.changeLanguage('en')
  onChange.mockReset()
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  document.body.replaceChildren()
})

it('lists every registered workflow in group order without synthetic entries', () => {
  render()
  const choices = [...container.querySelectorAll<HTMLButtonElement>('nav button')]
  expect(choices.map((button) => button.dataset.testid)).toEqual([
    'decider-workflow-select-session-setup',
    'decider-workflow-select-model-router',
    'decider-workflow-select-compact-retention',
    'decider-workflow-select-worker-review',
    'decider-workflow-select-tool-risk',
  ])
  expect(select('session-setup').getAttribute('aria-pressed')).toBe('true')
  expect(container.querySelectorAll('[data-testid^="decider-authority-"]')).toHaveLength(1)
})

it('switches the detail controls and edits the selected workflow in the shared draft', () => {
  render()
  act(() => select('model-router').click())
  expect(select('model-router').getAttribute('aria-pressed')).toBe('true')
  expect(select('session-setup').getAttribute('aria-pressed')).toBe('false')
  expect(container.querySelector('[data-testid="decider-authority-session-setup"]')).toBeNull()
  expect(
    container.querySelector('[data-testid="decider-model-select-model-router"]'),
  ).not.toBeNull()
  expect(
    container.querySelector('[data-testid="decider-fallback-select-model-router"]'),
  ).not.toBeNull()
  expect(
    container.querySelector('[data-testid="decider-challenger-select-model-router"]'),
  ).not.toBeNull()
  const enabled = container.querySelector<HTMLButtonElement>(
    '[data-testid="decider-mode-model-router-option"][data-value="on"]',
  )!
  act(() => enabled.click())
  expect(onChange).toHaveBeenCalledWith({
    ...draft,
    authorities: {
      'model-router': expect.objectContaining({ mode: 'on', threshold: 0.8 }),
    },
  })
  const detail = container.querySelector('[data-testid="decider-workflow-detail"]')!
  expect(select('model-router').getAttribute('aria-controls')).toBe(detail.id)
})

it('shows the global pause and disables the threshold without losing configured modes', () => {
  render(view, {
    ...draft,
    enabled: false,
    authorities: { 'session-setup': { mode: 'shadow', threshold: 0.85 } },
  })
  expect(container.querySelector('[data-testid="decider-workflows-disabled"]')).not.toBeNull()
  expect(container.querySelector<HTMLInputElement>('input[type="range"]')!.disabled).toBe(true)
  expect(
    container
      .querySelector('[data-testid="decider-mode-session-setup-option"][data-value="shadow"]')!
      .getAttribute('aria-checked'),
  ).toBe('true')
})

it('falls back to the first current registry entry when the selection is removed', () => {
  render()
  act(() => select('model-router').click())
  render({ ...view, authorities: [authority('worker-review', 'collaboration')] })
  expect(container.querySelectorAll('nav button')).toHaveLength(1)
  expect(select('worker-review').getAttribute('aria-pressed')).toBe('true')
  expect(container.querySelector('[data-testid="decider-authority-worker-review"]')).not.toBeNull()
  expect(onChange).not.toHaveBeenCalled()
})

it('shows an empty state without creating selectable defaults', () => {
  render({ ...view, authorities: [] })
  expect(container.querySelector('[data-testid="decider-workflows-empty"]')).not.toBeNull()
  expect(container.querySelector('nav')).toBeNull()
  expect(container.querySelector('[data-testid="decider-workflow-detail"]')).toBeNull()
  expect(onChange).not.toHaveBeenCalled()
})
