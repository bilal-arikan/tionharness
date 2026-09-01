import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api', () => ({ api: { getCatalog: vi.fn() } }))

import { THINKING_OPTIONS as agentOptions } from './agentOptions'
import { THINKING_OPTIONS as chatOptions } from '@/features/chat/composer/pickerOptions'
import { thinkingOptionsForModel } from '@/shared/lib/catalog'
import type { CatalogEntry } from '@/types'

describe('thinking options', () => {
  it('keeps agent and chat ramps aligned through ultra', () => {
    const ramp = ['off', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']
    expect(agentOptions.map((option) => option.value)).toEqual(ramp)
    expect(chatOptions.map((option) => option.value)).toEqual(['', ...ramp])
  })

  it('enables Codex GPT-5.6 Sol upper tiers in Agent Settings and chat composer', () => {
    const fullRamp = ['off', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra']
    const catalog: CatalogEntry[] = [
      {
        id: 'codex-cli',
        label: 'Codex',
        needsKey: false,
        allowCustomModel: true,
        appliesToolHooks: false,
        available: true,
        models: [
          {
            id: 'gpt-5.6-sol',
            label: 'GPT-5.6 Sol',
            thinkingClass: 'adaptive',
            thinkingTiers: fullRamp,
          },
        ],
      },
      {
        id: 'claude-cli',
        label: 'Claude',
        needsKey: false,
        allowCustomModel: true,
        appliesToolHooks: true,
        available: true,
        models: [
          {
            id: 'gpt-5.6-sol',
            label: 'GPT-5.6 Sol',
            thinkingClass: 'legacy',
            thinkingTiers: fullRamp.slice(0, 4),
          },
        ],
      },
      {
        id: 'deepseek',
        label: 'DeepSeek',
        needsKey: true,
        allowCustomModel: true,
        appliesToolHooks: true,
        available: true,
        models: [
          {
            id: 'deepseek-v4-flash',
            label: 'DeepSeek V4 Flash',
            thinkingClass: 'non-thinking',
            thinkingTiers: ['off'],
          },
        ],
      },
    ]

    const agentSettings = thinkingOptionsForModel(
      agentOptions,
      catalog,
      'codex-cli',
      'gpt-5.6-sol',
      '',
    )
    const chatComposer = thinkingOptionsForModel(
      chatOptions,
      catalog,
      'codex-cli',
      'gpt-5.6-sol',
      '',
    )

    for (const tier of ['xhigh', 'max', 'ultra']) {
      expect(agentSettings.find((option) => option.value === tier)?.disabled).not.toBe(true)
      expect(chatComposer.find((option) => option.value === tier)?.disabled).not.toBe(true)
    }
  })

  it('disables unsupported upper tiers for legacy and non-thinking catalog models', () => {
    const catalog: CatalogEntry[] = [
      {
        id: 'claude-cli',
        label: 'Claude',
        needsKey: false,
        allowCustomModel: true,
        appliesToolHooks: true,
        available: true,
        models: [
          {
            id: 'legacy-model',
            label: 'Legacy model',
            thinkingClass: 'legacy',
            thinkingTiers: ['off', 'low', 'medium', 'high'],
          },
        ],
      },
      {
        id: 'deepseek',
        label: 'DeepSeek',
        needsKey: true,
        allowCustomModel: true,
        appliesToolHooks: true,
        available: true,
        models: [
          {
            id: 'deepseek-v4-flash',
            label: 'DeepSeek V4 Flash',
            thinkingClass: 'non-thinking',
            thinkingTiers: ['off'],
          },
        ],
      },
    ]

    for (const [provider, model, options] of [
      ['claude-cli', 'legacy-model', agentOptions],
      ['deepseek', 'deepseek-v4-flash', chatOptions],
    ] as const) {
      const filtered = thinkingOptionsForModel(options, catalog, provider, model, '')
      for (const tier of ['xhigh', 'max', 'ultra']) {
        expect(filtered.find((option) => option.value === tier)?.disabled).toBe(true)
      }
    }
  })
})
