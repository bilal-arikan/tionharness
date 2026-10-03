// @vitest-environment jsdom

import { act, createElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sessionStartTest as calls } from '@/test/sessionStartEnvironment'
import { render, query } from '@/test/render'
import type { Agent } from '@/types'
import App from './App'
import { useSessionsController } from './useSessionsController'

const service = { id: 'SERVICE', name: 'Titler', system: true, systemKey: 'titler' } as Agent
const archived = { id: 'OLD', name: 'Archived developer', archived: true } as Agent
const ada = { id: 'ADA', name: 'Ada' } as Agent
const bryn = { id: 'BRYN', name: 'Bryn' } as Agent

beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  localStorage.setItem('tionharness.hasSetup', '1')
  calls.listAgents.mockResolvedValue([service, archived, ada, bryn])
  // Failed settings leave the default empty without overwriting stored preferences.
  calls.getWorkspaceSettings.mockRejectedValue(new Error('settings unavailable'))
  calls.listSessions.mockResolvedValue({ items: [], total: 0, hasMore: false })
  calls.getSessionsByIds.mockResolvedValue([])
  calls.activeSessions.mockResolvedValue([])
  calls.listArtifacts.mockResolvedValue({ items: [] })
  calls.createSession.mockImplementation(async (agentId: string) => ({
    id: 'NEW',
    agentId,
    kind: 'chat',
    messageCount: 0,
  }))
})

describe('session-start agent wiring', () => {
  it('offers only startable agents through the actual app and empty chat view', async () => {
    const { container } = render(createElement(App))
    await act(async () => {})
    const select = query<HTMLSelectElement>(container, '[data-testid="chat-empty-agent-select"]')
    expect([...select.options].map((option) => option.value)).toEqual(['ADA', 'BRYN'])
    expect(query<HTMLButtonElement>(container, '[data-testid="sidebar-new-chat"]').disabled).toBe(
      false,
    )
  })

  it('disables the app new-chat button when only service or archived agents exist', async () => {
    calls.listAgents.mockResolvedValue([service, archived])
    const { container } = render(createElement(App))
    await act(async () => {})
    const button = query<HTMLButtonElement>(container, '[data-testid="sidebar-new-chat"]')
    expect(button.disabled).toBe(true)
    expect(container.querySelector('[data-testid="chat-empty-agent-select"]')).toBeNull()
    act(() => button.click())
    expect(calls.createSession).not.toHaveBeenCalled()
  })

  it('falls back to a startable agent when settings did not provide a default', async () => {
    let controller: ReturnType<typeof useSessionsController> | undefined
    const setError = vi.fn()
    function Harness() {
      controller = useSessionsController({
        activeWorkspaceId: 'WS1',
        chipsParam: '',
        showArchived: false,
        setError,
        setView: calls.noop,
      })
      return null
    }
    render(createElement(Harness))
    await act(async () => {})
    expect(controller!.defaultAgentId).toBeNull()
    expect(controller!.sessionStartAgents.map((agent) => agent.id)).toEqual(['ADA', 'BRYN'])
    await act(async () => controller!.newSession())
    expect(calls.createSession).toHaveBeenCalledWith('ADA')
    expect(setError).not.toHaveBeenCalled()
  })
})
