// Shared building blocks for the three vertical flow videos (1080x1920).
import React from 'react'
import { AbsoluteFill, Easing, interpolate, random, spring, useCurrentFrame, useVideoConfig } from 'remotion'
import { loadFont as loadInter } from '@remotion/google-fonts/Inter'
import { loadFont as loadMono } from '@remotion/google-fonts/JetBrainsMono'
import { LOGO_RECTS } from '../components/Shapes'

loadInter('normal', { weights: ['400', '500', '600', '700', '800', '900'], subsets: ['latin', 'latin-ext'] })
loadMono('normal', { weights: ['500', '700'], subsets: ['latin', 'latin-ext'] })

export const W = 1080
export const H = 1920
export const FONT = '"Inter", ui-sans-serif, system-ui, sans-serif'
export const MONO = '"JetBrains Mono", ui-monospace, monospace'

// The app's dark theme tokens (frontend/src/index.css).
export const T = {
  bg: '#0b0b0e',
  surface: '#17171a',
  surface2: '#202024',
  border: '#2b2b30',
  text: '#ececf1',
  dim: '#9a9aa6',
  faint: '#5d5d68',
  accent: '#8b5cf6',
  success: '#34d399',
  warning: '#fbbf24',
  danger: '#f87171',
  info: '#38bdf8',
  fuchsia: '#e879f9',
  brand: '#863bff',
}

export const clamp = { extrapolateLeft: 'clamp', extrapolateRight: 'clamp' } as const
export const ease = (f: number, a: number, b: number, out: [number, number] = [0, 1]) =>
  interpolate(f, [a, b], out, { ...clamp, easing: Easing.bezier(0.22, 1, 0.36, 1) })
export const lin = (f: number, a: number, b: number, out: [number, number] = [0, 1]) => interpolate(f, [a, b], out, clamp)
export const pop = (f: number, fps: number, delay = 0, damping = 12, stiffness = 180) =>
  spring({ frame: f - delay, fps, config: { damping, stiffness, mass: 0.7 } })
/** 1 right on a beat, decaying before the next. */
export const pulse = (f: number, beat: number, decay = 5) => Math.exp(-((((f % beat) + beat) % beat) / beat) * decay)

// ---------------------------------------------------------------------------------
// Icons (24-unit grid, lucide-style strokes)
// ---------------------------------------------------------------------------------
const P: Record<string, React.ReactNode> = {
  play: <path d="M6 3l14 9-14 9z" />,
  bot: (
    <>
      <rect x="3" y="8" width="18" height="12" rx="2" />
      <path d="M12 8V4H8M2 14h2M20 14h2M15 13v2M9 13v2" />
    </>
  ),
  branch: (
    <>
      <path d="M6 3v12" />
      <circle cx="18" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <path d="M18 9a9 9 0 01-9 9" />
    </>
  ),
  braces: (
    <path d="M8 3H7a2 2 0 00-2 2v5a2 2 0 01-2 2 2 2 0 012 2v5a2 2 0 002 2h1M16 21h1a2 2 0 002-2v-5a2 2 0 012-2 2 2 0 01-2-2V5a2 2 0 00-2-2h-1" />
  ),
  zap: <path d="M13 2L3 14h9l-1 8 10-12h-9z" />,
  square: <rect x="4" y="4" width="16" height="16" rx="2" />,
  eye: (
    <>
      <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z" />
      <circle cx="12" cy="12" r="3" />
    </>
  ),
  check: <path d="M20 6L9 17l-5-5" />,
  x: <path d="M18 6L6 18M6 6l12 12" />,
  undo: (
    <>
      <path d="M3 7v6h6" />
      <path d="M21 17a9 9 0 00-15-6.7L3 13" />
    </>
  ),
  commit: (
    <>
      <circle cx="12" cy="12" r="3.5" />
      <path d="M3 12h5.5M15.5 12H21" />
    </>
  ),
  thumbUp: (
    <>
      <path d="M7 10v11" />
      <path d="M15 5.9L14 10h5.8a2 2 0 011.9 2.6l-2.3 8a2 2 0 01-1.9 1.4H4a2 2 0 01-2-2v-8a2 2 0 012-2h2.8a2 2 0 001.8-1.1L12 2a3.1 3.1 0 013 3.9z" />
    </>
  ),
  thumbDown: (
    <g transform="rotate(180 12 12)">
      <path d="M7 10v11" />
      <path d="M15 5.9L14 10h5.8a2 2 0 011.9 2.6l-2.3 8a2 2 0 01-1.9 1.4H4a2 2 0 01-2-2v-8a2 2 0 012-2h2.8a2 2 0 001.8-1.1L12 2a3.1 3.1 0 013 3.9z" />
    </g>
  ),
  shield: <path d="M20 13c0 5-3.5 7.5-7.7 9-4.2-1.5-8.3-4-8.3-9V6c2 0 4.5-1.2 6.2-2.7a1.2 1.2 0 011.6 0C13.5 4.8 16 6 18 6h2z" />,
  scale: (
    <>
      <path d="M16 16l3-8 3 8c-.9.6-1.9 1-3 1s-2.1-.4-3-1zM2 16l3-8 3 8c-.9.6-1.9 1-3 1s-2.1-.4-3-1z" />
      <path d="M7 21h10M12 3v18M3 7h2c2 0 5-1 7-2 2 1 5 2 7 2h2" />
    </>
  ),
  repeat: <path d="M17 2l4 4-4 4M3 11v-1a4 4 0 014-4h14M7 22l-4-4 4-4M21 13v1a4 4 0 01-4 4H3" />,
  flag: <path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1zM4 22v-7" />,
  message: <path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z" />,
  sparkle: <path d="M12 3l1.9 5.8L20 11l-6.1 2.2L12 19l-1.9-5.8L4 11l6.1-2.2z" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  funnel: <path d="M3 4h18l-7 8.5V19l-4 2v-8.5z" />,
  list: <path d="M8 6h13M8 12h13M8 18h13M3 6h.01M3 12h.01M3 18h.01" />,
  wrench: <path d="M14.7 6.3a4 4 0 005.4 5.1L21 12l-9 9a2.1 2.1 0 01-3-3l9-9-.6-.9a4 4 0 01-5.1-5.4l2.6 2.6 2.1-.5.5-2.1z" />,
  gauge: (
    <>
      <path d="M12 14l4-4" />
      <path d="M3.3 19a10 10 0 1117.4 0" />
    </>
  ),
  layers: <path d="M12 2l10 5-10 5L2 7zM2 17l10 5 10-5M2 12l10 5 10-5" />,
  send: <path d="M22 2L11 13M22 2l-7 20-4-9-9-4z" />,
}
export type IconName = keyof typeof P

export const Icon: React.FC<{ name: IconName; size?: number; color?: string; stroke?: number; fill?: string; style?: React.CSSProperties }> = ({
  name,
  size = 32,
  color = T.text,
  stroke = 2,
  fill = 'none',
  style,
}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" fill={fill} stroke={color} strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0, ...style }}>
    {P[name]}
  </svg>
)

export const Star: React.FC<{ size?: number; fill?: number; color?: string }> = ({ size = 34, fill = 1, color = T.warning }) => {
  const id = `st${Math.round(fill * 100)}-${size}-${color.replace(/[^a-z0-9]/gi, "")}`
  return (
    <svg width={size} height={size} viewBox="0 0 24 24">
      <defs>
        <linearGradient id={id}>
          <stop offset={fill} stopColor={color} />
          <stop offset={fill} stopColor={T.border} />
        </linearGradient>
      </defs>
      <path d="M12 2l3.1 6.3 6.9 1-5 4.9 1.2 6.8L12 17.8 5.8 21l1.2-6.8-5-4.9 6.9-1z" fill={`url(#${id})`} />
    </svg>
  )
}

// ---------------------------------------------------------------------------------
// Node types (frontend/src/features/flows/nodeChrome.ts)
// ---------------------------------------------------------------------------------
export type NodeType = 'input' | 'llm' | 'route' | 'transform' | 'trigger' | 'output'
export const NODE: Record<NodeType, { label: string; icon: IconName; color: string }> = {
  input: { label: 'input', icon: 'play', color: T.success },
  llm: { label: 'model', icon: 'bot', color: T.accent },
  route: { label: 'route', icon: 'branch', color: T.warning },
  transform: { label: 'transform', icon: 'braces', color: T.info },
  trigger: { label: 'trigger', icon: 'zap', color: T.fuchsia },
  output: { label: 'output', icon: 'square', color: T.success },
}

export type Status = 'idle' | 'running' | 'done' | 'error'

/** A flow node card, centred on (x, y). */
export const NodeCard: React.FC<{
  type: NodeType
  title: string
  x: number
  y: number
  w?: number
  t?: number
  status?: Status
  chips?: string[]
  badge?: React.ReactNode
  scale?: number
  dim?: boolean
}> = ({ type, title, x, y, w = 560, t = 1, status = 'idle', chips, badge, scale = 1, dim }) => {
  const frame = useCurrentFrame()
  const n = NODE[type]
  const ring =
    status === 'running'
      ? `0 0 0 3px ${T.accent}, 0 0 ${38 + 14 * Math.sin(frame / 3)}px ${T.accent}aa`
      : status === 'done'
        ? `0 0 0 3px ${T.success}, 0 0 24px ${T.success}55`
        : status === 'error'
          ? `0 0 0 3px ${T.danger}, 0 0 24px ${T.danger}66`
          : `0 18px 50px -20px #000`
  return (
    <div
      style={{
        position: 'absolute',
        left: x,
        top: y,
        width: w,
        transform: `translate(-50%, -50%) scale(${(0.7 + 0.3 * t) * scale})`,
        opacity: Math.min(1, t * 1.5) * (dim ? 0.35 : 1),
        background: `linear-gradient(180deg, ${T.surface2}, ${T.surface})`,
        border: `2px solid ${T.border}`,
        borderRadius: 26,
        padding: '22px 26px',
        boxShadow: ring,
        fontFamily: FONT,
        display: 'flex',
        flexDirection: 'column',
        gap: 14,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
        <div
          style={{
            width: 64,
            height: 64,
            borderRadius: 18,
            background: `${n.color}22`,
            border: `2px solid ${n.color}66`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <Icon name={n.icon} size={34} color={n.color} stroke={2.2} />
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: 22, fontWeight: 700, letterSpacing: 2, textTransform: 'uppercase', color: n.color }}>{n.label}</div>
          <div style={{ fontSize: 38, fontWeight: 700, color: T.text, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{title}</div>
        </div>
        {badge}
        {status === 'done' ? <Icon name="check" size={40} color={T.success} stroke={3} /> : null}
        {status === 'error' ? <Icon name="x" size={40} color={T.danger} stroke={3} /> : null}
      </div>
      {chips?.length ? (
        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
          {chips.map((c) => (
            <span key={c} style={{ fontSize: 22, fontWeight: 600, color: T.dim, background: T.bg, border: `1.5px solid ${T.border}`, borderRadius: 99, padding: '5px 14px' }}>
              {c}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  )
}

// ---------------------------------------------------------------------------------
// Edges
// ---------------------------------------------------------------------------------
export type Pt = { x: number; y: number }
export type Curve = { p0: Pt; c1: Pt; c2: Pt; p3: Pt }

/** Vertical S-curve between two points unless control points are given. */
export const curve = (p0: Pt, p3: Pt, c1?: Pt, c2?: Pt): Curve => {
  const dy = (p3.y - p0.y) * 0.5
  return { p0, p3, c1: c1 ?? { x: p0.x, y: p0.y + dy }, c2: c2 ?? { x: p3.x, y: p3.y - dy } }
}
export const at = (c: Curve, t: number): Pt => {
  const u = 1 - t
  return {
    x: u * u * u * c.p0.x + 3 * u * u * t * c.c1.x + 3 * u * t * t * c.c2.x + t * t * t * c.p3.x,
    y: u * u * u * c.p0.y + 3 * u * u * t * c.c1.y + 3 * u * t * t * c.c2.y + t * t * t * c.p3.y,
  }
}
const dPath = (c: Curve) => `M${c.p0.x},${c.p0.y} C${c.c1.x},${c.c1.y} ${c.c2.x},${c.c2.y} ${c.p3.x},${c.p3.y}`

/** Full-frame SVG layer for edges (put it under the node cards). */
export const EdgeLayer: React.FC<{ children: React.ReactNode; style?: React.CSSProperties }> = ({ children, style }) => (
  <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} style={{ position: 'absolute', left: 0, top: 0, overflow: 'visible', ...style }}>
    <defs>
      <filter id="eglow" x="-50%" y="-50%" width="200%" height="200%">
        <feGaussianBlur stdDeviation="6" result="b" />
        <feMerge>
          <feMergeNode in="b" />
          <feMergeNode in="SourceGraphic" />
        </feMerge>
      </filter>
    </defs>
    {children}
  </svg>
)

export const Edge: React.FC<{ c: Curve; p?: number; color?: string; dashed?: boolean; active?: number; label?: string; labelAt?: number; labelColor?: string; arrow?: boolean }> = ({
  c,
  p = 1,
  color = T.faint,
  dashed,
  active = 0,
  label,
  labelAt = 0.5,
  labelColor,
  arrow = true,
}) => {
  if (p <= 0) return null
  const end = at(c, Math.min(1, p))
  const prev = at(c, Math.max(0, Math.min(1, p) - 0.02))
  const ang = (Math.atan2(end.y - prev.y, end.x - prev.x) * 180) / Math.PI
  const lp = at(c, labelAt)
  const col = active > 0 ? interpolateHex(color, '#ffffff', active * 0.35) : color
  return (
    <g>
      <path
        d={dPath(c)}
        fill="none"
        stroke={col}
        strokeWidth={5 + active * 2}
        strokeLinecap="round"
        pathLength={1}
        strokeDasharray={dashed ? undefined : `${p} 2`}
        style={dashed ? { strokeDasharray: '0.025 0.02', opacity: Math.min(1, p * 2) } : undefined}
        filter={active > 0.2 ? 'url(#eglow)' : undefined}
      />
      {arrow && p > 0.9 ? <path d="M-16,-11 L2,0 L-16,11 Z" fill={col} transform={`translate(${end.x} ${end.y}) rotate(${ang})`} /> : null}
      {label && p > labelAt ? (
        <g transform={`translate(${lp.x} ${lp.y})`}>
          <rect x={-label.length * 9 - 18} y={-24} width={label.length * 18 + 36} height={48} rx={24} fill={T.bg} stroke={labelColor ?? color} strokeWidth={2.5} />
          <text textAnchor="middle" dy={9} fontFamily={FONT} fontWeight={700} fontSize={26} fill={labelColor ?? color}>
            {label}
          </text>
        </g>
      ) : null}
    </g>
  )
}

/** Glowing token travelling along a curve; t in [0, 1]. */
export const Token: React.FC<{ c: Curve; t: number; color?: string; r?: number }> = ({ c, t, color = '#fff', r = 13 }) => {
  if (t <= 0 || t >= 1) return null
  const p = at(c, t)
  const tail = [0.04, 0.08, 0.12].map((d) => at(c, Math.max(0, t - d)))
  return (
    <g>
      {tail.map((q, i) => (
        <circle key={i} cx={q.x} cy={q.y} r={r * (0.8 - i * 0.2)} fill={color} opacity={0.35 - i * 0.1} />
      ))}
      <circle cx={p.x} cy={p.y} r={r * 2.2} fill={color} opacity={0.25} />
      <circle cx={p.x} cy={p.y} r={r} fill="#fff" filter="url(#eglow)" />
    </g>
  )
}

const hex = (h: string) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16))
export const interpolateHex = (a: string, b: string, t: number) => {
  const A = hex(a)
  const B = hex(b)
  return `rgb(${A.map((v, i) => Math.round(v + (B[i] - v) * t)).join(',')})`
}

// ---------------------------------------------------------------------------------
// Type
// ---------------------------------------------------------------------------------
/** Accent-coloured run inside a headline. */
export const Hl: React.FC<{ c?: string; children: React.ReactNode }> = ({ c = T.accent, children }) => (
  <span style={{ color: c, textShadow: `0 0 40px ${c}66` }}>{children}</span>
)

export const Eyebrow: React.FC<{ text: string; color: string; at?: number; top?: number }> = ({ text, color, at: delay = 0, top = 230 }) => {
  const frame = useCurrentFrame()
  const t = ease(frame, delay, delay + 14)
  return (
    <div style={{ position: 'absolute', top, left: 0, width: W, display: 'flex', justifyContent: 'center', opacity: t }}>
      <div
        style={{
          fontFamily: FONT,
          fontWeight: 800,
          fontSize: 26,
          letterSpacing: 6,
          color,
          padding: '10px 26px',
          borderRadius: 99,
          border: `2px solid ${color}66`,
          background: `${color}14`,
          transform: `translateY(${(1 - t) * 20}px)`,
        }}
      >
        {text}
      </div>
    </div>
  )
}

export const Headline: React.FC<{ lines: React.ReactNode[]; at?: number; top?: number; size?: number; gap?: number; align?: 'center' | 'left' }> = ({
  lines,
  at: delay = 2,
  top = 300,
  size = 88,
  gap = 7,
}) => {
  const frame = useCurrentFrame()
  return (
    <div style={{ position: 'absolute', top, left: 60, width: W - 120, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
      {lines.map((l, i) => {
        const t = ease(frame, delay + i * gap, delay + i * gap + 16)
        return (
          <div key={i} style={{ overflow: 'hidden', paddingBottom: 6 }}>
            <div
              style={{
                fontFamily: FONT,
                fontWeight: 800,
                fontSize: size,
                lineHeight: 1.08,
                letterSpacing: -size * 0.025,
                color: T.text,
                textAlign: 'center',
                transform: `translateY(${(1 - t) * size * 1.5}px)`,
                opacity: t,
              }}
            >
              {l}
            </div>
          </div>
        )
      })}
    </div>
  )
}

export const Caption: React.FC<{ children: React.ReactNode; at?: number; top?: number; size?: number; width?: number }> = ({ children, at: delay = 20, top = 1560, size = 38, width = 900 }) => {
  const frame = useCurrentFrame()
  const t = ease(frame, delay, delay + 16)
  return (
    <div
      style={{
        position: 'absolute',
        top,
        left: (W - width) / 2,
        width,
        textAlign: 'center',
        fontFamily: FONT,
        fontWeight: 500,
        fontSize: size,
        lineHeight: 1.35,
        color: T.dim,
        opacity: t,
        transform: `translateY(${(1 - t) * 24}px)`,
      }}
    >
      {children}
    </div>
  )
}

export const Chip: React.FC<{ text: React.ReactNode; color?: string; t?: number; icon?: IconName; size?: number; mono?: boolean; solid?: boolean }> = ({
  text,
  color = T.dim,
  t = 1,
  icon,
  size = 28,
  mono,
  solid,
}) => (
  <div
    style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 10,
      fontFamily: mono ? MONO : FONT,
      fontWeight: 700,
      fontSize: size,
      color: solid ? T.bg : color,
      background: solid ? color : `${color}18`,
      border: `2px solid ${color}${solid ? '' : '55'}`,
      borderRadius: 99,
      padding: `${size * 0.32}px ${size * 0.7}px`,
      opacity: Math.min(1, t * 1.4),
      transform: `scale(${0.6 + 0.4 * t})`,
      whiteSpace: 'nowrap',
    }}
  >
    {icon ? <Icon name={icon} size={size * 1.05} color={solid ? T.bg : color} stroke={2.5} /> : null}
    {text}
  </div>
)

/** Row of chips that pop in one after another. */
export const ChipRow: React.FC<{ items: { text: React.ReactNode; color?: string; icon?: IconName; mono?: boolean }[]; at?: number; step?: number; top: number; size?: number; width?: number }> = ({
  items,
  at: delay = 0,
  step = 4,
  top,
  size = 28,
  width = 980,
}) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  return (
    <div style={{ position: 'absolute', top, left: (W - width) / 2, width, display: 'flex', flexWrap: 'wrap', justifyContent: 'center', gap: 14 }}>
      {items.map((it, i) => (
        <Chip key={i} {...it} size={size} t={pop(frame, fps, delay + i * step)} />
      ))}
    </div>
  )
}

export const typed = (text: string, frame: number, start: number, cps = 1.6) => text.slice(0, Math.max(0, Math.floor((frame - start) * cps)))

/** Rounded app-like panel. */
export const Panel: React.FC<{ x: number; y: number; w: number; h: number; t?: number; children?: React.ReactNode; title?: React.ReactNode; style?: React.CSSProperties }> = ({
  x,
  y,
  w,
  h,
  t = 1,
  children,
  title,
  style,
}) => (
  <div
    style={{
      position: 'absolute',
      left: x,
      top: y,
      width: w,
      height: h,
      borderRadius: 34,
      background: `linear-gradient(180deg, ${T.surface}, #121215)`,
      border: `2px solid ${T.border}`,
      boxShadow: '0 40px 120px -30px #000, inset 0 1px 0 #ffffff10',
      overflow: 'hidden',
      opacity: Math.min(1, t * 1.3),
      transform: `translateY(${(1 - t) * 80}px) scale(${0.94 + 0.06 * t})`,
      fontFamily: FONT,
      ...style,
    }}
  >
    {title ? (
      <div style={{ height: 76, borderBottom: `2px solid ${T.border}`, display: 'flex', alignItems: 'center', padding: '0 28px', gap: 14 }}>
        {[T.danger, T.warning, T.success].map((c) => (
          <div key={c} style={{ width: 16, height: 16, borderRadius: 8, background: c, opacity: 0.8 }} />
        ))}
        <div style={{ marginLeft: 14, fontSize: 26, fontWeight: 600, color: T.dim, flex: 1 }}>{title}</div>
      </div>
    ) : null}
    {children}
  </div>
)

// ---------------------------------------------------------------------------------
// Scene chrome
// ---------------------------------------------------------------------------------
/** Enter/exit transition around a scene. */
export const SceneWrap: React.FC<{ duration: number; children: React.ReactNode; enter?: number; exit?: number }> = ({ duration, children, enter = 12, exit = 8 }) => {
  const frame = useCurrentFrame()
  const tin = ease(frame, 0, enter)
  const tout = lin(frame, duration - exit, duration)
  return (
    <AbsoluteFill
      style={{
        opacity: tin * (1 - tout),
        transform: `translateY(${(1 - tin) * 50 - tout * 40}px) scale(${1 - tout * 0.05})`,
        filter: `blur(${(1 - tin) * 10 + tout * 10}px)`,
      }}
    >
      {children}
    </AbsoluteFill>
  )
}

/** Story-style progress segments + brand mark at the top. */
export const TopBar: React.FC<{ scenes: { from: number; duration: number }[]; accent: string; label: string }> = ({ scenes, accent, label }) => {
  const frame = useCurrentFrame()
  return (
    <div style={{ position: 'absolute', top: 120, left: 60, width: W - 120 }}>
      <div style={{ display: 'flex', gap: 10 }}>
        {scenes.map((s, i) => {
          const p = lin(frame, s.from, s.from + s.duration)
          return (
            <div key={i} style={{ flex: s.duration, height: 7, borderRadius: 4, background: '#ffffff1f', overflow: 'hidden' }}>
              <div style={{ width: `${p * 100}%`, height: '100%', background: accent, boxShadow: `0 0 12px ${accent}` }} />
            </div>
          )
        })}
      </div>
      <div style={{ marginTop: 22, display: 'flex', alignItems: 'center', gap: 14, fontFamily: FONT, fontWeight: 700, fontSize: 28, color: T.dim }}>
        <Logo size={40} />
        <span style={{ color: T.text }}>TionHarness</span>
        <span style={{ opacity: 0.6 }}>·</span>
        <span>{label}</span>
      </div>
    </div>
  )
}

export const Logo: React.FC<{ size: number; glow?: boolean }> = ({ size, glow }) => (
  <svg width={size} height={size} viewBox="0 0 48 48" style={{ filter: glow ? `drop-shadow(0 0 ${size / 4}px ${T.brand})` : undefined }}>
    <rect width={48} height={48} rx={11} fill={T.brand} />
    {LOGO_RECTS.map((r, i) => (
      <rect key={i} x={r.x} y={r.y} width={r.w} height={r.h} rx={1} fill="#fff" />
    ))}
  </svg>
)

/** Dark canvas with drifting grid, accent glows and beat-synced breathing. */
export const FlowBackground: React.FC<{ accent: string; accent2: string; beat: number }> = ({ accent, accent2, beat }) => {
  const frame = useCurrentFrame()
  const p = pulse(frame, beat, 4)
  const cell = 90
  const drift = (frame * 0.8) % cell
  return (
    <AbsoluteFill style={{ background: T.bg, overflow: 'hidden' }}>
      <AbsoluteFill
        style={{
          backgroundImage: `radial-gradient(${T.border} 2.2px, transparent 2.6px)`,
          backgroundSize: `${cell / 2}px ${cell / 2}px`,
          backgroundPosition: `0px ${drift}px`,
          maskImage: 'radial-gradient(ellipse at 50% 50%, black 15%, transparent 70%)',
          WebkitMaskImage: 'radial-gradient(ellipse at 50% 50%, black 15%, transparent 70%)',
          opacity: 0.5 + p * 0.25,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: 1700,
          height: 1700,
          left: W * 0.7 - 850 + Math.sin(frame / 45) * 70,
          top: H * 0.3 - 850 + Math.cos(frame / 55) * 90,
          background: `radial-gradient(circle, ${accent}40 0%, ${accent}00 60%)`,
          transform: `scale(${1 + p * 0.05})`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: 1500,
          height: 1500,
          left: W * 0.2 - 750 + Math.cos(frame / 38) * 80,
          top: H * 0.82 - 750,
          background: `radial-gradient(circle, ${accent2}30 0%, ${accent2}00 60%)`,
        }}
      />
      {new Array(34).fill(0).map((_, i) => {
        const x = random(`fx${i}`) * W
        const sp = 0.3 + random(`fs${i}`) * 1.1
        const y = (((random(`fy${i}`) * H - frame * sp) % H) + H) % H
        const s = 2 + random(`fz${i}`) * 3
        return (
          <div
            key={i}
            style={{ position: 'absolute', left: x, top: y, width: s, height: s, borderRadius: s, background: '#fff', opacity: 0.06 + 0.2 * random(`fo${i}`) * (0.5 + 0.5 * Math.sin(frame / 11 + i)) }}
          />
        )
      })}
      <AbsoluteFill style={{ background: 'radial-gradient(ellipse at center, transparent 55%, rgba(0,0,0,0.7) 100%)' }} />
    </AbsoluteFill>
  )
}

export const CutFlash: React.FC<{ at: number[]; strength?: number }> = ({ at: cuts, strength = 0.3 }) => {
  const frame = useCurrentFrame()
  let o = 0
  for (const a of cuts) {
    const d = frame - a
    if (d >= 0 && d < 6) o = Math.max(o, (1 - d / 6) * strength)
  }
  return <AbsoluteFill style={{ background: '#fff', opacity: o, mixBlendMode: 'overlay', pointerEvents: 'none' }} />
}

export const shake = (frame: number, at0: number, strength = 12) => {
  const d = frame - at0
  if (d < 0 || d > 12) return { x: 0, y: 0 }
  const k = (1 - d / 12) * strength
  return { x: Math.sin(d * 7.3) * k, y: Math.cos(d * 9.1) * k }
}

/** Shared closing card. */
export const Outro: React.FC<{ duration: number; title: React.ReactNode; tagline: string; accent: string; chips: string[] }> = ({ duration, title, tagline, accent, chips }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const logo = pop(frame, fps, 2, 10, 150)
  const fade = lin(frame, duration - 14, duration)
  return (
    <AbsoluteFill style={{ opacity: 1 - fade }}>
      <div style={{ position: 'absolute', top: 560, width: W, display: 'flex', justifyContent: 'center', transform: `scale(${logo}) rotate(${(1 - logo) * -40}deg)` }}>
        <Logo size={220} glow />
      </div>
      <svg width={W} height={H} style={{ position: 'absolute', left: 0, top: 0 }}>
        {[0, 1, 2].map((i) => {
          const t = lin(frame, 4 + i * 6, 40 + i * 6)
          if (t <= 0 || t >= 1) return null
          return <circle key={i} cx={W / 2} cy={670} r={130 + t * 420} fill="none" stroke={i === 1 ? accent : '#fff'} strokeWidth={8 * (1 - t)} opacity={1 - t} />
        })}
      </svg>
      <div style={{ position: 'absolute', top: 870, left: 60, width: W - 120, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 34 }}>
        <div
          style={{
            fontFamily: FONT,
            fontWeight: 800,
            fontSize: 96,
            lineHeight: 1.08,
            letterSpacing: -2.4,
            color: T.text,
            textAlign: 'center',
            opacity: ease(frame, 10, 26),
            transform: `translateY(${(1 - ease(frame, 10, 26)) * 60}px)`,
          }}
        >
          {title}
        </div>
        <div style={{ fontFamily: FONT, fontWeight: 500, fontSize: 44, color: T.dim, textAlign: 'center', opacity: ease(frame, 20, 36) }}>{tagline}</div>
        <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'center', gap: 14 }}>
          {chips.map((c, i) => (
            <Chip key={c} text={c} color={accent} size={30} t={pop(frame, fps, 28 + i * 3)} />
          ))}
        </div>
        <div style={{ marginTop: 40, fontFamily: FONT, fontWeight: 700, fontSize: 34, letterSpacing: 4, color: T.dim, opacity: ease(frame, 34, 50) }}>NOW IN TIONHARNESS</div>
      </div>
    </AbsoluteFill>
  )
}

export type SceneDef = { id: string; beats: number; accent: string }
export const buildTimeline = <S extends SceneDef>(scenes: S[], beat: number) => {
  let a = 0
  return scenes.map((s) => {
    const from = a
    a += s.beats * beat
    return { ...s, from, duration: s.beats * beat }
  })
}
