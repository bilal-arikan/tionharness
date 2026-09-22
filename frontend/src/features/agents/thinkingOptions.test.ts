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
            id: 'deepseek-flash',
            label: 'DeepSeek V4.1 Flash',
            thinkingClass: 'effort',
            thinkingTiers: ['off', 'low', 'high', 'max'],
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

  it('disables unsupported upper tiers for legacy catalog models', () => {
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
    ]

    for (const options of [agentOptions, chatOptions]) {
      const filtered = thinkingOptionsForModel(options, catalog, 'claude-cli', 'legacy-model', '')
      for (const tier of ['xhigh', 'max', 'ultra']) {
        expect(filtered.find((option) => option.value === tier)?.disabled).toBe(true)
      }
    }
  })

  it('folds in-between tiers on effort-class models and greys off where reasoning cannot stop', () => {
    const catalog: CatalogEntry[] = [
      {
        id: 'deepseek',
        label: 'DeepSeek',
        needsKey: true,
        allowCustomModel: true,
        appliesToolHooks: true,
        available: true,
        models: [
          {
            id: 'deepseek-flash',
            label: 'DeepSeek V4.1 Flash',
            thinkingClass: 'effort',
            thinkingTiers: ['off', 'low', 'high', 'max'],
          },
        ],
      },
      {
        id: 'zai',
        label: 'Z.ai GLM',
        needsKey: true,
        allowCustomModel: true,
        appliesToolHooks: true,
        available: true,
        models: [
          {
            id: 'glm-5.3',
            label: 'GLM-5.3',
            thinkingClass: 'effort',
            thinkingTiers: ['low', 'high', 'max'],
          },
        ],
      },
    ]

    const flash = thinkingOptionsForModel(agentOptions, catalog, 'deepseek', 'deepseek-flash', '')
    const tier = (value: string) => flash.find((option) => option.value === value)
    for (const value of ['off', 'low', 'high', 'max']) {
      expect(tier(value)?.disabled).not.toBe(true)
    }
    expect(tier('medium')?.disabled).toBe(true)
    expect(tier('medium')?.hint).toContain('Yüksek')
    for (const value of ['xhigh', 'ultra']) {
      expect(tier(value)?.disabled).toBe(true)
      expect(tier(value)?.hint).toContain('Maks')
    }

    const glm = thinkingOptionsForModel(chatOptions, catalog, 'zai', 'glm-5.3', '')
    const off = glm.find((option) => option.value === 'off')
    expect(off?.disabled).toBe(true)
    expect(off?.hint).toContain('kapatılamaz')
    expect(glm.find((option) => option.value === 'max')?.disabled).not.toBe(true)
  })

  it('explains the ultra tier on native-effort providers as a max fallback, not high', () => {
    const catalog: CatalogEntry[] = [
      {
        id: 'anthropic',
        label: 'Anthropic',
        needsKey: true,
        allowCustomModel: true,
        appliesToolHooks: false,
        available: true,
        models: [
          {
            id: 'claude-opus-4-8',
            label: 'Opus 4.8',
            thinkingClass: 'adaptive',
            // No "ultra": output_config.effort cannot carry it.
            thinkingTiers: ['off', 'low', 'medium', 'high', 'xhigh', 'max'],
          },
        ],
      },
    ]
    const filtered = thinkingOptionsForModel(
      agentOptions,
      catalog,
      'anthropic',
      'claude-opus-4-8',
      '',
    )
    const ultra = filtered.find((option) => option.value === 'ultra')
    expect(ultra?.disabled).toBe(true)
    expect(ultra?.hint).toContain('Maks')
    expect(filtered.find((option) => option.value === 'max')?.disabled).not.toBe(true)
  })
})
