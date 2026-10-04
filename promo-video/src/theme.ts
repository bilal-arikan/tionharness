import { Easing, interpolate, spring } from 'remotion'

export const FPS = 30
/** 120 BPM at 30 fps: one beat is exactly 15 frames (see scripts/make-music.mjs). */
export const BEAT = 15
export const TOTAL_FRAMES = 78 * BEAT

export const C = {
  bg: '#08080b',
  bg2: '#0e0e10',
  surface: '#17171a',
  surface2: '#202024',
  border: '#2b2b30',
  text: '#f2f2f5',
  dim: '#9a9aa6',
  brand: '#863bff',
  violet: '#8b5cf6',
  blue: '#58a6ff',
  emerald: '#34d399',
  rose: '#fb7185',
  amber: '#f59e0b',
  nord: '#88c0d0',
  cyan: '#22d3ee',
  lime: '#a3e635',
  orange: '#fb923c',
  fuchsia: '#e879f9',
  danger: '#f87171',
}

/** The app's ten theme accents, in its own order. */
export const PALETTE = [C.violet, C.blue, C.emerald, C.rose, C.amber, C.nord, C.cyan, C.lime, C.orange, C.fuchsia]

export const FONT = '"Inter", ui-sans-serif, system-ui, sans-serif'

/** Scene list in beats; every cut lands on a kick. */
export const SCENES = [
  { id: 'intro', beats: 8 },
  { id: 'agents', beats: 6 },
  { id: 'delegate', beats: 6 },
  { id: 'flows', beats: 6 },
  { id: 'board', beats: 6 },
  { id: 'workspaces', beats: 6 },
  { id: 'tools', beats: 6 },
  { id: 'control', beats: 6 },
  { id: 'models', beats: 6 },
  { id: 'stats', beats: 8 },
  { id: 'themes', beats: 4 },
  { id: 'outro', beats: 10 },
] as const

/** 1 right on a beat, decaying to ~0 before the next. */
export const pulse = (frame: number, decay = 5) => Math.exp(-((frame % BEAT) / BEAT) * decay)

export const clamp = { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' } as const

export const pop = (frame: number, fps: number, delay = 0, damping = 11, stiffness = 170) =>
  spring({ frame: frame - delay, fps, config: { damping, stiffness, mass: 0.7 } })

export const ease = (frame: number, from: number, to: number, out: [number, number] = [0, 1]) =>
  interpolate(frame, [from, to], out, { ...clamp, easing: Easing.bezier(0.22, 1, 0.36, 1) })

export const easeIn = (frame: number, from: number, to: number, out: [number, number] = [0, 1]) =>
  interpolate(frame, [from, to], out, { ...clamp, easing: Easing.bezier(0.64, 0, 0.78, 0) })

export const polar = (cx: number, cy: number, r: number, deg: number) => {
  const a = ((deg - 90) * Math.PI) / 180
  return { x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) }
}

/** Point along a quadratic bezier. */
export const qPoint = (
  p0: { x: number; y: number },
  p1: { x: number; y: number },
  p2: { x: number; y: number },
  t: number,
) => ({
  x: (1 - t) * (1 - t) * p0.x + 2 * (1 - t) * t * p1.x + t * t * p2.x,
  y: (1 - t) * (1 - t) * p0.y + 2 * (1 - t) * t * p1.y + t * t * p2.y,
})
