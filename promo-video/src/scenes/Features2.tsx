import React from 'react'
import { interpolate, random, useCurrentFrame, useVideoConfig } from 'remotion'
import { SceneFrame } from '../components/Layout'
import { Defs, ICONS, Icon, LogoBadge, Ring, Spark } from '../components/Shapes'
import { C, clamp, ease, polar, pop, pulse } from '../theme'

type P = { duration: number }

/** Board-driven execution: a card hops columns; each landing fires an agent. */
export const Board: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const colX = [40, 280, 520, 760]
  const colW = 200
  const top = 150
  const cardH = 92
  const gap = 18
  const tagColors = [C.blue, C.rose, C.amber, C.violet, C.cyan, C.lime]
  const counts = [3, 2, 2, 1]
  const headColors = [C.dim, C.blue, C.amber, C.emerald]

  // hero card hops: col0 -> 1 -> 2 -> 3
  const hops = [
    { at: 22, from: 0, to: 1 },
    { at: 44, from: 1, to: 2 },
    { at: 66, from: 2, to: 3 },
  ]
  let heroCol = 0
  let travel = 0
  let hop = -1
  hops.forEach((h, i) => {
    if (frame >= h.at) {
      heroCol = h.to
      hop = i
      travel = ease(frame, h.at, h.at + 10)
    }
  })
  const fromCol = hop >= 0 ? hops[hop].from : 0
  const slotY = (slot: number) => top + 70 + slot * (cardH + gap)
  const heroX = interpolate(travel, [0, 1], [colX[fromCol], colX[heroCol]]) + 15
  const lift = hop >= 0 ? Math.sin(travel * Math.PI) : 0
  const heroY = slotY(0) - lift * 90
  // Cards make room for the hero card in its column and close the gap it leaves.
  const shiftFor = (ci: number) => {
    if (hop < 0) return ci === 0 ? 1 : 0
    const t = ease(frame, hops[hop].at, hops[hop].at + 8)
    if (ci === hops[hop].to) return t
    if (ci === hops[hop].from) return 1 - t
    return 0
  }

  return (
    <SceneFrame duration={duration} word="Automate" accent={C.emerald}>
      <Defs />
      {colX.map((x, ci) => {
        const s = ease(frame, ci * 2, ci * 2 + 12)
        const shift = shiftFor(ci)
        return (
          <g key={ci} opacity={s} transform={`translate(0 ${(1 - s) * 60})`}>
            <rect x={x} y={top} width={colW} height={720} rx={24} fill={C.surface} stroke={C.border} strokeWidth={3} />
            <circle cx={x + 30} cy={top + 34} r={9} fill={headColors[ci]} />
            <rect x={x + 50} y={top + 28} width={80} height={12} rx={6} fill={C.surface2} />
            {new Array(counts[ci]).fill(0).map((_, k) => {
              const drop = pop(frame, fps, 4 + ci * 3 + k * 2, 12)
              const y = slotY(k + shift)
              const tag = tagColors[(ci * 3 + k) % tagColors.length]
              return (
                <g key={k} transform={`translate(${x + 15} ${y - (1 - drop) * 200})`} opacity={drop}>
                  <rect width={colW - 30} height={cardH} rx={16} fill={C.surface2} stroke={C.border} strokeWidth={2} />
                  <rect x={16} y={18} width={40} height={10} rx={5} fill={tag} />
                  <rect x={16} y={42} width={120} height={9} rx={4.5} fill="#ffffff22" />
                  <rect x={16} y={62} width={80} height={9} rx={4.5} fill="#ffffff16" />
                </g>
              )
            })}
          </g>
        )
      })}
      {/* hero card */}
      <g transform={`translate(${heroX} ${heroY}) rotate(${lift * -6} 85 46) scale(${pop(frame, fps, 10, 10)})`}>
        <rect width={colW - 30} height={cardH} rx={16} fill={C.surface2} stroke={C.emerald} strokeWidth={4} filter="url(#glow)" />
        <rect x={16} y={18} width={40} height={10} rx={5} fill={C.emerald} />
        <rect x={16} y={42} width={120} height={9} rx={4.5} fill="#ffffff44" />
        <rect x={16} y={62} width={80} height={9} rx={4.5} fill="#ffffff2a" />
      </g>
      {/* landing triggers: an agent spark bursts out of the card */}
      {hops.map((h, i) => {
        const land = h.at + 10
        const t = interpolate(frame, [land, land + 18], [0, 1], clamp)
        if (t <= 0 || t >= 1) return null
        const cx = colX[h.to] + colW / 2
        const cy = slotY(0) + cardH / 2
        const icon: keyof typeof ICONS = i === 2 ? 'check' : 'gear'
        return (
          <g key={i}>
            <Ring cx={cx} cy={cy} r0={40} r1={170} t={t} color={i === 2 ? C.emerald : C.amber} width={8} />
            <g transform={`translate(${cx} ${cy - 40 - t * 110}) scale(${1 - t * 0.3})`} opacity={1 - t * t}>
              <circle r={42} fill={C.bg2} stroke={i === 2 ? C.emerald : C.amber} strokeWidth={5} />
              {i === 2 ? <Icon name={icon} x={0} y={0} size={44} color={C.emerald} width={3} /> : <Spark x={0} y={0} r={24} rot={frame * 6} fill={C.amber} />}
            </g>
          </g>
        )
      })}
    </SceneFrame>
  )
}

/** Physical workspace isolation: one block splits into four themed, sealed tiles. */
export const Workspaces: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const appear = pop(frame, fps, 0, 12)
  const split = ease(frame, 20, 34)
  const colors = [C.violet, C.emerald, C.rose, C.blue]
  const tile = 330
  const gapPx = split * 90
  return (
    <SceneFrame duration={duration} word="Isolated" accent={C.amber}>
      <Defs />
      <g transform={`translate(500 500) scale(${appear}) rotate(${(1 - appear) * -20})`}>
        {colors.map((c, i) => {
          const gx = i % 2 === 0 ? -1 : 1
          const gy = i < 2 ? -1 : 1
          const x = gx * (gapPx / 2) + (gx < 0 ? -tile : 0)
          const y = gy * (gapPx / 2) + (gy < 0 ? -tile : 0)
          const float = Math.sin(frame / 10 + i) * 6 * split
          const tint = interpolate(split, [0, 1], [0, 1])
          return (
            <g key={i} transform={`translate(${x} ${y + float})`}>
              <rect width={tile} height={tile} rx={34 * split + 4} fill={C.surface} stroke={tint > 0.5 ? c : C.border} strokeWidth={5} filter={tint > 0.5 ? 'url(#glow)' : undefined} />
              {/* mini app skeleton */}
              <rect x={18} y={18} width={58} height={tile - 36} rx={14} fill={C.surface2} />
              {[0, 1, 2, 3].map((k) => (
                <circle key={k} cx={47} cy={52 + k * 46} r={11} fill={k === 0 ? c : '#ffffff22'} opacity={k === 0 ? tint : 1} />
              ))}
              <rect x={96} y={30} width={150} height={16} rx={8} fill={c} opacity={0.25 + tint * 0.75} />
              <rect x={96} y={66} width={200} height={12} rx={6} fill="#ffffff1c" />
              <rect x={96} y={94} width={170} height={12} rx={6} fill="#ffffff14" />
              {/* sealed contents: particles bounce off the walls */}
              {new Array(5).fill(0).map((_, k) => {
                const vx = 2 + random(`vx${i}${k}`) * 4
                const vy = 2 + random(`vy${i}${k}`) * 4
                const inner = { x0: 96, y0: 130, w: 210, h: 180 }
                const tri = (v: number, len: number) => {
                  const m = v % (2 * len)
                  return m < len ? m : 2 * len - m
                }
                const px = inner.x0 + tri(random(`px${i}${k}`) * 400 + frame * vx, inner.w)
                const py = inner.y0 + tri(random(`py${i}${k}`) * 400 + frame * vy, inner.h)
                return <circle key={k} cx={px} cy={py} r={9} fill={c} opacity={split} />
              })}
            </g>
          )
        })}
        {/* lock in the centre seals the boundary */}
        <g transform={`scale(${pop(frame, fps, 34, 9)})`}>
          <circle r={58} fill={C.bg2} stroke={C.amber} strokeWidth={6} />
          <rect x={-22} y={-6} width={44} height={34} rx={8} fill={C.amber} />
          <path d={`M-13 -6 V-${18 + (1 - ease(frame, 40, 46)) * 12} a13 13 0 0 1 26 0 V-6`} stroke={C.amber} strokeWidth={7} fill="none" />
        </g>
      </g>
      <Ring cx={500} cy={500} r0={60} r1={480} t={interpolate(frame, [44, 64], [0, 1], clamp)} color={C.amber} width={8} />
    </SceneFrame>
  )
}

/** Tools and MCP: icons dock onto a hub; signals pulse along the cables. */
export const Tools: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const names: (keyof typeof ICONS)[] = ['file', 'terminal', 'search', 'globe', 'plug', 'code', 'branch', 'clock']
  const colors = [C.orange, C.cyan, C.violet, C.blue, C.emerald, C.rose, C.amber, C.fuchsia]
  const rot = frame * 0.4
  const hub = pop(frame, fps, 0, 10)
  const hexPath = new Array(6)
    .fill(0)
    .map((_, i) => {
      const p = polar(0, 0, 120, i * 60 + 30)
      return `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`
    })
    .join(' ')
  return (
    <SceneFrame duration={duration} word="Connect" accent={C.orange}>
      <Defs />
      <circle cx={500} cy={500} r={340} fill="none" stroke={C.border} strokeWidth={3} strokeDasharray="6 16" transform={`rotate(${-frame} 500 500)`} />
      {names.map((n, i) => {
        const t = pop(frame, fps, 4 + i * 3, 12)
        const r = interpolate(t, [0, 1], [760, 340])
        const p = polar(500, 500, r, i * 45 + rot)
        const cable = ease(frame, 14 + i * 3, 26 + i * 3)
        const inner = polar(500, 500, 125, i * 45 + rot)
        const end = { x: inner.x + (p.x - inner.x) * cable, y: inner.y + (p.y - inner.y) * cable }
        const sig = ((frame - 30 - i * 4) / 16) % 1
        const dir = i % 2 === 0 ? sig : 1 - sig
        return (
          <g key={n}>
            <line x1={inner.x} y1={inner.y} x2={end.x} y2={end.y} stroke={`${colors[i]}aa`} strokeWidth={6} strokeLinecap="round" />
            {frame > 30 + i * 4 ? (
              <circle cx={inner.x + (p.x - inner.x) * dir} cy={inner.y + (p.y - inner.y) * dir} r={10} fill={colors[i]} filter="url(#glow)" />
            ) : null}
            <g transform={`translate(${p.x} ${p.y}) rotate(${(1 - t) * 180}) scale(${0.4 + t * 0.6 + (cable >= 1 ? pulse(frame + i * 2) * 0.06 : 0)})`}>
              <circle r={70} fill={C.surface} stroke={colors[i]} strokeWidth={6} filter="url(#glow)" />
              <Icon name={n} x={0} y={0} size={64} color="#fff" width={2} />
            </g>
          </g>
        )
      })}
      <g transform={`translate(500 500) scale(${hub * (1 + pulse(frame) * 0.07)}) rotate(${frame * 0.6})`}>
        <path d={hexPath} fill={C.surface2} stroke={C.orange} strokeWidth={8} filter="url(#glow)" />
      </g>
      <g transform={`scale(${hub})`} style={{ transformOrigin: '500px 500px' }}>
        <LogoBadge cx={500} cy={500} size={110} />
      </g>
    </SceneFrame>
  )
}
