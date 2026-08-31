// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Agent, Message } from '@/types'
import { MessageList } from './MessageList'

vi.mock('./AssistantTurn', () => ({
  AssistantTurn: ({ agent, message }: { agent?: Agent; message: Message }) => (
    <div data-testid={`assistant-${message.id}`} data-agent-id={agent?.id} />
  ),
}))

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []

const agent = (id: string, name: string): Agent =>
  ({
    id,
    name,
    soul: '',
    identity: '',
    provider: 'anthropic',
    model: 'claude-opus-5',
    mcpEnabled: true,
    allowedTools: '',
    blockedTools: '',
    skills: [],
    createdAt: 0,
    updatedAt: 0,
  }) as Agent

const message = (id: string, agentId?: string): Message => ({
  id,
  sessionId: 'SES1',
  role: 'assistant',
  agentId,
  text: '',
  createdAt: 1,
})

const agents = [agent('AGT-active', 'Active'), agent('AGT-explicit', 'Explicit')]

function renderList(overrides: Partial<Parameters<typeof MessageList>[0]> = {}) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(
      <MessageList
        messages={[message('ghost')]}
        pending={false}
        pendingAgentId="AGT-active"
        agents={agents}
        streaming
        {...overrides}
      />,
    ),
  )
  return container
}

function renderedAgent(container: HTMLElement, messageId: string) {
  return container
    .querySelector(`[data-testid="assistant-${messageId}"]`)
    ?.getAttribute('data-agent-id')
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('MessageList ghost assistant agent fallback', () => {
  it('attributes the last live ghost message to the pending agent', () => {
    const container = renderList()

    expect(renderedAgent(container, 'ghost')).toBe('AGT-active')
  })

  it('prefers an explicit message agent over the pending agent fallback', () => {
    const container = renderList({ messages: [message('ghost', 'AGT-explicit')] })

    expect(renderedAgent(container, 'ghost')).toBe('AGT-explicit')
  })

  it('does not apply the pending agent fallback to completed or earlier messages', () => {
    const completed = renderList({ streaming: false })
    const earlier = renderList({ messages: [message('earlier'), message('last', 'AGT-explicit')] })

    expect(renderedAgent(completed, 'ghost')).toBeNull()
    expect(renderedAgent(earlier, 'earlier')).toBeNull()
  })
})

// The CLI cold-start divider is backend-attributed (db.Message.CLIColdStart), so
// it must render off that flag alone — independent of the timestamp gap the cache
// divider is derived from.
describe('MessageList CLI cold-start divider', () => {
  const dividerText = (container: HTMLElement) => container.textContent ?? ''

  it('draws the divider above a turn that restarted the CLI session', () => {
    const flagged: Message = { ...message('cold'), cliColdStart: true }
    const container = renderList({ messages: [flagged], streaming: false })

    expect(dividerText(container)).toContain('yeni CLI oturumu')
  })

  it('draws nothing for an ordinary warm-resumed turn', () => {
    const container = renderList({ messages: [message('warm')], streaming: false })

    expect(dividerText(container)).not.toContain('yeni CLI oturumu')
  })

  it('stacks both dividers when a long gap also broke the CLI thread', () => {
    const earlier = message('earlier')
    // Two hours apart: past the 1h cache TTL, so the cache divider applies too.
    const flagged: Message = { ...message('cold'), createdAt: 7300, cliColdStart: true }
    const container = renderList({ messages: [earlier, flagged], streaming: false })

    expect(dividerText(container)).toContain('yeni CLI oturumu')
    expect(dividerText(container)).toContain('cache soğudu')
  })
})
