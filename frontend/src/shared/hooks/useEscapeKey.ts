import { useEffect } from 'react'

// useEscapeKey invokes `onClose` whenever Escape is pressed while enabled.
// Captures the ubiquitous close-on-Escape pattern used by the app's modals.
export function useEscapeKey(onClose: () => void, enabled = true): void {
  useEffect(() => {
    if (!enabled) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose, enabled])
}
