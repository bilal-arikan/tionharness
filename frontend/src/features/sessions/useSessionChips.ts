import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ALL_SESSION_CHIPS,
  SESSION_CHIPS_OFF_KEY,
  normalizeChipsOff,
  nextChipsOff,
  type ChipClickMode,
} from './sessionKindMeta'

export interface SessionChipsState {
  // The UNTICKED chips (see SESSION_CHIPS_OFF_KEY for why the off set is what
  // gets persisted).
  chipsOff: string[]
  // The selected chips, for the row predicate.
  chipSet: ReadonlySet<string>
  // The `chips` query value for /api/sessions: the selection, comma-joined. The
  // server pages the filtered set, so this is what keeps "Daha fazla yükle"
  // paging through the rows the sidebar actually renders instead of through a
  // mixed list where the visible kind is a handful of rows per page.
  chipsParam: string
  clickChip: (key: string, mode: ChipClickMode) => void
}

type SessionChipStorage = Pick<Storage, 'getItem' | 'setItem'>

export function browserSessionChipStorage(
  scope: { readonly localStorage: Storage } = globalThis,
): SessionChipStorage | null {
  try {
    return scope.localStorage
  } catch {
    return null
  }
}

export function readSessionChipsOff(storage: SessionChipStorage | null): string[] {
  if (!storage) return []
  try {
    return normalizeChipsOff(storage.getItem(SESSION_CHIPS_OFF_KEY))
  } catch {
    return []
  }
}

export function writeSessionChipsOff(storage: SessionChipStorage | null, chipsOff: string[]): void {
  if (!storage) return
  try {
    storage.setItem(SESSION_CHIPS_OFF_KEY, JSON.stringify(chipsOff))
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}

// The chip selection lives above the sidebar because it is now a SERVER filter
// as well as a view filter: the session list request carries it, so the
// controller that fetches the list needs the same state the chips render from.
export function useSessionChips(): SessionChipsState {
  const [chipsOff, setChipsOff] = useState<string[]>(() =>
    readSessionChipsOff(browserSessionChipStorage()),
  )
  useEffect(() => {
    writeSessionChipsOff(browserSessionChipStorage(), chipsOff)
  }, [chipsOff])

  const selected = useMemo(() => ALL_SESSION_CHIPS.filter((k) => !chipsOff.includes(k)), [chipsOff])
  const chipSet = useMemo(() => new Set(selected), [selected])
  const chipsParam = useMemo(() => selected.join(','), [selected])

  const clickChip = useCallback((key: string, mode: ChipClickMode) => {
    setChipsOff((prev) => nextChipsOff(prev, key, mode))
  }, [])

  return { chipsOff, chipSet, chipsParam, clickChip }
}
