import { useEffect, useRef } from 'react'

// useOutsideClick fires `onOutside` when a mousedown lands outside the element
// the returned ref is attached to. Spread the ref on the root node of the popover
// / menu. Pass `active=false` to detach the listener while the menu is closed (so
// a closed menu costs nothing). The callback is read through a ref, so passing a
// fresh inline closure each render does NOT re-subscribe the listener.
export function useOutsideClick<T extends HTMLElement>(onOutside: () => void, active = true) {
  const ref = useRef<T>(null)
  const cb = useRef(onOutside)
  cb.current = onOutside
  useEffect(() => {
    if (!active) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) cb.current()
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [active])
  return ref
}
