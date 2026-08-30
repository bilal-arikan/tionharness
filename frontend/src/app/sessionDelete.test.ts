import { describe, expect, it, vi } from 'vitest'
import { deleteSessionAndRefresh } from './sessionDelete'

describe('deleteSessionAndRefresh', () => {
  it('refreshes sessions after confirmed deletion', async () => {
    const calls: string[] = []
    const deleteSession = vi.fn(async () => {
      calls.push('delete')
    })
    const refreshSessions = vi.fn(() => {
      calls.push('refresh')
    })

    await deleteSessionAndRefresh('session-1', deleteSession, refreshSessions)

    expect(deleteSession).toHaveBeenCalledWith('session-1')
    expect(refreshSessions).toHaveBeenCalledOnce()
    expect(calls).toEqual(['delete', 'refresh'])
  })

  it('does not refresh when deletion fails', async () => {
    const deleteSession = vi.fn(async () => {
      throw new Error('delete failed')
    })
    const refreshSessions = vi.fn()

    await expect(
      deleteSessionAndRefresh('session-1', deleteSession, refreshSessions),
    ).rejects.toThrow('delete failed')
    expect(refreshSessions).not.toHaveBeenCalled()
  })
})
