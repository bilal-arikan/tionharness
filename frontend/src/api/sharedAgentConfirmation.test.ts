// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { req, setActiveWorkspace } from './client'

afterEach(() => vi.restoreAllMocks())

const conflict = () =>
  new Response(
    JSON.stringify({
      error: 'Confirmation required',
      confirmationRequired: true,
      agentName: 'Reviewer',
      workspaces: [{ workspaceName: 'Studio' }, { workspaceName: 'Research' }],
    }),
    { status: 409 },
  )

describe('shared agent write confirmation', () => {
  it('names affected workspaces and retries the same write after confirmation', async () => {
    setActiveWorkspace('WS2')
    const fetch = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(conflict())
      .mockResolvedValueOnce(new Response('{"saved":true}'))
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
    const body = JSON.stringify({ soul: 'Reviewed' })
    await expect(req('/api/agents/AGT1', { method: 'PUT', body })).resolves.toEqual({ saved: true })
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('Studio\n• Research'))
    expect(fetch).toHaveBeenLastCalledWith(
      '/api/agents/AGT1',
      expect.objectContaining({
        method: 'PUT',
        body,
        headers: expect.objectContaining({
          'X-Workspace-Id': 'WS2',
          'X-Confirm-Shared-Agent': 'true',
        }),
      }),
    )
  })
  it('does not send an authorized mutation when cancelled', async () => {
    const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(conflict())
    vi.spyOn(window, 'confirm').mockReturnValue(false)
    await expect(req('/api/agent-catalog/AGT1', { method: 'PUT', body: '{}' })).rejects.toThrow(
      'cancelled',
    )
    expect(fetch).toHaveBeenCalledTimes(1)
  })
  it('does not prompt for unrelated conflicts', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response('{"error":"Agent is running"}', { status: 409 }),
    )
    const confirm = vi.spyOn(window, 'confirm')
    await expect(req('/api/agents/AGT1', { method: 'DELETE' })).rejects.toThrow('Agent is running')
    expect(confirm).not.toHaveBeenCalled()
  })
})
