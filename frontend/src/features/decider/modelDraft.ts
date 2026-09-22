// Pure helpers behind the decision-model form: the draft a new model starts
// from (a backend or a ready-made preset), the draft an existing model is
// edited as, and the request body a draft saves as. React-free for vitest.
import type {
  DeciderBackend,
  DeciderModelInput,
  DeciderModelInstance,
  DeciderPreset,
} from '@/types/decider'

// API_KEY is the secret holding a model's own API key.
export const API_KEY = 'key'

// canBorrow reports whether a backend's models can use a provider account.
export function canBorrow(backend: DeciderBackend | undefined): boolean {
  return (backend?.providerKinds.length ?? 0) > 0
}

// draftFromBackend is a blank model of a backend with its defaults.
export function draftFromBackend(backend: DeciderBackend): DeciderModelInput {
  return {
    label: backend.label,
    backend: backend.id,
    enabled: true,
    model: backend.defaultModel,
    credentials: 'own',
    providerInstanceId: '',
    baseUrl: backend.defaultBaseUrl,
    timeoutMs: backend.defaultTimeoutMs,
    contextTokens: 0,
    config: {},
  }
}

// draftFromPreset is a model filled from one of a backend's presets.
export function draftFromPreset(backend: DeciderBackend, preset: DeciderPreset): DeciderModelInput {
  const base = draftFromBackend(backend)
  return {
    ...base,
    label: preset.label,
    model: preset.model || base.model,
    credentials: preset.credentials,
    baseUrl: preset.credentials === 'own' ? preset.baseUrl || base.baseUrl : '',
    timeoutMs: preset.timeoutMs || base.timeoutMs,
    contextTokens: preset.contextTokens ?? 0,
    config: { ...(preset.config ?? {}) },
  }
}

// draftFromModel is an existing model as an editable draft. Secrets are never
// part of it: the key stays stored unless the user types a new one.
export function draftFromModel(model: DeciderModelInstance): DeciderModelInput {
  return {
    id: model.id,
    label: model.label,
    backend: model.backend,
    enabled: model.enabled,
    model: model.model,
    credentials: model.credentials,
    providerInstanceId: model.providerInstanceId,
    baseUrl: model.baseUrl,
    timeoutMs: model.timeoutMs,
    contextTokens: model.contextTokens,
    config: { ...model.config },
  }
}

// toInput is the request body for a draft: the typed key is sent only when the
// user typed one (or asked to clear it); borrowed credentials send no base URL.
export function toInput(
  draft: DeciderModelInput,
  keyText: string,
  clearKey: boolean,
): DeciderModelInput {
  const out: DeciderModelInput = { ...draft, config: { ...draft.config } }
  delete out.secrets
  if (out.credentials === 'provider') {
    out.baseUrl = ''
  } else {
    out.providerInstanceId = ''
    if (keyText.trim() !== '') out.secrets = { [API_KEY]: keyText.trim() }
    else if (clearKey) out.secrets = { [API_KEY]: '' }
  }
  return out
}

// keyMissing reports a hosted backend's own-credential model saved without a key.
export function keyMissing(
  backend: DeciderBackend | undefined,
  draft: DeciderModelInput,
  keyText: string,
  keyStored: boolean,
  clearKey: boolean,
): boolean {
  if (!backend?.keyRequired || draft.credentials !== 'own') return false
  if (keyText.trim() !== '') return false
  return !keyStored || clearKey
}

// isLocalUrl reports a base URL on this machine or the local network, where a
// model runs for free.
export function isLocalUrl(url: string): boolean {
  try {
    const host = new URL(url).hostname.toLowerCase()
    return (
      host === 'localhost' ||
      host.endsWith('.localhost') ||
      host.endsWith('.local') ||
      /^127\./.test(host) ||
      /^10\./.test(host) ||
      /^192\.168\./.test(host) ||
      /^172\.(1[6-9]|2\d|3[01])\./.test(host) ||
      host === '[::1]'
    )
  } catch {
    return false
  }
}
