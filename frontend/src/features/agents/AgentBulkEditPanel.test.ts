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

  it('rejects a missing provider instance instead of silently skipping the update', async () => {
    const update = vi.fn().mockResolvedValue(undefined)

    await expect(updateAgentProviderModels([agent('AGT1')], '', 'model', update)).rejects.toThrow(
      'provider instance is required',
    )
    expect(update).not.toHaveBeenCalled()
  })
})
