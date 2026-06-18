import { useCallback, useEffect, useRef, useState } from 'react'

interface Options {
  /** Persisted under this localStorage key (survives reloads). */
  storageKey: string
  /** Initial width when nothing is stored yet. */
  defaultWidth: number
  min?: number
  max?: number
  /** Which edge the drag handle sits on. A right-docked panel grows leftward. */
  side?: 'left' | 'right'
}

// useResizableWidth drives a drag-to-resize panel: it returns the current width,
// a handle onPointerDown to start dragging, and a flag while dragging. The width
// is clamped to [min, max] and persisted to localStorage so it survives reloads.
export function useResizableWidth({
  storageKey,
  defaultWidth,
  min = 280,
  max = 760,
  side = 'left',
}: Options) {
  const clamp = useCallback((w: number) => Math.min(max, Math.max(min, w)), [min, max])

  const [width, setWidth] = useState<number>(() => {
    const stored = Number(localStorage.getItem(storageKey))
    return stored ? clamp(stored) : clamp(defaultWidth)
  })
  const [dragging, setDragging] = useState(false)
  const frame = useRef<number | null>(null)

  // While dragging, translate the pointer x into a panel width. A left-edge
  // handle on a right-docked panel means width = viewportRight - pointerX.
  useEffect(() => {
    if (!dragging) return
    const onMove = (e: PointerEvent) => {
      if (frame.current != null) return
      frame.current = requestAnimationFrame(() => {
        frame.current = null
        const w = side === 'left' ? window.innerWidth - e.clientX : e.clientX
        setWidth(clamp(w))
      })
    }
    const onUp = () => setDragging(false)
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    // Avoid text selection / wrong cursor mid-drag.
    const prevCursor = document.body.style.cursor
    const prevSelect = document.body.style.userSelect
    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      document.body.style.cursor = prevCursor
      document.body.style.userSelect = prevSelect
      if (frame.current != null) cancelAnimationFrame(frame.current)
      frame.current = null
    }
  }, [dragging, side, clamp])

  // Persist the final width when a drag ends.
  useEffect(() => {
    if (!dragging) localStorage.setItem(storageKey, String(width))
  }, [dragging, width, storageKey])

  const onHandleDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault()
    setDragging(true)
  }, [])

  return { width, dragging, onHandleDown }
}
