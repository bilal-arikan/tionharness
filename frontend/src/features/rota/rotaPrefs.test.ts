import { describe, expect, it } from 'vitest'
import { DEFAULT_ROTA_PREFS, parseRotaPrefs, serializeRotaPrefs } from './rotaPrefs'

describe('rotaPrefs', () => {
  it('round-trips a full preference set', () => {
    const prefs = { cutoff: 86400 as const, collapseGaps: false, normalizeBars: false }
    expect(parseRotaPrefs(serializeRotaPrefs(prefs))).toEqual(prefs)
  })

  it('falls back to defaults for missing, corrupt or partial input', () => {
    expect(parseRotaPrefs(null)).toEqual(DEFAULT_ROTA_PREFS)
    expect(parseRotaPrefs('')).toEqual(DEFAULT_ROTA_PREFS)
    expect(parseRotaPrefs('{not json')).toEqual(DEFAULT_ROTA_PREFS)
    expect(parseRotaPrefs('42')).toEqual(DEFAULT_ROTA_PREFS)
    // Field by field: a bad cutoff does not reset the toggles.
    expect(parseRotaPrefs(JSON.stringify({ cutoff: 999, collapseGaps: false }))).toEqual({
      ...DEFAULT_ROTA_PREFS,
      collapseGaps: false,
    })
    expect(parseRotaPrefs(JSON.stringify({ cutoff: 0, normalizeBars: 'yes' }))).toEqual({
      ...DEFAULT_ROTA_PREFS,
      cutoff: 0,
    })
  })
})
