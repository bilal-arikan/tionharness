import React from 'react'
import { interpolate, random, useCurrentFrame, useVideoConfig } from 'remotion'
import { SceneFrame } from '../components/Layout'
import { Defs, LogoBadge, Ring } from '../components/Shapes'
import { C, PALETTE, clamp, ease, polar, pop, pulse } from '../theme'

type P = { duration: number }

const SHIELD = 'M500 170 L740 250 V470 C740 640 630 750 500 810 C370 750 260 640 260 470 V250 Z'

/** Permission layer: shield seals, requests are let through or bounced; three mode switches flip. */
export const Control: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const draw = ease(frame, 0, 20)
  const fill = ease(frame, 14, 30)
  const lockClose = ease(frame, 28, 33)
  const click = interpolate(frame, [33, 50], [0, 1], clamp)
  const modes = [
    { color: C.emerald, at: 36 },
    { color: C.amber, at: 50 },
    { color: C.blue, at: 64 },
  ]
  return (
    <SceneFrame duration={duration} word="Control" accent={C.lime}>
      <Defs />
      {/* incoming tool calls */}
      {new Array(9).fill(0).map((_, i) => {
        const start = 26 + i * 6
        const t = interpolate(frame, [start, start + 18], [0, 1], clamp)
        if (t <= 0 || t >= 1) return null
        const ok = random(`ok${i}`) > 0.4
        const side = i % 2 === 0 ? -1 : 1
        const y = 300 + random(`y${i}`) * 380
        const x0 = 500 + side * 560
        const hitX = 500 + side * 250
        let x: number
        let op = 1
        if (ok) {
          x = interpolate(t, [0, 1], [x0, 500])
          op = interpolate(t, [0.7, 1], [1, 0], clamp)
        } else {
          x = t < 0.55 ? interpolate(t, [0, 0.55], [x0, hitX]) : interpolate(t, [0.55, 1], [hitX, hitX + side * 160])
          op = interpolate(t, [0.55, 1], [1, 0], clamp)
        }
        const c = ok ? C.emerald : C.danger
        return (
          <g key={i} opacity={op}>
            <circle cx={x} cy={y} r={16} fill={c} filter="url(#glow)" />
            {!ok && t > 0.55 ? (
              <path d={`M${hitX - 22} ${y - 22} l44 44 M${hitX + 22} ${y - 22} l-44 44`} stroke={C.danger} strokeWidth={8} strokeLinecap="round" />
            ) : null}
          </g>
        )
      })}
      <path d={SHIELD} fill={`${C.lime}22`} opacity={fill} filter="url(#glowBig)" />
      <path d={SHIELD} fill={C.surface} opacity={fill} />
      <path d={SHIELD} fill="none" stroke={C.lime} strokeWidth={12} strokeLinejoin="round" pathLength={1} strokeDasharray="1 1" strokeDashoffset={1 - draw} />
      {/* lock */}
      <g transform={`translate(500 500) scale(${pop(frame, fps, 16, 9) * (1 + click * (1 - click) * 0.6)})`}>
        <path d={`M-48 -10 V${-60 - (1 - lockClose) * 40} a48 48 0 0 1 96 0 V-10`} stroke="#fff" strokeWidth={20} fill="none" strokeLinecap="round" />
        <rect x={-80} y={-20} width={160} height={130} rx={26} fill={C.lime} />
        <circle cx={0} cy={36} r={16} fill={C.bg2} />
        <rect x={-6} y={40} width={12} height={34} rx={6} fill={C.bg2} />
      </g>
      <Ring cx={500} cy={500} r0={120} r1={420} t={click} color={C.lime} width={10} />
      {/* auto / ask / read-only switches */}
      {modes.map((m, i) => {
        const x = 250 + i * 250
        const s = pop(frame, fps, 24 + i * 3, 11)
        const on = ease(frame, m.at, m.at + 6)
        return (
          <g key={i} transform={`translate(${x} 900) scale(${s})`}>
            <rect x={-80} y={-34} width={160} height={68} rx={34} fill={on > 0.5 ? m.color : C.surface2} stroke={m.color} strokeWidth={4} filter={on > 0.5 ? 'url(#glow)' : undefined} />
            <circle cx={-46 + on * 92} cy={0} r={24} fill="#fff" />
          </g>
        )
      })}
    </SceneFrame>
  )
}

/** Any model: twelve provider orbs wired into one runtime, a wave of activity runs around. */
export const Models: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const n = 12
  const rot = frame * 0.6
  const active = Math.floor(frame / 4) % n
  return (
    <SceneFrame duration={duration} word="Any model" accent={C.fuchsia}>
      <Defs />
      <circle cx={500} cy={500} r={430} fill="none" stroke={`${C.fuchsia}44`} strokeWidth={3} strokeDasharray="2 18" transform={`rotate(${-frame * 1.5} 500 500)`} />
      {new Array(n).fill(0).map((_, i) => {
        const p = polar(500, 500, 360, (i * 360) / n + rot)
        const s = pop(frame, fps, i * 2, 10)
        const c = PALETTE[i % PALETTE.length]
        const wire = ease(frame, 10 + i, 22 + i)
        const isActive = frame > 24 && i === active
        const sig = ((frame % 4) + 1) / 4
        return (
          <g key={i}>
            <line x1={500} y1={500} x2={500 + (p.x - 500) * wire} y2={500 + (p.y - 500) * wire} stroke={isActive ? c : `${c}44`} strokeWidth={isActive ? 7 : 3} />
            {isActive ? <circle cx={p.x + (500 - p.x) * sig} cy={p.y + (500 - p.y) * sig} r={12} fill={c} filter="url(#glow)" /> : null}
            <g transform={`translate(${p.x} ${p.y}) scale(${s * (isActive ? 1.25 : 1)})`}>
              <circle r={52} fill={`${c}2a`} stroke={c} strokeWidth={5} filter={isActive ? 'url(#glow)' : undefined} />
              <circle r={22} fill={c} />
              <circle r={36} fill="none" stroke={c} strokeWidth={3} strokeDasharray="10 8" transform={`rotate(${frame * 5 * (i % 2 ? 1 : -1)})`} />
            </g>
          </g>
        )
      })}
      {/* runtime chip */}
      <g transform={`translate(500 500) scale(${pop(frame, fps, 0, 10) * (1 + pulse(frame) * 0.06)})`}>
        {new Array(4).fill(0).map((_, side) =>
          [-50, -17, 17, 50].map((o) => (
            <rect key={`${side}${o}`} x={o - 7} y={-150} width={14} height={36} rx={4} fill={C.dim} transform={`rotate(${side * 90})`} />
          )),
        )}
        <rect x={-122} y={-122} width={244} height={244} rx={40} fill={C.surface} stroke={C.fuchsia} strokeWidth={6} filter="url(#glow)" />
      </g>
      <g transform={`scale(${pop(frame, fps, 0, 10)})`} style={{ transformOrigin: '500px 500px' }}>
        <LogoBadge cx={500} cy={500} size={150} />
      </g>
    </SceneFrame>
  )
}
