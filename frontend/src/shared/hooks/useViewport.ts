import { useEffect, useSyncExternalStore } from 'react'
import {
  classifyViewport,
  type ViewportAspect,
  type ViewportClass,
  type ViewportTier,
} from '@/shared/lib/viewport'

// One window-level subscription shared by every consumer: the viewport is
// classified on resize and listeners are only notified when the TIER or ASPECT
// actually changes, so components re-render at layout boundaries rather than on
// every pixel of a drag-resize. SSR / node-test safe (falls back to 'wide').

const FALLBACK: ViewportClass = { tier: 'wide', aspect: 'landscape' }

let current: ViewportClass = FALLBACK
let listeners = new Set<() => void>()
let installed = false
let frame = 0

function measure(): ViewportClass {
  if (typeof window === 'undefined') return FALLBACK
  return classifyViewport(window.innerWidth, window.innerHeight)
}

function refresh() {
  frame = 0
  const next = measure()
  if (next.tier === current.tier && next.aspect === current.aspect) return
  current = next
  listeners.forEach((fn) => fn())
}

function onResize() {
  // Coalesce the resize burst into one classification per frame.
  if (frame) return
  frame = window.requestAnimationFrame(refresh)
}

function subscribe(fn: () => void) {
  if (!installed && typeof window !== 'undefined') {
    installed = true
    current = measure()
    window.addEventListener('resize', onResize)
    window.addEventListener('orientationchange', onResize)
  }
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

function getSnapshot() {
  return current
}

function getServerSnapshot() {
  return FALLBACK
}

// useViewport returns the current width tier + aspect class. Stable object
// identity between changes, so it is safe to use as an effect dependency.
export function useViewport(): ViewportClass {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot)
}

export function useViewportTier(): ViewportTier {
  return useViewport().tier
}

export function useViewportAspect(): ViewportAspect {
  return useViewport().aspect
}

// useViewportAttribute mirrors the classification onto <html data-viewport
// data-aspect> so plain CSS (styles/layout.css) can key the reading measure,
// column transitions and drawer behaviour off the same tiers the JS shell uses.
// Call it once, at the app root.
export function useViewportAttribute() {
  const { tier, aspect } = useViewport()
  useEffect(() => {
    const root = document.documentElement
    root.dataset.viewport = tier
    root.dataset.aspect = aspect
  }, [tier, aspect])
}

// Test hook: reset the module store between cases.
export function __resetViewportStoreForTests() {
  listeners = new Set()
  installed = false
  current = FALLBACK
  frame = 0
}
