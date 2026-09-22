import { describe, expect, it } from 'vitest'
import type {
  DeciderAuthority,
  DeciderAuthorityStats,
  DeciderBackend,
  DeciderConfig,
  DeciderModelInstance,
  DeciderView,
} from '@/types/decider'
import {
  MIN_COMPARISONS,
  adviceFor,
  agreement,
  authorityConfig,
  challengerAdvice,
  defaultModelId,
  groupAuthorities,
  isCustomModel,
  modelLabel,
  modelPrice,
  modelTone,
  pruneModelRefs,
  sameConfig,
  setupSteps,
  statusTone,
  withAuthority,
} from './deciderModel'

const authority: DeciderAuthority = {
  id: 'tool-risk',
  group: 'safety',
  pattern: 'gate',
  label: 'Shell command risk check',
  description: '',
  modes: ['off', 'shadow', 'on'],
  defaultMode: 'shadow',
  defaultThreshold: 0.8,
  thresholdHint: '',
  explicit: false,
}

const config: DeciderConfig = { enabled: true, defaultModel: '', authorities: {} }

const stats = (
  compared: number,
  agreed: number,
  extra: Partial<DeciderAuthorityStats> = {},
): DeciderAuthorityStats => ({
  authority: 'tool-risk',
  calls: compared,
  errors: 0,
  shadow: compared,
  compared,
  agreed,
  applied: 0,
  fallbacks: 0,
  p50Ms: 300,
  p95Ms: 600,
  costUsd: 0,
  challengerCalls: 0,
  challengerErrors: 0,
  challengerCompared: 0,
  challengerAgreed: 0,
  challengerP50Ms: 0,
  challengerCostUsd: 0,
  ...extra,
})

const model = (id: string, patch: Partial<DeciderModelInstance> = {}): DeciderModelInstance => ({
  id,
  label: `Model ${id}`,
  backend: 'systemone',
  enabled: true,
  model: 'openjev-latest',
  credentials: 'own',
  providerInstanceId: '',
  baseUrl: '',
  timeoutMs: 3000,
  contextTokens: 0,
  config: {},
  secretsSet: {},
  createdAt: '',
  updatedAt: '',
  status: { ready: true },
  usedBy: [],
  ...patch,
})

describe('authority config', () => {
  it('falls back to the authority defaults', () => {
    expect(authorityConfig(config, authority)).toEqual({
      mode: 'shadow',
      threshold: 0.8,
      model: '',
      fallback: '',
      challenger: '',
    })
  })

  it('patches one authority without touching the rest', () => {
    const next = withAuthority(config, authority, { mode: 'on', challenger: 'DM2' })
    expect(next.authorities['tool-risk']).toEqual({
      mode: 'on',
      threshold: 0.8,
      model: '',
      fallback: '',
      challenger: 'DM2',
    })
    expect(config.authorities).toEqual({})
  })

  it('compares configs regardless of key order and empty references', () => {
    const a: DeciderConfig = {
      ...config,
      authorities: {
        x: { mode: 'on', threshold: 0.7 },
        y: { mode: 'off', threshold: 0.8, fallback: '' },
      },
    }
    const b: DeciderConfig = {
      ...config,
      authorities: {
        y: { mode: 'off', threshold: 0.8 },
        x: { mode: 'on', threshold: 0.7, model: '' },
      },
    }
    expect(sameConfig(a, b)).toBe(true)
    expect(sameConfig(a, { ...b, defaultModel: 'DM1' })).toBe(false)
  })
})

describe('advice', () => {
  it('collects before it advises', () => {
    expect(adviceFor('shadow', stats(10, 10))).toEqual({
      kind: 'collect',
      done: 10,
      needed: MIN_COMPARISONS,
    })
  })

  it('recommends switching on at high agreement and keeping shadow at low', () => {
    expect(adviceFor('shadow', stats(100, 95))).toEqual({ kind: 'ready' })
    expect(adviceFor('shadow', stats(100, 70))).toEqual({ kind: 'keep' })
    expect(adviceFor('shadow', stats(100, 85))).toBeNull()
    expect(adviceFor('on', stats(100, 95))).toBeNull()
  })

  it('advises on a challenger only when there is one', () => {
    const s = stats(0, 0, { challengerCompared: 60, challengerAgreed: 58 })
    expect(challengerAdvice('DM2', s)).toEqual({ kind: 'ready' })
    expect(challengerAdvice('', s)).toBeNull()
    expect(challengerAdvice('DM2', stats(0, 0))).toEqual({
      kind: 'collect',
      done: 0,
      needed: MIN_COMPARISONS,
    })
  })

  it('reports agreement only after a comparison', () => {
    expect(agreement(stats(0, 0))).toBeNull()
    expect(agreement(stats(4, 3))).toBe(0.75)
  })
})

describe('models', () => {
  const backend: DeciderBackend = {
    id: 'openrouter',
    label: 'OpenRouter Decisions',
    providerKinds: ['openrouter'],
    defaultBaseUrl: 'https://openrouter.ai/api/v1',
    keyRequired: true,
    models: [
      { id: 'typesafe/jev-1.13', label: 'Jev 1.13', inputPerMTok: 0.042, outputPerMTok: 0 },
      { id: 'free-model', label: 'Free', inputPerMTok: 0, outputPerMTok: 0 },
    ],
    defaultModel: 'typesafe/jev-1.13',
    contextTokens: 32000,
    defaultTimeoutMs: 3000,
    limits: {},
    decisionOnly: true,
    calibrated: true,
  }

  it('prices listed models and recognises custom ids', () => {
    expect(modelPrice(backend, 'typesafe/jev-1.13')).toBe(0.042)
    expect(modelPrice(backend, 'free-model')).toBeNull()
    expect(isCustomModel(backend, 'typesafe/jev-2')).toBe(true)
    expect(isCustomModel(backend, 'typesafe/jev-1.13')).toBe(false)
  })

  it('resolves the default model and labels', () => {
    const models = [model('DM1', { enabled: false }), model('DM2')]
    expect(defaultModelId(config, models)).toBe('DM2')
    expect(defaultModelId({ ...config, defaultModel: 'DM1' }, models)).toBe('DM1')
    expect(modelLabel(models, 'DM2')).toBe('Model DM2')
    expect(modelLabel(models, 'gone')).toBe('gone')
  })

  it('colours a model by its health', () => {
    expect(modelTone(model('a'))).toBe('success')
    expect(modelTone(model('a', { enabled: false }))).toBe('muted')
    expect(modelTone(model('a', { status: { ready: false, backoffUntil: 1 } }))).toBe('warning')
    expect(modelTone(model('a', { status: { ready: false, problem: 'x' } }))).toBe('danger')
  })

  it('prunes references to deleted models from a draft', () => {
    const draft: DeciderConfig = {
      enabled: true,
      defaultModel: 'gone',
      authorities: {
        x: { mode: 'on', threshold: 0.7, model: 'DM1', fallback: 'gone', challenger: 'gone' },
      },
    }
    expect(pruneModelRefs(draft, ['DM1'])).toEqual({
      enabled: true,
      defaultModel: '',
      authorities: {
        x: { mode: 'on', threshold: 0.7, model: 'DM1', fallback: '', challenger: '' },
      },
    })
  })
})

describe('status and grouping', () => {
  it('colours the header badge', () => {
    expect(statusTone({ enabled: false, ready: false }, false)).toBe('muted')
    expect(statusTone({ enabled: true, ready: true }, true)).toBe('success')
    expect(statusTone({ enabled: true, ready: false, backoffUntil: 5 }, true)).toBe('warning')
    expect(statusTone({ enabled: true, ready: false }, true)).toBe('danger')
  })

  it('groups authorities in the server order and drops empty groups', () => {
    const flow = { ...authority, id: 'flow-judge', group: 'flows' }
    const odd = { ...authority, id: 'custom', group: 'lab' }
    const groups = groupAuthorities([flow, authority, odd], ['safety', 'coordination', 'flows'])
    expect(groups.map((g) => g.group)).toEqual(['safety', 'flows', 'lab'])
    expect(groups[1].items).toEqual([flow])
  })
})

describe('setup steps', () => {
  const view = (patch: Partial<DeciderView> = {}): DeciderView => ({
    config: { enabled: false, defaultModel: '', authorities: {} },
    status: { enabled: false, ready: false },
    backends: [
      {
        id: 'openrouter',
        label: 'OpenRouter Decisions',
        providerKinds: ['openrouter'],
        defaultBaseUrl: '',
        keyRequired: true,
        models: [],
        defaultModel: '',
        contextTokens: 0,
        defaultTimeoutMs: 3000,
        limits: {},
        decisionOnly: true,
        calibrated: true,
      },
    ],
    groups: [],
    authorities: [],
    models: [],
    providerCandidates: { openrouter: [] },
    stats: [],
    recent: [],
    statsDays: 7,
    ...patch,
  })

  it('starts with nothing done', () => {
    expect(setupSteps(view()).map((s) => s.done)).toEqual([false, false, false])
  })

  it('counts a borrowable account, or an own key, as the credential', () => {
    const account = { id: 'PRV1', kind: 'openrouter', label: '', enabled: true, available: true }
    expect(setupSteps(view({ providerCandidates: { openrouter: [account] } }))[0].done).toBe(true)
    const keyless = model('DM1', { backend: 'openrouter', status: { ready: false } })
    expect(setupSteps(view({ models: [keyless] }))[0].done).toBe(false)
    const keyed = model('DM1', {
      backend: 'openrouter',
      secretsSet: { key: true },
      status: { ready: false },
    })
    expect(setupSteps(view({ models: [keyed] }))[0].done).toBe(true)
  })

  it('needs a ready provider and the master switch', () => {
    const ready = view({
      models: [model('DM1')],
      config: { enabled: true, defaultModel: '', authorities: {} },
    })
    expect(setupSteps(ready).map((s) => s.done)).toEqual([true, true, true])
  })
})
