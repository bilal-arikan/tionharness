// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Agent } from '@/types'
import { AgentPicker } from './AgentPicker'
import { archivedAgentLabel, pickableAgents } from './pickableAgents'

vi.mock('@/shared/lib/catalog', () => ({
  useCatalog: () => [],
  resolveModelLabel: () => '',
}))

const live = { id: 'AGT1', name: 'Live' } as Agent
const archived = { id: 'AGT2', name: 'Old', archived: true } as Agent
const agents = [live, archived]

describe('pickableAgents', () => {
  it('hides archived agents from new selections', () => {
    expect(pickableAgents(agents).map((a) => a.id)).toEqual(['AGT1'])
    expect(pickableAgents(agents, 'AGT1').map((a) => a.id)).toEqual(['AGT1'])
  })

  it('keeps the currently-saved archived agent', () => {
    expect(pickableAgents(agents, 'AGT2').map((a) => a.id)).toEqual(['AGT1', 'AGT2'])
  })
})

describe('AgentPicker archived agents', () => {
  let container: HTMLDivElement
  let root: Root

  beforeEach(() => {
    ;(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(() => {
    act(() => root.unmount())
    container.remove()
  })

  const open = (value: string) => {
    act(() => root.render(<AgentPicker agents={agents} value={value} onChange={() => {}} />))
    const trigger = container.querySelector<HTMLButtonElement>(
      '[data-testid="agent-picker-trigger"]',
    )
    if (!trigger) throw new Error('agent picker trigger not rendered')
    act(() => trigger.click())
    return trigger
  }

  const optionIds = () =>
    Array.from(container.querySelectorAll('[data-testid="agent-picker-option"]')).map((el) =>
      el.getAttribute('data-agent-id'),
    )

  it('does not offer an archived agent for a new selection', () => {
    const trigger = open('')
    expect(optionIds()).toEqual(['AGT1'])
    expect(trigger.textContent).not.toContain(archivedAgentLabel())
  })

  it('still shows a saved archived agent, marked as archived', () => {
    const trigger = open('AGT2')
    expect(optionIds()).toEqual(['AGT1', 'AGT2'])
    expect(trigger.textContent).toContain('Old')
    expect(trigger.textContent).toContain(archivedAgentLabel())
    expect(container.querySelectorAll('[data-testid="agent-picker-archived"]')).toHaveLength(2)
  })
})
