import React from 'react'
import { AbsoluteFill, interpolate, random, useCurrentFrame, useVideoConfig } from 'remotion'
import { KineticWord, useOrientation } from '../components/Layout'
import { Defs, LOGO_RECTS, Ring, Spark } from '../components/Shapes'
import { C, PALETTE, clamp, ease, easeIn, pop } from '../theme'

/** 8 beats: logo assembles from its own rects, bursts, then zooms through the camera into the drop. */
export const Intro: React.FC<{ duration: number }> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps, width, height } = useVideoConfig()
  const o = useOrientation()
  const S = Math.min(width, height) * (o === 'landscape' ? 0.95 : 0.9)

  const dot = pop(frame, fps, 0, 9)
  const badge = pop(frame, fps, 12, 10, 140)
  const burstT = interpolate(frame, [58, 90], [0, 1], clamp)
  const charge = interpolate(frame, [88, 112], [0, 1], clamp)
  const zoom = easeIn(frame, 108, duration, [1, 9])
  const zoomFade = interpolate(frame, [112, duration], [1, 0], clamp)
  const jitter = charge * 4
  const jx = Math.sin(frame * 3.1) * jitter
  const jy = Math.cos(frame * 2.7) * jitter

  const k = 300 / 48
  const rot = interpolate(badge, [0, 1], [-120, 0])

  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          width: S,
          height: S,
          left: (width - S) / 2 + jx,
          top: (height - S) / 2 - (o === 'portrait' ? height * 0.06 : height * 0.04) + jy,
          transform: `scale(${zoom})`,
          opacity: zoomFade,
        }}
      >
        <svg viewBox="0 0 1000 1000" width="100%" height="100%" style={{ overflow: 'visible' }}>
          <Defs />
          {/* seed dot and its ripples */}
          <circle cx={500} cy={500} r={14 * dot * (1 - badge)} fill={C.brand} filter="url(#glow)" />
          <Ring cx={500} cy={500} r0={10} r1={260} t={interpolate(frame, [2, 26], [0, 1], clamp)} color={C.violet} width={8} />
          <Ring cx={500} cy={500} r0={10} r1={360} t={interpolate(frame, [8, 34], [0, 1], clamp)} color={C.cyan} width={5} />

          {/* orbiting sparks that get sucked in while charging */}
          {new Array(10).fill(0).map((_, i) => {
            const a = (i / 10) * Math.PI * 2 + frame / 18
            const r = interpolate(frame, [10, 40, 88, 110], [520, 330, 330, 120], clamp)
            const vis = interpolate(frame, [10 + i, 22 + i], [0, 1], clamp) * zoomFade
            return (
              <Spark
                key={i}
                x={500 + Math.cos(a) * r}
                y={500 + Math.sin(a) * r}
                r={14 + 6 * Math.sin(frame / 5 + i)}
                rot={frame * 4}
                fill={PALETTE[i]}
                opacity={vis}
              />
            )
          })}

          {/* badge */}
          <g transform={`translate(500 500) rotate(${rot}) scale(${badge})`}>
            <rect
              x={-150}
              y={-150}
              width={300}
              height={300}
              rx={11 * k}
              fill={C.brand}
              filter="url(#glowBig)"
              opacity={0.55 + charge * 0.45}
            />
            <rect x={-150} y={-150} width={300} height={300} rx={11 * k} fill={C.brand} />
            <rect x={-150} y={-150} width={300} height={300} rx={11 * k} fill="url(#introSheen)" />
            {LOGO_RECTS.map((r, i) => {
              const t = pop(frame, fps, 28 + i * 5, 12, 200)
              const ang = random(`la${i}`) * Math.PI * 2
              const dist = (1 - t) * 420
              const x = (r.x - 24) * k + Math.cos(ang) * dist
              const y = (r.y - 24) * k + Math.sin(ang) * dist
              return (
                <rect
                  key={i}
                  x={x}
                  y={y}
                  width={r.w * k}
                  height={r.h * k}
                  rx={k}
                  fill="#fff"
                  opacity={interpolate(t, [0, 0.2], [0, 1], clamp)}
                  transform={`rotate(${(1 - t) * 160} ${x + (r.w * k) / 2} ${y + (r.h * k) / 2})`}
                />
              )
            })}
          </g>
          <defs>
            <linearGradient id="introSheen" x1="0" y1="0" x2="1" y2="1">
              <stop offset={Math.max(0, ease(frame, 60, 80) - 0.15)} stopColor="#fff" stopOpacity={0} />
              <stop offset={ease(frame, 60, 80)} stopColor="#fff" stopOpacity={0.35} />
              <stop offset={Math.min(1, ease(frame, 60, 80) + 0.15)} stopColor="#fff" stopOpacity={0} />
            </linearGradient>
          </defs>

          {/* burst */}
          <Ring cx={500} cy={500} r0={170} r1={620} t={burstT} color="#fff" width={10} />
          <Ring cx={500} cy={500} r0={170} r1={480} t={interpolate(frame, [62, 86], [0, 1], clamp)} color={C.violet} width={14} />
          {burstT > 0 && burstT < 1
            ? new Array(28).fill(0).map((_, i) => {
                const a = (i / 28) * Math.PI * 2 + random(`ba${i}`) * 0.3
                const d = 180 + ease(frame, 58, 90) * (260 + random(`bd${i}`) * 260)
                return (
                  <circle
                    key={i}
                    cx={500 + Math.cos(a) * d}
                    cy={500 + Math.sin(a) * d}
                    r={4 + random(`br${i}`) * 7}
                    fill={PALETTE[i % PALETTE.length]}
                    opacity={1 - burstT}
                  />
                )
              })
            : null}
        </svg>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 0,
          width,
          top: (height - S) / 2 + S * (o === 'portrait' ? 0.72 : 0.76) - (o === 'portrait' ? height * 0.06 : height * 0.04),
          display: 'flex',
          justifyContent: 'center',
          opacity: zoomFade,
          transform: `scale(${interpolate(charge, [0, 1], [1, 1.06])})`,
        }}
      >
        {frame >= 64 ? (
          <div style={{ position: 'relative' }}>
            <KineticWord text="TionHarness" size={Math.min(width, height) * (o === 'landscape' ? 0.1 : 0.115)} accent={C.brand} duration={1000} delay={64} />
          </div>
        ) : null}
      </div>
    </AbsoluteFill>
  )
}
