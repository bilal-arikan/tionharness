// Pure helpers behind the decision-layer settings page. Kept free of React and
// of the api client so they run under vitest's node environment.
import type {
  DeciderAuthority,
  DeciderAuthorityConfig,
  DeciderAuthorityStats,
  DeciderBackend,
  DeciderConfig,
  DeciderMode,
  DeciderModelInstance,
  DeciderStatus,
  DeciderView,
} from '@/types/decider'

// An agreement rate is only trusted after this many comparisons.
export const MIN_COMPARISONS = 50
// Agreement at or above this suggests switching on (or letting a challenger
// take over) …
export const SWITCH_ON_AGREEMENT = 0.9
// … and below this that it should stay where it is.
export const KEEP_SHADOW_AGREEMENT = 0.8

export const MODES: DeciderMode[] = ['off', 'shadow', 'on']

// authorityConfig returns an authority's settings, falling back to its defaults.
export function authorityConfig(
  config: DeciderConfig,
  authority: DeciderAuthority,
): DeciderAuthorityConfig {
  const ac = config.authorities[authority.id]
  return {
    mode: ac?.mode || authority.defaultMode,
    threshold: ac?.threshold || authority.defaultThreshold,
    model: ac?.model ?? '',
    fallback: ac?.fallback ?? '',
    challenger: ac?.challenger ?? '',
  }
}

// withAuthority returns a copy of config with one authority's settings patched.
export function withAuthority(
  config: DeciderConfig,
  authority: DeciderAuthority,
  patch: Partial<DeciderAuthorityConfig>,
): DeciderConfig {
  return {
    ...config,
    authorities: {
      ...config.authorities,
      [authority.id]: { ...authorityConfig(config, authority), ...patch },
    },
  }
}

// canonical renders a config with sorted authority keys and empty optional
// references normalised, so two configs that differ only in map order or in
// "" vs a missing field compare equal.
function canonical(config: DeciderConfig): string {
  const authorities = Object.keys(config.authorities)
    .sort()
    .map((k) => {
      const a = config.authorities[k]
      return [k, a.mode, a.threshold, a.model || '', a.fallback || '', a.challenger || '']
    })
  return JSON.stringify([config.enabled, config.defaultModel || '', authorities])
}

export function sameConfig(a: DeciderConfig, b: DeciderConfig): boolean {
  return canonical(a) === canonical(b)
}

// agreement is the share of shadow comparisons in which the model agreed with
// the authority's current logic, or null before any comparison exists.
export function agreement(stats?: DeciderAuthorityStats): number | null {
  if (!stats || stats.compared === 0) return null
  return stats.agreed / stats.compared
}

// challengerAgreement is the share of challenger comparisons in which the
// challenger reached the same verdict as the answering model.
export function challengerAgreement(stats?: DeciderAuthorityStats): number | null {
  if (!stats || stats.challengerCompared === 0) return null
  return stats.challengerAgreed / stats.challengerCompared
}

export type Advice =
  { kind: 'ready' } | { kind: 'keep' } | { kind: 'collect'; done: number; needed: number }

// adviceFrom turns a comparison count and rate into advice.
function adviceFrom(done: number, rate: number | null): Advice | null {
  if (done < MIN_COMPARISONS) return { kind: 'collect', done, needed: MIN_COMPARISONS }
  if ((rate ?? 0) >= SWITCH_ON_AGREEMENT) return { kind: 'ready' }
  if ((rate ?? 0) < KEEP_SHADOW_AGREEMENT) return { kind: 'keep' }
  return null
}

// adviceFor tells an authority in shadow mode whether its numbers support
// switching it on. No advice for other modes.
export function adviceFor(mode: DeciderMode, stats?: DeciderAuthorityStats): Advice | null {
  if (mode !== 'shadow') return null
  return adviceFrom(stats?.compared ?? 0, agreement(stats))
}

// challengerAdvice tells whether a challenger agrees often enough to take over
// as the authority's model. No advice without a challenger.
export function challengerAdvice(
  challenger: string | undefined,
  stats?: DeciderAuthorityStats,
): Advice | null {
  if (!challenger) return null
  return adviceFrom(stats?.challengerCompared ?? 0, challengerAgreement(stats))
}

export type StatusTone = 'success' | 'warning' | 'danger' | 'muted'

// statusTone colours the header badge: off is neutral, ready green, paused
// amber, anything else red.
export function statusTone(status: DeciderStatus, enabled: boolean): StatusTone {
  if (!enabled) return 'muted'
  if (status.ready) return 'success'
  if (status.backoffUntil) return 'warning'
  return 'danger'
}

// modelTone colours a model card's status badge.
export function modelTone(model: DeciderModelInstance): StatusTone {
  if (!model.enabled) return 'muted'
  if (model.status.ready) return 'success'
  if (model.status.backoffUntil) return 'warning'
  return 'danger'
}

// isCustomModel reports whether model is not one of the backend's suggestions.
export function isCustomModel(backend: DeciderBackend | undefined, model: string): boolean {
  if (!backend || !model) return false
  return !backend.models.some((m) => m.id === model)
}

// modelPrice returns the listed input price of a model, if known and non-zero.
export function modelPrice(backend: DeciderBackend | undefined, model: string): number | null {
  const m = backend?.models.find((x) => x.id === model)
  return m && m.inputPerMTok > 0 ? m.inputPerMTok : null
}

// statsFor finds an authority's stats row.
export function statsFor(
  stats: DeciderAuthorityStats[],
  authorityId: string,
): DeciderAuthorityStats | undefined {
  return stats.find((s) => s.authority === authorityId)
}

// groupAuthorities splits authorities into the settings groups, in the
// server's group order, dropping empty groups.
export function groupAuthorities(
  authorities: DeciderAuthority[],
  groups: string[],
): Array<{ group: string; items: DeciderAuthority[] }> {
  const known = new Set(groups)
  const order = [...groups, ...authorities.map((a) => a.group).filter((g) => !known.has(g))]
  return [...new Set(order)]
    .map((group) => ({ group, items: authorities.filter((a) => a.group === group) }))
    .filter((g) => g.items.length > 0)
}

// defaultModelId is the model that answers when an authority names none: the
// configured default, else the first enabled model.
export function defaultModelId(config: DeciderConfig, models: DeciderModelInstance[]): string {
  if (config.defaultModel) return config.defaultModel
  return models.find((m) => m.enabled)?.id ?? ''
}

// modelLabel names a model for selects and the activity list.
export function modelLabel(models: DeciderModelInstance[], id: string | undefined): string {
  if (!id) return ''
  return models.find((m) => m.id === id)?.label || id
}

// pruneModelRefs clears every reference to a model that no longer exists (the
// default model and each authority's model, fallback and challenger), so an
// unsaved draft stays valid after a model is deleted.
export function pruneModelRefs(config: DeciderConfig, modelIds: string[]): DeciderConfig {
  const known = new Set(modelIds)
  const keep = (id: string | undefined) => (id && known.has(id) ? id : '')
  const authorities: DeciderConfig['authorities'] = {}
  for (const [id, ac] of Object.entries(config.authorities)) {
    authorities[id] = {
      ...ac,
      model: keep(ac.model),
      fallback: keep(ac.fallback),
      challenger: keep(ac.challenger),
    }
  }
  return { ...config, defaultModel: keep(config.defaultModel), authorities }
}

// SetupStepId names one step of the setup guide.
export type SetupStepId = 'account' | 'provider' | 'enable'

export interface SetupStep {
  id: SetupStepId
  done: boolean
}

// setupSteps is the three-step setup the guide walks through: a credential (a
// provider account a decision provider can borrow, or a decision provider with
// its own key — a local server needs none), a decision provider that is ready,
// and the master switch on.
export function setupSteps(view: DeciderView): SetupStep[] {
  const keyRequired = (m: DeciderModelInstance) =>
    view.backends.find((b) => b.id === m.backend)?.keyRequired ?? false
  const ready = view.models.some((m) => m.enabled && m.status.ready)
  const account =
    ready ||
    Object.values(view.providerCandidates).some((list) => list.length > 0) ||
    view.models.some((m) => m.credentials === 'own' && (!keyRequired(m) || m.secretsSet.key))
  return [
    { id: 'account', done: account },
    { id: 'provider', done: ready },
    { id: 'enable', done: view.config.enabled },
  ]
}
