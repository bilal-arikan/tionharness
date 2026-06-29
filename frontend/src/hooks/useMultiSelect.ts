import { useCallback, useEffect, useRef, useState } from 'react'

// Modifier-key shape extracted from a mouse/keyboard event. We only read the
// three flags we care about so callers can pass a real React.MouseEvent or a
// hand-built object (tests).
export interface ClickModifiers {
  ctrlKey: boolean
  metaKey: boolean
  shiftKey: boolean
}

export interface MultiSelect {
  /** The set of currently-selected ids (stable reference per change). */
  selected: ReadonlySet<string>
  /** Convenience: number of selected ids. */
  count: number
  isSelected: (id: string) => boolean
  /**
   * Interpret a list-item click with its modifier keys.
   *   - Ctrl/Cmd+Click  → toggle the id in/out of the selection
   *   - Shift+Click      → select the contiguous range from the anchor to id,
   *                        using `ordered` (the currently-rendered, visible row
   *                        order, so the range can cross group boundaries)
   *   - plain Click      → clear the selection and set the anchor
   * Returns true when the click was a SELECTION gesture (a modifier was held);
   * the caller should then suppress its normal navigation (open/activate).
   */
  handleClick: (e: ClickModifiers, id: string, ordered: string[]) => boolean
  /** Toggle a single id (used by explicit checkboxes / select-all UIs). */
  toggle: (id: string) => void
  clear: () => void
  /** Replace the selection with every id in `ids` (Ctrl+A / "select all"). */
  selectAll: (ids: string[]) => void
  /** Replace the selection with an exact set (e.g. after a bulk action prunes ids). */
  replace: (ids: string[]) => void
}

// useMultiSelect is the list-agnostic core of Ctrl/Cmd+Click multi-selection.
// It owns only the selection set + a shift-range anchor; the rendering list maps
// over `isSelected` for styling and calls `handleClick` from each row's onClick.
// While at least one item is selected, Escape clears the selection globally.
export function useMultiSelect(): MultiSelect {
  const [selected, setSelected] = useState<Set<string>>(() => new Set())
  // The shift-range anchor: the last id clicked without shift. Held in a ref so
  // updating it never triggers a re-render.
  const anchor = useRef<string | null>(null)

  const isSelected = useCallback((id: string) => selected.has(id), [selected])

  const handleClick = useCallback((e: ClickModifiers, id: string, ordered: string[]): boolean => {
    const additive = e.ctrlKey || e.metaKey
    const range = e.shiftKey

    if (range && anchor.current && ordered.includes(anchor.current)) {
      const a = ordered.indexOf(anchor.current)
      const b = ordered.indexOf(id)
      const lo = Math.min(a, b)
      const hi = Math.max(a, b)
      const slice = ordered.slice(lo, hi + 1)
      setSelected((prev) => {
        // Shift+Click replaces the selection with the range; Ctrl+Shift+Click
        // merges the range into the existing selection.
        const next = new Set(additive ? prev : [])
        for (const x of slice) next.add(x)
        return next
      })
      // Anchor intentionally stays put so the user can re-extend the same range.
      return true
    }

    if (additive) {
      setSelected((prev) => {
        const next = new Set(prev)
        if (next.has(id)) next.delete(id)
        else next.add(id)
        return next
      })
      anchor.current = id
      return true
    }

    // Plain click: this is navigation, not selection. Reset any selection and
    // remember the row as the anchor for a subsequent Shift+Click.
    anchor.current = id
    setSelected((prev) => (prev.size ? new Set() : prev))
    return false
  }, [])

  const toggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
    anchor.current = id
  }, [])

  const clear = useCallback(() => {
    anchor.current = null
    setSelected((prev) => (prev.size ? new Set() : prev))
  }, [])

  const selectAll = useCallback((ids: string[]) => {
    setSelected(new Set(ids))
  }, [])

  const replace = useCallback((ids: string[]) => {
    setSelected(new Set(ids))
  }, [])

  // Escape clears the selection — bound only while something is selected so an
  // empty list costs nothing and we never swallow Escape elsewhere.
  useEffect(() => {
    if (selected.size === 0) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') clear()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [selected.size, clear])

  return { selected, count: selected.size, isSelected, handleClick, toggle, clear, selectAll, replace }
}
