import React from 'react'
import { C } from '../theme'

/** Shared SVG filters; every scene artboard includes these once. */
export const Defs: React.FC = () => (
  <defs>
    <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
      <feGaussianBlur stdDeviation="10" result="b" />
      <feMerge>
        <feMergeNode in="b" />
        <feMergeNode in="SourceGraphic" />
      </feMerge>
    </filter>
    <filter id="glowBig" x="-80%" y="-80%" width="260%" height="260%">
      <feGaussianBlur stdDeviation="22" result="b" />
      <feMerge>
        <feMergeNode in="b" />
        <feMergeNode in="SourceGraphic" />
      </feMerge>
    </filter>
    <filter id="soft" x="-50%" y="-50%" width="200%" height="200%">
      <feGaussianBlur stdDeviation="4" />
    </filter>
  </defs>
)

/** The TH monogram -- same rects as website/public/favicon.svg. */
export const LOGO_RECTS = [
  { x: 7, y: 14, w: 15, h: 4.4 },
  { x: 12.3, y: 14, w: 4.4, h: 20 },
  { x: 26, y: 14, w: 4.4, h: 20 },
  { x: 36.6, y: 14, w: 4.4, h: 20 },
  { x: 26, y: 21.8, w: 15, h: 4.4 },
]

/** Static logo badge centred at (cx, cy) with the given edge size. */
export const LogoBadge: React.FC<{ cx: number; cy: number; size: number; glow?: boolean }> = ({ cx, cy, size, glow }) => {
  const k = size / 48
  return (
    <g transform={`translate(${cx - size / 2} ${cy - size / 2}) scale(${k})`} filter={glow ? 'url(#glow)' : undefined}>
      <rect width={48} height={48} rx={11} fill={C.brand} />
      {LOGO_RECTS.map((r, i) => (
        <rect key={i} x={r.x} y={r.y} width={r.w} height={r.h} rx={1} fill="#fff" />
      ))}
    </g>
  )
}

/** Four-point spark used as the "agent" glyph. */
export const Spark: React.FC<{ x: number; y: number; r: number; rot?: number; fill?: string; opacity?: number }> = ({
  x,
  y,
  r,
  rot = 0,
  fill = '#fff',
  opacity = 1,
}) => (
  <path
    transform={`translate(${x} ${y}) rotate(${rot}) scale(${r / 10})`}
    d="M0,-10 C1.2,-3 3,-1.2 10,0 C3,1.2 1.2,3 0,10 C-1.2,3 -3,1.2 -10,0 C-3,-1.2 -1.2,-3 0,-10 Z"
    fill={fill}
    opacity={opacity}
  />
)

/** Line icons on a 24-unit grid, drawn with strokes. */
export const ICONS: Record<string, React.ReactNode> = {
  file: (
    <>
      <path d="M6 2h8l4 4v16H6z" />
      <path d="M14 2v4h4M9 12h6M9 16h6" />
    </>
  ),
  terminal: (
    <>
      <rect x="2" y="4" width="20" height="16" rx="2" />
      <path d="M6 10l3 2-3 2M12 15h5" />
    </>
  ),
  search: (
    <>
      <circle cx="10.5" cy="10.5" r="6" />
      <path d="M15 15l6 6" />
    </>
  ),
  globe: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c3 3.5 3 14.5 0 18M12 3c-3 3.5-3 14.5 0 18" />
    </>
  ),
  gear: (
    <>
      <circle cx="12" cy="12" r="3.2" />
      <path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1" />
    </>
  ),
  code: <path d="M8 6l-6 6 6 6M16 6l6 6-6 6M14 4l-4 16" />,
  plug: (
    <>
      <path d="M9 2v5M15 2v5M6 7h12v4a6 6 0 01-12 0zM12 17v5" />
    </>
  ),
  branch: (
    <>
      <circle cx="6" cy="5" r="2.2" />
      <circle cx="6" cy="19" r="2.2" />
      <circle cx="18" cy="8" r="2.2" />
      <path d="M6 7.2v9.6M18 10.2c0 4-6 3-11 7" />
    </>
  ),
  check: <path d="M5 12.5l4.5 4.5L19 7.5" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3.5 2" />
    </>
  ),
  key: (
    <>
      <circle cx="7" cy="12" r="4.5" />
      <path d="M11.5 12H22M18.5 12v3.5M21.5 12v2.5" />
    </>
  ),
  db: (
    <>
      <ellipse cx="12" cy="5.5" rx="8" ry="3" />
      <path d="M4 5.5v13c0 1.7 3.6 3 8 3s8-1.3 8-3v-13M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" />
    </>
  ),
  cube: (
    <>
      <path d="M12 2l9 5v10l-9 5-9-5V7z" />
      <path d="M3 7l9 5 9-5M12 12v10" />
    </>
  ),
}

export const Icon: React.FC<{
  name: keyof typeof ICONS
  x: number
  y: number
  size: number
  color?: string
  width?: number
  opacity?: number
}> = ({ name, x, y, size, color = '#fff', width = 2, opacity = 1 }) => (
  <g
    transform={`translate(${x - size / 2} ${y - size / 2}) scale(${size / 24})`}
    fill="none"
    stroke={color}
    strokeWidth={width}
    strokeLinecap="round"
    strokeLinejoin="round"
    opacity={opacity}
  >
    {ICONS[name]}
  </g>
)

/** Expanding ring, e.g. a shockwave. t in [0, 1]. */
export const Ring: React.FC<{ cx: number; cy: number; r0: number; r1: number; t: number; color: string; width?: number }> = ({
  cx,
  cy,
  r0,
  r1,
  t,
  color,
  width = 6,
}) => {
  if (t <= 0 || t >= 1) return null
  return <circle cx={cx} cy={cy} r={r0 + (r1 - r0) * t} fill="none" stroke={color} strokeWidth={width * (1 - t)} opacity={1 - t} />
}
