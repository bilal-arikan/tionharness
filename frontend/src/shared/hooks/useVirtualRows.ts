import { useCallback, useEffect, useState } from 'react'

// A minimal fixed-height row virtualizer. Deliberately hand-rolled rather than
// pulled from a library: the only consumer is the diff panel, whose rows are
// single monospace lines of a known, uniform height (the panel forces `pre` +
// horizontal scroll precisely so no row can wrap and break that assumption).
//
// Every row MUST be rendered at exactly `rowHeight` px, otherwise the spacer
// padding drifts from the real content and the scrollbar lies.

export interface VirtualWindow {
  /** First row index to render (inclusive). */
  start: number
  /** Last row index to render (exclusive). */
  end: number
  /** Spacer heights standing in for the rows outside the window. */
  padTop: number
  padBottom: number
  /** Re-measure after the row count or the container size changes out of band. */
  remeasure: () => void
}

// The scrolling container's ref is OWNED BY THE CALLER (and passed in) rather
// than created here: the component that renders the element is the one that
// should hold its ref, and handing a ref back out through a returned object
// defeats React's static tracking of it.
export function useVirtualRows(
  ref: React.RefObject<HTMLDivElement | null>,
  count: number,
  rowHeight: number,
  overscan = 24,
): VirtualWindow {
  const [range, setRange] = useState({ start: 0, end: Math.min(count, 120) })

  const measure = useCallback(() => {
    const el = ref.current
    // Before the first paint the container has no height yet; render a first
    // screenful rather than nothing, so the panel is never blank on mount.
    const viewport = el?.clientHeight || 0
    if (!viewport) {
      setRange({ start: 0, end: Math.min(count, 120) })
      return
    }
    const scrollTop = el ? el.scrollTop : 0
    const first = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan)
    const visible = Math.ceil(viewport / rowHeight) + overscan * 2
    setRange({ start: first, end: Math.min(count, first + visible) })
  }, [ref, count, rowHeight, overscan])

  useEffect(() => {
    const el = ref.current
    if (!el) return
    const onScroll = () => measure()
    el.addEventListener('scroll', onScroll, { passive: true })
    // observe() fires once immediately with the current size, which doubles as
    // the initial measurement — so the effect body itself sets no state.
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => {
      el.removeEventListener('scroll', onScroll)
      ro.disconnect()
    }
  }, [ref, measure])

  return {
    start: range.start,
    end: range.end,
    padTop: range.start * rowHeight,
    padBottom: Math.max(0, (count - range.end) * rowHeight),
    remeasure: measure,
  }
}
