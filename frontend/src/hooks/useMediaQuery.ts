import { useEffect, useState } from 'react'

// useMediaQuery tracks a CSS media query and re-renders when it flips. SSR-safe
// (returns false when window is unavailable). Used to branch layout between the
// desktop multi-column shell and the mobile single-column shell.
export function useMediaQuery(query: string): boolean {
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
// (768px) — i.e. a portrait phone. Kept in one place so the breakpoint stays in
// sync with the `md:` utility classes used across the responsive layout.
export function useIsMobile(): boolean {
  return useMediaQuery('(max-width: 767px)')
}
