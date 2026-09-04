// Rota's chip selection, persisted per browser. Mirrors useSessionChips: the
// UNTICKED chips are what gets stored, so a chip added to SESSION_CHIPS later
// starts selected instead of silently hiding its lanes.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { ALL_SESSION_CHIPS, normalizeChipsOff } from '@/features/sessions/sessionKindMeta'
import { ROTA_CHIPS_OFF_KEY, nextChipsOff, type ChipClickMode } from './rotaChips'

export interface RotaChipsState {
  chipsOff: string[]
  chipSet: ReadonlySet<string>
  clickChip: (key: string, mode: ChipClickMode) => void
  reset: () => void
}

function read(): string[] {
  try {
    return normalizeChipsOff(globalThis.localStorage?.getItem(ROTA_CHIPS_OFF_KEY))
  } catch {
    return []
  }
}

function write(chipsOff: string[]): void {
  try {
    globalThis.localStorage?.setItem(ROTA_CHIPS_OFF_KEY, JSON.stringify(chipsOff))
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}

export function useRotaChips(): RotaChipsState {
  const [chipsOff, setChipsOff] = useState<string[]>(read)
  useEffect(() => {
    write(chipsOff)
  }, [chipsOff])

  const chipSet = useMemo(
    () => new Set(ALL_SESSION_CHIPS.filter((k) => !chipsOff.includes(k))),
    [chipsOff],
  )

  const clickChip = useCallback((key: string, mode: ChipClickMode) => {
    setChipsOff((prev) => nextChipsOff(prev, key, mode))
  }, [])

  const reset = useCallback(() => setChipsOff([]), [])

  return { chipsOff, chipSet, clickChip, reset }
}
