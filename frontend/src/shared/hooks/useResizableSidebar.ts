import { useEffect, useRef, useState } from 'react'
import { useViewportTier } from './useViewport'
import { capListColumnWidth } from '@/shared/lib/viewport'

interface Options {
  // localStorage key the chosen width is persisted under (per sidebar kind).
  storageKey: string
  // Default width when nothing valid is stored yet.
  defaultWidth: number
  min?: number
  max?: number
  // Invert the drag direction: for a RIGHT-hand panel whose handle sits on its
  // LEFT edge, dragging left (decreasing clientX) must GROW the panel.
  invert?: boolean
  // Apply the per-tier width cap (shared/lib/viewport.ts). Default true; a panel
  // that is never docked on the capped tiers (it becomes a drawer there) passes
  // false so its drawer keeps the user's full width.
  capToTier?: boolean
}

function readStoredWidth(key: string): number {
  try {
    return Number(globalThis.localStorage.getItem(key))
  } catch {
    return Number.NaN
  }
}

function writeStoredWidth(key: string, width: number) {
  try {
    globalThis.localStorage.setItem(key, String(width))
  } catch {
    // Persistence is best-effort when storage is blocked by browser policy.
  }
}

// useResizableSidebar gives any list column a draggable, persisted width — the
// behaviour the sessions sidebar and skills list had baked in individually.
// Extracting it here lets every secondary sidebar be resizable the same way.
// Returns the current width plus a mousedown handler to wire onto a drag handle
// (pair it with the shared <ResizeHandle />).
//
// The returned width is the persisted value CAPPED for the current viewport tier
// (shared/lib/viewport.ts): on the square tier a list column may not exceed the
// tier maximum, so three columns still fit; the stored width is untouched and
// comes back as soon as the window is wide again. While a drag is in flight
// <body data-th-resizing> is set so `.th-col` width transitions pause and the
// column tracks the pointer 1:1.
export function useResizableSidebar({
  storageKey,
  defaultWidth,
  min = 200,
  max = 640,
  invert = false,
  capToTier = true,
}: Options) {
  const [width, setWidth] = useState(() => {
    const saved = readStoredWidth(storageKey)
    return saved >= min && saved <= max ? saved : defaultWidth
  })
  const drag = useRef<{ startX: number; startW: number } | null>(null)
  const tier = useViewportTier()
  const effectiveWidth = capToTier ? capListColumnWidth(width, tier) : width

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!drag.current) return
      const delta = e.clientX - drag.current.startX
      const next = drag.current.startW + (invert ? -delta : delta)
      setWidth(Math.min(max, Math.max(min, next)))
    }
    const onUp = () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      delete document.body.dataset.thResizing
      writeStoredWidth(storageKey, width)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [width, min, max, storageKey, invert])

  const startDrag = (e: React.MouseEvent) => {
    e.preventDefault()
    drag.current = { startX: e.clientX, startW: effectiveWidth }
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
    document.body.dataset.thResizing = '1'
  }

  return { width: effectiveWidth, startDrag }
}
