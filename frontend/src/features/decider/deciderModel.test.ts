import { describe, expect, it } from 'vitest'
import type { DeciderBackend, DeciderConfig, DeciderSite, DeciderSiteStats } from '@/types/decider'
import {
  MIN_COMPARISONS,
  adviceFor,
  agreement,
  isCustomModel,
  modelPrice,
  sameConfig,
  siteConfig,
  statusTone,
  withSite,
} from './deciderModel'

const site: DeciderSite = {
  id: 'tool-risk',
  label: 'Shell command risk check',
  description: '',
  modes: ['off', 'shadow', 'on'],
  defaultMode: 'shadow',
  defaultThreshold: 0.8,
  thresholdHint: '',
  explicit: false,
}

const config: DeciderConfig = {
  enabled: true,
  backend: 'openrouter',
  providerInstanceId: '',
  model: 'typesafe/jev-1.13',
  timeoutMs: 3000,
  sites: {},
}

const stats = (compared: number, agreed: number): DeciderSiteStats => ({
  site: 'tool-risk',
  calls: compared,
  errors: 0,
  shadow: compared,
  compared,
  agreed,
  applied: 0,
  p50Ms: 300,
  p95Ms: 700,
  costUsd: 0.001,
})

describe('site config', () => {
  it('falls back to the site defaults', () => {
    expect(siteConfig(config, site)).toEqual({ mode: 'shadow', threshold: 0.8 })
  })

  it('patches one site without touching the original', () => {
    const next = withSite(config, site, { mode: 'on' })
    expect(next.sites['tool-risk']).toEqual({ mode: 'on', threshold: 0.8 })
    expect(config.sites['tool-risk']).toBeUndefined()
  })

  it('compares configs regardless of site order', () => {
    const a = {
      ...config,
      sites: {
        x: { mode: 'on' as const, threshold: 0.7 },
        y: { mode: 'off' as const, threshold: 0.8 },
      },
    }
    const b = {
      ...config,
      sites: {
        y: { mode: 'off' as const, threshold: 0.8 },
        x: { mode: 'on' as const, threshold: 0.7 },
      },
    }
    expect(sameConfig(a, b)).toBe(true)
    expect(sameConfig(a, { ...b, model: 'other' })).toBe(false)
  })
})

describe('agreement advice', () => {
  it('has no agreement before any comparison', () => {
    expect(agreement(undefined)).toBeNull()
    expect(agreement(stats(0, 0))).toBeNull()
    expect(agreement(stats(10, 9))).toBeCloseTo(0.9)
  })

  it('asks for more data below the minimum', () => {
    expect(adviceFor('shadow', stats(12, 12))).toEqual({
      kind: 'collect',
      done: 12,
      needed: MIN_COMPARISONS,
    })
  })

  it('recommends switching on or staying in shadow', () => {
    expect(adviceFor('shadow', stats(100, 95))).toEqual({ kind: 'ready' })
    expect(adviceFor('shadow', stats(100, 70))).toEqual({ kind: 'keep' })
    expect(adviceFor('shadow', stats(100, 85))).toBeNull()
  })

  it('advises only in shadow mode', () => {
    expect(adviceFor('on', stats(100, 95))).toBeNull()
    expect(adviceFor('off', undefined)).toBeNull()
  })
})

describe('status and models', () => {
  const backend: DeciderBackend = {
    id: 'openrouter',
    label: 'OpenRouter Decisions',
    providerKinds: ['openrouter'],
    billingProvider: 'openrouter',
    models: [{ id: 'typesafe/jev-1.13', label: 'Jev 1.13', inputPerMTok: 0.042, outputPerMTok: 0 }],
    defaultModel: 'typesafe/jev-1.13',
    contextTokens: 32000,
  }

  it('colours the status badge', () => {
    expect(statusTone({ enabled: false, backend: '', model: '', ready: true }, false)).toBe('muted')
    expect(statusTone({ enabled: true, backend: '', model: '', ready: true }, true)).toBe('success')
    expect(
      statusTone({ enabled: true, backend: '', model: '', ready: false, backoffUntil: 1 }, true),
    ).toBe('warning')
    expect(statusTone({ enabled: true, backend: '', model: '', ready: false }, true)).toBe('danger')
  })

  it('tells listed from custom models', () => {
    expect(isCustomModel(backend, 'typesafe/jev-1.13')).toBe(false)
    expect(isCustomModel(backend, 'typesafe/jev-2')).toBe(true)
    expect(modelPrice(backend, 'typesafe/jev-1.13')).toBe(0.042)
    expect(modelPrice(backend, 'typesafe/jev-2')).toBeNull()
  })
})
