// Rota toolbar preferences persisted per browser: the idle cutoff and the two
// time-axis toggles. Pure parse/serialize — no DOM — so a corrupt or partial
// stored value degrades to the defaults field by field instead of resetting all.
// Chip selection has its own key (rotaChips.ts); zoom is deliberately session
// scoped, it depends on the panel width the user had at the time.
import type { IdleCutoff } from './RotaToolbar'

export const ROTA_PREFS_KEY = 'tionharness.rotaPrefs'

export interface RotaPrefs {
  cutoff: IdleCutoff
  collapseGaps: boolean
  normalizeBars: boolean
}

export const DEFAULT_ROTA_PREFS: RotaPrefs = {
  cutoff: 21600,
  collapseGaps: true,
  normalizeBars: true,
}

const CUTOFFS: readonly IdleCutoff[] = [3600, 21600, 86400, 0]

export function parseRotaPrefs(raw: string | null | undefined): RotaPrefs {
  if (!raw) return DEFAULT_ROTA_PREFS
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return DEFAULT_ROTA_PREFS
  }
  if (!parsed || typeof parsed !== 'object') return DEFAULT_ROTA_PREFS
  const obj = parsed as Record<string, unknown>
  return {
    cutoff: (CUTOFFS as readonly unknown[]).includes(obj.cutoff)
      ? (obj.cutoff as IdleCutoff)
      : DEFAULT_ROTA_PREFS.cutoff,
    collapseGaps:
      typeof obj.collapseGaps === 'boolean' ? obj.collapseGaps : DEFAULT_ROTA_PREFS.collapseGaps,
    normalizeBars:
      typeof obj.normalizeBars === 'boolean' ? obj.normalizeBars : DEFAULT_ROTA_PREFS.normalizeBars,
  }
}

export function serializeRotaPrefs(prefs: RotaPrefs): string {
  return JSON.stringify(prefs)
}
