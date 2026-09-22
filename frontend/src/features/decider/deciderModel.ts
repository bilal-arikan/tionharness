// Pure helpers behind the decision-model settings page. Kept free of React and
// of the api client so they run under vitest's node environment.
import type {
  DeciderBackend,
  DeciderConfig,
  DeciderMode,
  DeciderSite,
  DeciderSiteConfig,
  DeciderSiteStats,
  DeciderStatus,
} from '@/types/decider'

// A site's agreement is only trusted after this many shadow comparisons.
export const MIN_COMPARISONS = 50
// Agreement at or above this suggests the site can be switched on …
export const SWITCH_ON_AGREEMENT = 0.9
// … and below this that it should stay in shadow.
export const KEEP_SHADOW_AGREEMENT = 0.8

export const MODES: DeciderMode[] = ['off', 'shadow', 'on']

// siteConfig returns a site's settings, falling back to the site's defaults.
export function siteConfig(config: DeciderConfig, site: DeciderSite): DeciderSiteConfig {
  const sc = config.sites[site.id]
  return {
    mode: sc?.mode || site.defaultMode,
    threshold: sc?.threshold || site.defaultThreshold,
  }
}

// withSite returns a copy of config with one site's settings patched.
export function withSite(
  config: DeciderConfig,
  site: DeciderSite,
  patch: Partial<DeciderSiteConfig>,
): DeciderConfig {
  return {
    ...config,
    sites: { ...config.sites, [site.id]: { ...siteConfig(config, site), ...patch } },
  }
}

// canonical renders a config with sorted site keys so two configs that differ
// only in map order compare equal.
function canonical(config: DeciderConfig): string {
  const sites = Object.keys(config.sites)
    .sort()
    .map((k) => [k, config.sites[k].mode, config.sites[k].threshold])
  return JSON.stringify([
    config.enabled,
    config.backend,
    config.providerInstanceId,
    config.model,
    config.timeoutMs,
    sites,
  ])
}

export function sameConfig(a: DeciderConfig, b: DeciderConfig): boolean {
  return canonical(a) === canonical(b)
}

// agreement is the share of shadow comparisons in which the model agreed with
// the site's current logic, or null before any comparison exists.
export function agreement(stats?: DeciderSiteStats): number | null {
  if (!stats || stats.compared === 0) return null
  return stats.agreed / stats.compared
}

export type Advice =
  { kind: 'ready' } | { kind: 'keep' } | { kind: 'collect'; done: number; needed: number }

// adviceFor tells a site in shadow mode whether its numbers support switching
// it on. No advice for other modes or for sites without comparisons.
export function adviceFor(mode: DeciderMode, stats?: DeciderSiteStats): Advice | null {
  if (mode !== 'shadow') return null
  const done = stats?.compared ?? 0
  if (done < MIN_COMPARISONS) return { kind: 'collect', done, needed: MIN_COMPARISONS }
  const rate = agreement(stats) ?? 0
  if (rate >= SWITCH_ON_AGREEMENT) return { kind: 'ready' }
  if (rate < KEEP_SHADOW_AGREEMENT) return { kind: 'keep' }
  return null
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

// isCustomModel reports whether model is not one of the backend's listed models.
export function isCustomModel(backend: DeciderBackend | undefined, model: string): boolean {
  if (!backend || !model) return false
  return !backend.models.some((m) => m.id === model)
}

// modelPrice returns the listed input price of a model, if known.
export function modelPrice(backend: DeciderBackend | undefined, model: string): number | null {
  const m = backend?.models.find((x) => x.id === model)
  return m ? m.inputPerMTok : null
}

// statsFor finds a site's stats row.
export function statsFor(stats: DeciderSiteStats[], siteId: string): DeciderSiteStats | undefined {
  return stats.find((s) => s.site === siteId)
}
