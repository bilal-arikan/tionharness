import { describe, expect, it, vi } from 'vitest'
import type { Agent } from '@/types'
import { updateAgentProviderModels } from './agentBulkEdit'

function agent(id: string, system = false): Agent {
  return {
    id,
    name: id,
    soul: '',
    identity: '',
    system,
    provider: 'claude-cli',
    providerInstanceId: 'claude-cli',
    model: 'old-model',
    mcpEnabled: true,
    allowedTools: '',
    blockedTools: '',
    createdAt: 0,
    updatedAt: 0,
  }
}

describe('updateAgentProviderModels', () => {
  it('patches every editable agent with only provider instance and model', async () => {
    const update = vi.fn().mockResolvedValue(undefined)

    await updateAgentProviderModels(
      [agent('AGT1'), agent('SYS1', true), agent('AGT2')],
      'PRV7',
      'new-model',
      update,
    )

    expect(update.mock.calls).toEqual([
      ['AGT1', { provider: 'PRV7', model: 'new-model' }],
      ['AGT2', { provider: 'PRV7', model: 'new-model' }],
    ])
  })

  it('patches only system agents in the system scope', async () => {
    const update = vi.fn().mockResolvedValue(undefined)

    await updateAgentProviderModels(
      [agent('AGT1'), agent('SYS1', true), agent('SYS2', true)],
      'PRV7',
      'new-model',
      update,
      'system',
    )

    expect(update.mock.calls).toEqual([
      ['SYS1', { provider: 'PRV7', model: 'new-model' }],
      ['SYS2', { provider: 'PRV7', model: 'new-model' }],
    ])
  })

  // A caller re-fetches the roster on failure; rejecting while a sibling write is
  // still in flight would let that fetch miss the row it is about to change.
  it('settles every write before rejecting with the first failure', async () => {
    let finishSlow = () => {}
    let slowDone = false
    const update = vi.fn((id: string) =>
      id === 'AGT1'
        ? Promise.reject(new Error('AGT1 failed'))
        : new Promise<void>((resolve) => {
            finishSlow = () => {
              slowDone = true
              resolve()
            }
          }),
    )

    const result = updateAgentProviderModels(
      [agent('AGT1'), agent('AGT2')],
      'PRV7',
      'new-model',
      update,
    )
    let settled = false
    void result
      .catch(() => {})
      .finally(() => {
        settled = true
      })
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(settled).toBe(false)

    finishSlow()
    await expect(result).rejects.toThrow('AGT1 failed')
    expect(slowDone).toBe(true)
  })

  it('rejects a missing provider instance instead of silently skipping the update', async () => {
    const update = vi.fn().mockResolvedValue(undefined)

    await expect(updateAgentProviderModels([agent('AGT1')], '', 'model', update)).rejects.toThrow(
      'provider instance is required',
    )
    expect(update).not.toHaveBeenCalled()
  })
})
