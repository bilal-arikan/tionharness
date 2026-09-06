import { useEffect, useState } from 'react'
import { TIER_MIN_WIDTH } from '@/shared/lib/viewport'

// useMediaQuery tracks a CSS media query and re-renders when it flips. SSR-safe
// (returns false when window is unavailable). Used to branch layout between the
// desktop multi-column shell and the mobile single-column shell.
function useMediaQuery(query: string): boolean {
  const [match, setMatch] = useState(() =>
    typeof window !== 'undefined' ? window.matchMedia(query).matches : false,
  )
  useEffect(() => {
    const m = window.matchMedia(query)
    const onChange = () => setMatch(m.matches)
    onChange()
    m.addEventListener('change', onChange)
    return () => m.removeEventListener('change', onChange)
  }, [query])
  return match
}

// useIsMobile reports whether the viewport is below Tailwind's `md` breakpoint
// (768px), i.e. the `narrow` tier: a portrait phone. The bound comes from the
// shared tier table so the `md:` utilities, this hook and useViewport agree.
export function useIsMobile(): boolean {
  return useMediaQuery(`(max-width: ${TIER_MIN_WIDTH.square - 1}px)`)
}
