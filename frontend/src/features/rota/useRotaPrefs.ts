// Stateful wrapper over rotaPrefs: reads once on mount, writes on every change.
// Mirrors useRotaChips so the toolbar's three controls survive a reopen of the
// screen and a browser restart.
import { useCallback, useEffect, useState } from 'react'
import { parseRotaPrefs, ROTA_PREFS_KEY, serializeRotaPrefs, type RotaPrefs } from './rotaPrefs'

function read(): RotaPrefs {
  try {
    return parseRotaPrefs(globalThis.localStorage?.getItem(ROTA_PREFS_KEY))
  } catch {
    return parseRotaPrefs(null)
  }
}

function write(prefs: RotaPrefs): void {
  try {
    globalThis.localStorage?.setItem(ROTA_PREFS_KEY, serializeRotaPrefs(prefs))
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}

export interface RotaPrefsState {
  prefs: RotaPrefs
  setCutoff: (cutoff: RotaPrefs['cutoff']) => void
  setCollapseGaps: (value: boolean) => void
  setNormalizeBars: (value: boolean) => void
}

export function useRotaPrefs(): RotaPrefsState {
  const [prefs, setPrefs] = useState<RotaPrefs>(read)
  useEffect(() => {
    write(prefs)
  }, [prefs])

  const setCutoff = useCallback(
    (cutoff: RotaPrefs['cutoff']) => setPrefs((p) => ({ ...p, cutoff })),
    [],
  )
  const setCollapseGaps = useCallback(
    (collapseGaps: boolean) => setPrefs((p) => ({ ...p, collapseGaps })),
    [],
  )
  const setNormalizeBars = useCallback(
    (normalizeBars: boolean) => setPrefs((p) => ({ ...p, normalizeBars })),
    [],
  )

  return { prefs, setCutoff, setCollapseGaps, setNormalizeBars }
}
