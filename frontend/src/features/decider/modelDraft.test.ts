import { describe, expect, it } from 'vitest'
import type { DeciderBackend, DeciderModelInstance } from '@/types/decider'
import {
  API_KEY,
  draftFromBackend,
  draftFromModel,
  draftFromPreset,
  isLocalUrl,
  keyMissing,
  toInput,
} from './modelDraft'

const systemOne: DeciderBackend = {
  id: 'systemone',
  label: 'System One API',
  defaultBaseUrl: 'https://api.typesafe.ai/v1',
  keyRequired: false,
  models: [{ id: 'jev-latest', label: 'Jev', inputPerMTok: 0.042, outputPerMTok: 0 }],
  defaultModel: 'jev-latest',
  contextTokens: 16000,
  defaultTimeoutMs: 3000,
  limits: { maxOptions: 52, maxLevels: 10 },
  decisionOnly: true,
  calibrated: true,
  presets: [
    {
      id: 'openjev-local',
      label: 'OpenJev · this machine',
      credentials: 'own',
      baseUrl: 'http://127.0.0.1:8080/v1',
      model: 'openjev-latest',
      contextTokens: 16000,
      timeoutMs: 10000,
    },
    {
      id: 'jev-openrouter-systemone',
      label: 'Jev · OpenRouter',
      credentials: 'own',
      baseUrl: 'https://openrouter.ai/api/v1',
      model: 'jev-1.13',
    },
  ],
}

describe('drafts', () => {
  it('starts a blank model from the backend defaults', () => {
    expect(draftFromBackend(systemOne)).toEqual({
      label: 'System One API',
      backend: 'systemone',
      enabled: true,
      model: 'jev-latest',
      credentials: 'own',
      baseUrl: 'https://api.typesafe.ai/v1',
      timeoutMs: 3000,
      contextTokens: 0,
      config: {},
    })
  })

  it('fills a model from a preset', () => {
    const local = draftFromPreset(systemOne, systemOne.presets![0])
    expect(local).toMatchObject({
      label: 'OpenJev · this machine',
      model: 'openjev-latest',
      baseUrl: 'http://127.0.0.1:8080/v1',
      timeoutMs: 10000,
      contextTokens: 16000,
    })
    const hosted = draftFromPreset(systemOne, systemOne.presets![1])
    expect(hosted).toMatchObject({
      credentials: 'own',
      baseUrl: 'https://openrouter.ai/api/v1',
      model: 'jev-1.13',
    })
  })

  it('edits a model without its secrets', () => {
    const m = {
      id: 'DM2',
      label: 'Local',
      backend: 'systemone',
      enabled: true,
      model: 'openjev-latest',
      credentials: 'own',
      baseUrl: 'http://127.0.0.1:8080/v1',
      timeoutMs: 5000,
      contextTokens: 0,
      config: { a: 'b' },
      secretsSet: { key: true },
      createdAt: '',
      updatedAt: '',
      status: { ready: true },
      usedBy: [],
    } satisfies DeciderModelInstance
    const draft = draftFromModel(m)
    expect(draft.id).toBe('DM2')
    expect('secrets' in draft).toBe(false)
    draft.config.a = 'changed'
    expect(m.config.a).toBe('b')
  })
})

describe('request body', () => {
  const draft = draftFromBackend(systemOne)

  it('sends a typed key, a clear request, or nothing', () => {
    expect(toInput(draft, ' sk-1 ', false).secrets).toEqual({ [API_KEY]: 'sk-1' })
    expect(toInput(draft, '', true).secrets).toEqual({ [API_KEY]: '' })
    expect(toInput(draft, '', false).secrets).toBeUndefined()
  })

  it('flags a hosted backend saved without a key', () => {
    const hosted = { ...systemOne, keyRequired: true }
    expect(keyMissing(hosted, '', false, false)).toBe(true)
    expect(keyMissing(hosted, 'k', false, false)).toBe(false)
    expect(keyMissing(hosted, '', true, false)).toBe(false)
    expect(keyMissing(hosted, '', true, true)).toBe(true)
    expect(keyMissing(systemOne, '', false, false)).toBe(false)
  })

  it('knows which URLs are local', () => {
    expect(isLocalUrl('http://127.0.0.1:8080/v1')).toBe(true)
    expect(isLocalUrl('http://192.168.1.4:11434/v1')).toBe(true)
    expect(isLocalUrl('http://gpu.local:8000/v1')).toBe(true)
    expect(isLocalUrl('https://api.typesafe.ai/v1')).toBe(false)
    expect(isLocalUrl('not a url')).toBe(false)
  })
})
