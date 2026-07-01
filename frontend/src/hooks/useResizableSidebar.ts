import { useEffect, useRef, useState } from 'react'

interface Options {
  // localStorage key the chosen width is persisted under (per sidebar kind).
  storageKey: string
  // Default width when nothing valid is stored yet.
  defaultWidth: number
  min?: number
  max?: number
}

// useResizableSidebar gives any list column a draggable, persisted width — the
// behaviour the sessions sidebar and skills list had baked in individually.
// Extracting it here lets every secondary sidebar be resizable the same way.
// Returns the current width plus a mousedown handler to wire onto a drag handle
// (pair it with the shared <ResizeHandle />).
export function useResizableSidebar({ storageKey, defaultWidth, min = 200, max = 640 }: Options) {
  const [width, setWidth] = useState(() => {
    const saved = Number(localStorage.getItem(storageKey))
    return saved >= min && saved <= max ? saved : defaultWidth
  })
  const drag = useRef<{ startX: number; startW: number } | null>(null)

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!drag.current) return
      setWidth(Math.min(max, Math.max(min, drag.current.startW + (e.clientX - drag.current.startX))))
    }
    const onUp = () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      localStorage.setItem(storageKey, String(width))
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [width, min, max, storageKey])

  const startDrag = (e: React.MouseEvent) => {
    e.preventDefault()
    drag.current = { startX: e.clientX, startW: width }
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
  }

  return { width, startDrag }
}
