import { useCallback, useEffect, useRef } from 'react'

// useDragScroll adds click-and-drag horizontal panning with the MOUSE to an
// overflow-x scroll container (e.g. the mobile bottom nav strip). Touch is left
// untouched — it already scrolls natively with momentum, so we only wire mouse
// events and would otherwise fight the browser's touch handling.
//
// Usage: spread the returned props on the scroll container:
//   const drag = useDragScroll<HTMLDivElement>()
//   <div ref={drag.ref} onMouseDown={drag.onMouseDown} onClickCapture={drag.onClickCapture} …>
//
// A drag past a small threshold suppresses the click that the browser fires at
// the end of the gesture (capture phase), so dragging over a button does not also
// activate it.
export function useDragScroll<T extends HTMLElement>() {
  const ref = useRef<T | null>(null)
  // Mutable gesture state kept in a ref so the window listeners always read the
  // latest values without re-subscribing.
  const st = useRef({ down: false, dragging: false, startX: 0, startScroll: 0 })

  const onMouseDown = useCallback((e: React.MouseEvent) => {
    if (e.button !== 0) return // primary button only
    const el = ref.current
    if (!el) return
    st.current = { down: true, dragging: false, startX: e.clientX, startScroll: el.scrollLeft }
  }, [])

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      const s = st.current
      const el = ref.current
      if (!s.down || !el) return
      const dx = e.clientX - s.startX
      // Small dead zone so a plain click is not mistaken for a drag.
      if (!s.dragging && Math.abs(dx) > 4) s.dragging = true
      if (s.dragging) {
        el.scrollLeft = s.startScroll - dx
        e.preventDefault() // avoid text/selection while panning
      }
    }
    const onUp = () => {
      // Leave `dragging` set so the trailing click can be suppressed in capture;
      // it is reset on the next mousedown or by onClickCapture.
      st.current.down = false
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [])

  const onClickCapture = useCallback((e: React.MouseEvent) => {
    if (st.current.dragging) {
      // The click closes a drag gesture — swallow it before it reaches a child.
      e.stopPropagation()
      e.preventDefault()
      st.current.dragging = false
    }
  }, [])

  return { ref, onMouseDown, onClickCapture }
}
