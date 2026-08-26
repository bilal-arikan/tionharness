import { describe, expect, it, vi } from 'vitest'
import { saveDefaultAgent } from './defaultAgentSave'

describe('saveDefaultAgent', () => {
  it('restores the previous agent and reports a failed API write', async () => {
    const setCurrent = vi.fn()
    const reportError = vi.fn()
    const persist = vi.fn().mockRejectedValue(new Error('workspace settings unavailable'))

    const saved = await saveDefaultAgent({
      nextId: 'agent-new',
      previousId: 'agent-old',
      setCurrent,
      persist,
      reportError,
    })

    expect(saved).toBe(false)
    expect(setCurrent.mock.calls).toEqual([['agent-new'], ['agent-old']])
    expect(reportError).toHaveBeenCalledWith('workspace settings unavailable')
  })
})
