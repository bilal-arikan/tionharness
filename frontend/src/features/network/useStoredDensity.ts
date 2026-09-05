import { useCallback, useEffect, useState } from 'react'

// Physics density slider value, persisted per browser under `key` so leaving a
// graph screen and coming back keeps the packing the user dialled in. Out-of-
// range or unreadable values fall back to 1.
export const DENSITY_MIN = 0.4
export const DENSITY_MAX = 2
export const DENSITY_DEFAULT = 1

export function parseDensity(raw: string | null | undefined): number {
  if (!raw) return DENSITY_DEFAULT
  const value = Number(raw)
  if (!Number.isFinite(value) || value < DENSITY_MIN || value > DENSITY_MAX) return DENSITY_DEFAULT
  return value
}

export function useStoredDensity(key: string): [number, (value: number) => void] {
  const [density, setDensityState] = useState(() => {
    try {
      return parseDensity(globalThis.localStorage?.getItem(key))
    } catch {
      return DENSITY_DEFAULT
    }
  })
  useEffect(() => {
    try {
      globalThis.localStorage?.setItem(key, String(density))
    } catch {
      // Persistence is best-effort when storage is blocked by browser policy.
    }
  }, [key, density])
  const setDensity = useCallback(
    (value: number) => setDensityState(parseDensity(String(value))),
    [],
  )
  return [density, setDensity]
}
