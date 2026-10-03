import { useViewportTier } from './useViewport'

// Match the shared width tier and Tailwind's md breakpoint, regardless of aspect.
export function useIsMobile(): boolean {
  return useViewportTier() === 'narrow'
}
