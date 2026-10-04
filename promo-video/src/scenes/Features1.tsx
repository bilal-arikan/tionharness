import React from 'react'
import { interpolate, useCurrentFrame, useVideoConfig } from 'remotion'
import { SceneFrame } from '../components/Layout'
import { Defs, Icon, Ring, Spark } from '../components/Shapes'
import { C, clamp, ease, polar, pop, pulse, qPoint } from '../theme'

type P = { duration: number }

/** Agents with an identity: five avatars, each with its own colour, orbit and thinking meter. */
export const Agents: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const colors = [C.violet, C.cyan, C.rose, C.amber, C.emerald]
  const levels = [3, 5, 2, 4, 1]
  const spin = frame * 0.5
  const pts = colors.map((_, i) => polar(500, 500, 300, i * 72 + spin))
  const hub = pop(frame, fps, 2)
  return (
    <SceneFrame duration={duration} word="Agents" accent={C.violet}>
      <Defs />
      {/* links between agents */}
      {pts.map((p, i) => {
        const q = pts[(i + 1) % pts.length]
        const t = ease(frame, 26 + i * 3, 40 + i * 3)
        const flow = (frame / 18 + i * 0.2) % 1
        return (
          <g key={`l${i}`}>
            <line x1={p.x} y1={p.y} x2={p.x + (q.x - p.x) * t} y2={p.y + (q.y - p.y) * t} stroke={C.border} strokeWidth={4} />
            <line x1={500} y1={500} x2={500 + (p.x - 500) * t} y2={500 + (p.y - 500) * t} stroke={`${colors[i]}55`} strokeWidth={3} strokeDasharray="8 10" strokeDashoffset={-frame * 2} />
            {t >= 1 ? <circle cx={p.x + (q.x - p.x) * flow} cy={p.y + (q.y - p.y) * flow} r={7} fill={colors[i]} filter="url(#glow)" /> : null}
          </g>
        )
      })}
      {/* hub */}
      <g transform={`translate(500 500) scale(${hub * (1 + pulse(frame) * 0.08)})`}>
        <circle r={70} fill={C.surface} stroke={C.brand} strokeWidth={5} />
        <Spark x={0} y={0} r={34} rot={frame * 3} fill={C.brand} />
      </g>
      {pts.map((p, i) => {
        const s = pop(frame, fps, 4 + i * 4, 9)
        const c = colors[i]
        const lit = Math.floor(interpolate(frame, [30 + i * 4, 60 + i * 4], [0, levels[i]], clamp))
        return (
          <g key={i} transform={`translate(${p.x} ${p.y}) scale(${s})`}>
            <circle r={98} fill="none" stroke={`${c}66`} strokeWidth={3} strokeDasharray="4 14" transform={`rotate(${frame * (i % 2 ? 3 : -3)})`} />
            <circle r={78} fill={`${c}22`} stroke={c} strokeWidth={6} filter="url(#glow)" />
            <circle r={60} fill={C.surface} />
            <Spark x={0} y={0} r={30 + 6 * pulse(frame + i * 3)} rot={frame * 2 + i * 20} fill={c} />
            {(() => {
              const sat = polar(0, 0, 98, frame * 6 + i * 60)
              return <circle cx={sat.x} cy={sat.y} r={9} fill={c} />
            })()}
            {/* thinking-level meter */}
            {new Array(5).fill(0).map((_, k) => (
              <rect key={k} x={-62 + k * 26} y={110} width={20} height={14} rx={4} fill={k < lit ? c : C.surface2} opacity={k < lit ? 1 : 0.8} />
            ))}
          </g>
        )
      })}
    </SceneFrame>
  )
}

/** Coordinator and workers: a tree grows, results stream back up to the root. */
export const Delegate: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const root = { x: 500, y: 170 }
  const kids = [
    { x: 200, y: 480 },
    { x: 500, y: 480 },
    { x: 800, y: 480 },
  ]
  const leaves = kids.flatMap((k, i) => [
    { x: k.x - 95, y: 800, parent: i },
    { x: k.x + 95, y: 800, parent: i },
  ])
  const ctrl = (a: { x: number; y: number }, b: { x: number; y: number }) => ({ x: b.x, y: (a.y + b.y) / 2 - 40 })

  // Results travel leaf -> kid -> root, staggered per leaf.
  const arrivals = leaves.map((_, i) => 46 + i * 5 + 16)
  const rootHit = arrivals.reduce((acc, a) => (frame >= a && frame - a < 14 ? Math.max(acc, 1 - (frame - a) / 14) : acc), 0)

  return (
    <SceneFrame duration={duration} word="Delegate" accent={C.cyan}>
      <Defs />
      {kids.map((k, i) => {
        const t = ease(frame, 8 + i * 2, 20 + i * 2)
        const c1 = ctrl(root, k)
        return (
          <path
            key={`e${i}`}
            d={`M${root.x} ${root.y} Q${c1.x} ${c1.y} ${k.x} ${k.y}`}
            stroke={C.cyan}
            strokeOpacity={0.55}
            strokeWidth={6}
            fill="none"
            pathLength={1}
            strokeDasharray="1 1"
            strokeDashoffset={1 - t}
          />
        )
      })}
      {leaves.map((l, i) => {
        const k = kids[l.parent]
        const t = ease(frame, 22 + i * 2, 34 + i * 2)
        const c1 = ctrl(k, l)
        return (
          <path
            key={`f${i}`}
            d={`M${k.x} ${k.y} Q${c1.x} ${c1.y} ${l.x} ${l.y}`}
            stroke={C.cyan}
            strokeOpacity={0.4}
            strokeWidth={4}
            fill="none"
            pathLength={1}
            strokeDasharray="1 1"
            strokeDashoffset={1 - t}
          />
        )
      })}
      {/* result packets */}
      {leaves.map((l, i) => {
        const k = kids[l.parent]
        const start = 46 + i * 5
        const a = interpolate(frame, [start, start + 8], [0, 1], clamp)
        const b = interpolate(frame, [start + 8, start + 16], [0, 1], clamp)
        if (frame < start || frame > start + 16) return null
        const p = a < 1 ? qPoint(l, ctrl(k, l), k, a) : qPoint(k, ctrl(root, k), root, b)
        return <circle key={`p${i}`} cx={p.x} cy={p.y} r={13} fill={C.emerald} filter="url(#glow)" />
      })}
      {leaves.map((l, i) => {
        const s = pop(frame, fps, 30 + i * 2, 9)
        const done = frame >= 46 + i * 5
        return (
          <g key={`n${i}`} transform={`translate(${l.x} ${l.y}) scale(${s})`}>
            <circle r={44} fill={C.surface} stroke={done ? C.emerald : C.cyan} strokeWidth={5} />
            {done ? (
              <Icon name="check" x={0} y={0} size={46} color={C.emerald} width={3} />
            ) : (
              <Spark x={0} y={0} r={22} rot={frame * 5} fill={C.cyan} />
            )}
            <Ring cx={0} cy={0} r0={44} r1={90} t={interpolate(frame, [46 + i * 5, 58 + i * 5], [0, 1], clamp)} color={C.emerald} width={5} />
          </g>
        )
      })}
      {kids.map((k, i) => {
        const s = pop(frame, fps, 16 + i * 2, 9)
        return (
          <g key={`k${i}`} transform={`translate(${k.x} ${k.y}) scale(${s})`}>
            <circle r={62} fill={C.surface} stroke={C.cyan} strokeWidth={6} filter="url(#glow)" />
            <Spark x={0} y={0} r={30} rot={-frame * 4} fill={C.cyan} />
          </g>
        )
      })}
      <g transform={`translate(${root.x} ${root.y}) scale(${pop(frame, fps, 2, 9) * (1 + rootHit * 0.15)})`}>
        <circle r={84 + rootHit * 20} fill={`${C.brand}33`} filter="url(#glowBig)" />
        <circle r={84} fill={C.surface} stroke={C.brand} strokeWidth={7} />
        <Spark x={0} y={0} r={44} rot={frame * 3} fill={C.brand} />
        {/* coordinator crown */}
        <path d="M-40 -110 L-26 -132 L-12 -112 L0 -138 L12 -112 L26 -132 L40 -110 Z" fill={C.amber} opacity={ease(frame, 6, 14)} />
      </g>
    </SceneFrame>
  )
}

/** Visual flows: start -> agent -> branch -> parallel -> merge (loop) -> end, with packets flowing. */
export const Flows: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const N = {
    start: { x: 80, y: 500 },
    a: { x: 240, y: 500 },
    br: { x: 410, y: 500 },
    b: { x: 590, y: 300 },
    c: { x: 590, y: 700 },
    m: { x: 760, y: 500 },
    end: { x: 920, y: 500 },
  }
  type E = { from: keyof typeof N; to: keyof typeof N; at: number; ctrl?: { x: number; y: number } }
  const edges: E[] = [
    { from: 'start', to: 'a', at: 4 },
    { from: 'a', to: 'br', at: 10 },
    { from: 'br', to: 'b', at: 16, ctrl: { x: 450, y: 300 } },
    { from: 'br', to: 'c', at: 16, ctrl: { x: 450, y: 700 } },
    { from: 'b', to: 'm', at: 24, ctrl: { x: 720, y: 300 } },
    { from: 'c', to: 'm', at: 24, ctrl: { x: 720, y: 700 } },
    { from: 'm', to: 'end', at: 32 },
    { from: 'm', to: 'a', at: 36, ctrl: { x: 500, y: 1000 } },
  ]
  const nodeAt: Record<keyof typeof N, number> = { start: 0, a: 8, br: 14, b: 22, c: 22, m: 30, end: 36 }
  const mid = (p: { x: number; y: number }, q: { x: number; y: number }) => ({ x: (p.x + q.x) / 2, y: (p.y + q.y) / 2 })
  return (
    <SceneFrame duration={duration} word="Flows" accent={C.blue}>
      <Defs />
      {edges.map((e, i) => {
        const p = N[e.from]
        const q = N[e.to]
        const c = e.ctrl ?? mid(p, q)
        const t = ease(frame, e.at, e.at + 10)
        const loop = e.to === 'a' && e.from === 'm'
        return (
          <g key={i}>
            <path
              d={`M${p.x} ${p.y} Q${c.x} ${c.y} ${q.x} ${q.y}`}
              stroke={loop ? C.amber : C.blue}
              strokeOpacity={0.6}
              strokeWidth={6}
              strokeDasharray={loop ? undefined : undefined}
              fill="none"
              pathLength={1}
              style={{ strokeDasharray: '1 1', strokeDashoffset: 1 - t }}
            />
            {t >= 1
              ? [0, 0.5].map((off) => {
                  const u = ((frame - e.at) / 22 + off) % 1
                  const pt = qPoint(p, c, q, u)
                  return <circle key={off} cx={pt.x} cy={pt.y} r={9} fill={loop ? C.amber : '#fff'} filter="url(#glow)" />
                })
              : null}
          </g>
        )
      })}
      {(Object.keys(N) as (keyof typeof N)[]).map((k) => {
        const n = N[k]
        const s = pop(frame, fps, nodeAt[k], 9)
        const hit = pulse(frame + (k === 'm' ? 7 : 0), 6)
        let body: React.ReactNode
        if (k === 'start' || k === 'end') {
          body = (
            <>
              <circle r={42} fill={k === 'end' ? C.emerald : C.blue} />
              {k === 'end' ? <Icon name="check" x={0} y={0} size={44} color="#0b0b0f" width={3.5} /> : <path d="M-10 -16 L18 0 L-10 16 Z" fill="#0b0b0f" />}
            </>
          )
        } else if (k === 'br') {
          body = (
            <>
              <rect x={-52} y={-52} width={104} height={104} rx={14} transform="rotate(45)" fill={C.surface} stroke={C.amber} strokeWidth={6} />
              <Icon name="branch" x={0} y={0} size={46} color={C.amber} width={2.4} />
            </>
          )
        } else if (k === 'm') {
          body = (
            <>
              <rect x={-60} y={-50} width={120} height={100} rx={22} fill={C.surface} stroke={C.blue} strokeWidth={6} />
              <path d="M-26 -20 L0 0 L-26 20 M0 0 H28" stroke="#fff" strokeWidth={6} fill="none" strokeLinecap="round" strokeLinejoin="round" />
            </>
          )
        } else {
          body = (
            <>
              <rect x={-62} y={-50} width={124} height={100} rx={22} fill={C.surface} stroke={C.blue} strokeWidth={6} />
              <Spark x={0} y={0} r={28} rot={frame * 4} fill={C.blue} />
            </>
          )
        }
        return (
          <g key={k} transform={`translate(${n.x} ${n.y}) scale(${s * (1 + hit * 0.05)})`} filter="url(#glow)">
            {body}
          </g>
        )
      })}
    </SceneFrame>
  )
}
