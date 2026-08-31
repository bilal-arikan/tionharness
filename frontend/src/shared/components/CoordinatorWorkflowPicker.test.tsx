// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Skill } from '@/types'
import { CoordinatorWorkflowPicker } from './CoordinatorWorkflowPicker'

const apiMock = vi.hoisted(() => ({ listSkills: vi.fn() }))
vi.mock('@/api', () => ({ api: apiMock }))

function recipe(slug: string, name: string, pattern: string): Skill {
  return {
    slug,
    name,
    description: `${name} recipe`,
    source: 'global',
    kind: 'coordinator-workflow',
    pattern,
  }
}

const recipes: Skill[] = [
  recipe('fanout', 'Fan-out', 'fanout'),
  recipe('review', 'Review', 'adversarial'),
  recipe('classify', 'Classify', 'classify'),
  recipe('loop', 'Loop', 'loop'),
  recipe('tournament', 'Tournament', 'tournament'),
]

const roots: ReturnType<typeof createRoot>[] = []
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

function renderPicker(value = '', onChange = vi.fn()) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(
      <CoordinatorWorkflowPicker value={value} onChange={onChange} groupName="workflow-test" />,
    ),
  )
  return { container, onChange }
}

async function flush() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  apiMock.listSkills.mockResolvedValue(recipes)
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('CoordinatorWorkflowPicker', () => {
  it('keeps free first and selected by default while standard recipes stay visible', async () => {
    const { container } = renderPicker()
    await flush()

    const radios = [...container.querySelectorAll<HTMLInputElement>('input[type="radio"]')]
    expect(radios[0].checked).toBe(true)
    expect(radios[0].closest('label')?.textContent).toContain('Serbest')
    expect(container.textContent).toContain('Fan-out')
    expect(container.textContent).toContain('Review')
  })

  it('puts advanced recipes in a closed group that can be opened and selected', async () => {
    const onChange = vi.fn()
    const { container } = renderPicker('', onChange)
    await flush()

    const details = container.querySelector('details')
    expect(details?.open).toBe(false)
    expect(details?.textContent).toContain('Classify')
    expect(details?.textContent).toContain('Loop')
    expect(details?.textContent).toContain('Tournament')

    act(() => details?.querySelector('summary')?.click())
    expect(details?.open).toBe(true)
    const loop = [...details!.querySelectorAll<HTMLInputElement>('input[type="radio"]')].find(
      (radio) => radio.closest('label')?.textContent?.includes('Loop'),
    )
    act(() => loop?.click())
    expect(onChange).toHaveBeenCalledWith('loop')
  })

  it('opens the advanced group when its selected recipe loads', async () => {
    const { container } = renderPicker('loop')
    await flush()

    expect(container.querySelector('details')?.open).toBe(true)
    const loop = [...container.querySelectorAll<HTMLInputElement>('input[type="radio"]')].find(
      (radio) => radio.closest('label')?.textContent?.includes('Loop'),
    )
    expect(loop?.checked).toBe(true)
  })
})
