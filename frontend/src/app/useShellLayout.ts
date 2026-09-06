import { useMemo } from 'react'
import { useViewport } from '@/shared/hooks/useViewport'
import type { ViewportAspect, ViewportTier } from '@/shared/lib/viewport'

// How each shell column presents at the current viewport class. Derived purely
// from the tier/aspect so App.tsx, NavRail and the sidebars all agree; user
// toggles (detail open, list drawer open, rail collapsed) stay where they are
// and are combined with these modes at the call site.
export interface ShellLayout {
  // Primary navigation: hidden (bottom bar instead), an icon-only rail that
  // expands as a temporary overlay, or the full user-collapsible rail.
  rail: 'hidden' | 'compact' | 'full'
  // Left list column (sessions / roster): a slide-in drawer or a docked column.
  list: 'drawer' | 'docked'
  // Right-hand session detail panel: docked column vs a right slide-in drawer.
  detail: 'drawer' | 'docked'
  // Whether the main content keeps a centred reading measure (CSS-driven via
  // <html data-viewport>; exposed here so components can branch on it too).
  measure: 'fluid' | 'centered'
}

export function computeShellLayout(tier: ViewportTier, aspect: ViewportAspect): ShellLayout {
  const narrow = tier === 'narrow'
  const square = tier === 'square'
  return {
    rail: narrow ? 'hidden' : square ? 'compact' : 'full',
    list: narrow ? 'drawer' : 'docked',
    // A docked detail column needs horizontal room: never on narrow/square
    // widths, and not on portrait-oriented monitors either (tall, not wide).
    detail: narrow || square || aspect === 'portrait' ? 'drawer' : 'docked',
    measure: tier === 'ultra' ? 'centered' : 'fluid',
  }
}

export function useShellLayout(): ShellLayout {
  const { tier, aspect } = useViewport()
  return useMemo(() => computeShellLayout(tier, aspect), [tier, aspect])
}
