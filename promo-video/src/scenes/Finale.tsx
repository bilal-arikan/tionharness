import React from 'react'
import { AbsoluteFill, interpolate, random, spring, useCurrentFrame, useVideoConfig } from 'remotion'
import { KineticWord, shake, useOrientation } from '../components/Layout'
import { Defs, ICONS, Icon, LogoBadge, Ring, Spark } from '../components/Shapes'
import { BEAT, C, FONT, PALETTE, clamp, ease, pulse } from '../theme'

type P = { duration: number }

const STATS: { value: string; label: string; icon: keyof typeof ICONS; color: string; strike: boolean }[] = [
  { value: '1', label: 'binary', icon: 'cube', color: C.violet, strike: false },
  { value: '0', label: 'databases', icon: 'db', color: C.cyan, strike: true },
  { value: '0', label: 'API keys', icon: 'key', color: C.emerald, strike: true },
]

/** Three numbers slam in on alternate beats. */
export const Stats: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps, width, height } = useVideoConfig()
  const o = useOrientation()
  const exit = interpolate(frame, [duration - 6, duration], [0, 1], clamp)
  const hits = STATS.map((_, i) => i * BEAT * 2)
  const sh = hits.reduce((acc, h) => {
    const s = shake(frame, h, 16)
    return { x: acc.x + s.x, y: acc.y + s.y }
  }, { x: 0, y: 0 })

  const u = o === 'landscape' ? height * 0.2 : o === 'portrait' ? width * 0.22 : height * 0.17
  const row = o !== 'landscape'

  return (
    <AbsoluteFill
      style={{
        display: 'flex',
        flexDirection: row ? 'column' : 'row',
        alignItems: 'center',
        justifyContent: 'center',
        gap: row ? u * 0.35 : u * 0.9,
        transform: `translate(${sh.x}px, ${sh.y}px) scale(${1 - exit * 0.15})`,
        opacity: 1 - exit,
        filter: `blur(${exit * 10}px)`,
      }}
    >
      {STATS.map((s, i) => {
        const t = spring({ frame: frame - hits[i], fps, config: { damping: 12, stiffness: 220, mass: 0.6 } })
        const vis = frame >= hits[i]
        const scale = interpolate(t, [0, 1], [2.8, 1])
        const beat = pulse(frame, 6) * 0.04
        const strike = ease(frame, hits[i] + 8, hits[i] + 16)
        return (
          <div
            key={i}
            style={{
              display: 'flex',
              flexDirection: row ? 'row' : 'column',
              alignItems: 'center',
              gap: u * 0.3,
              width: row ? u * 3.6 : undefined,
              opacity: vis ? interpolate(t, [0, 0.3], [0, 1], clamp) : 0,
              transform: `scale(${scale + beat})`,
              filter: `blur(${(1 - Math.min(1, t)) * 8}px)`,
            }}
          >
            <svg width={u * 1.15} height={u * 1.15} viewBox="0 0 100 100" style={{ overflow: 'visible', flexShrink: 0 }}>
              <Defs />
              <circle cx={50} cy={50} r={46} fill={`${s.color}22`} stroke={s.color} strokeWidth={3} />
              <Icon name={s.icon} x={50} y={50} size={52} color="#fff" width={1.8} />
              {s.strike ? (
                <line x1={20} y1={80} x2={20 + 60 * strike} y2={80 - 60 * strike} stroke={C.danger} strokeWidth={7} strokeLinecap="round" />
              ) : null}
              <Ring cx={50} cy={50} r0={46} r1={95} t={interpolate(frame, [hits[i], hits[i] + 16], [0, 1], clamp)} color={s.color} width={4} />
            </svg>
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: row ? 'flex-start' : 'center' }}>
              <div
                style={{
                  fontFamily: FONT,
                  fontWeight: 900,
                  fontSize: u * 1.05,
                  lineHeight: 0.95,
                  color: C.text,
                  textShadow: `0 0 ${u * 0.4}px ${s.color}88`,
                }}
              >
                {s.value}
              </div>
              <div
                style={{
                  fontFamily: FONT,
                  fontWeight: 600,
                  fontSize: u * 0.26,
                  color: s.color,
                  letterSpacing: u * 0.01,
                  textTransform: 'uppercase',
                }}
              >
                {s.label}
              </div>
            </div>
          </div>
        )
      })}
    </AbsoluteFill>
  )
}

/** Ten theme accents flash past on the snare roll; a mini app re-skins itself each time. */
export const Themes: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { width, height } = useVideoConfig()
  const step = duration / PALETTE.length
  const idx = Math.min(PALETTE.length - 1, Math.floor(frame / step))
  const local = (frame - idx * step) / step
  const accent = PALETTE[idx]
  const S = Math.min(width, height) * 0.62
  const enter = ease(frame, 0, 8)
  const exit = ease(frame, duration - 8, duration)
  const diag = Math.hypot(width, height)
  return (
    <AbsoluteFill style={{ overflow: 'hidden' }}>
      {/* sweeping stripes in the current and previous colour */}
      {[0, 1, 2].map((k) => {
        const c = PALETTE[(idx + PALETTE.length - k) % PALETTE.length]
        const x = interpolate(local, [0, 1], [-diag, diag]) - k * diag * 0.35
        return (
          <div
            key={k}
            style={{
              position: 'absolute',
              left: width / 2 - diag / 2 + x,
              top: height / 2 - diag,
              width: diag * 0.18,
              height: diag * 2,
              background: c,
              opacity: 0.22 - k * 0.06,
              transform: 'rotate(25deg)',
            }}
          />
        )
      })}
      <div
        style={{
          position: 'absolute',
          left: (width - S * 1.25) / 2,
          top: (height - S) / 2 - S * 0.06,
          width: S * 1.25,
          height: S,
          transform: `perspective(1600px) rotateY(${Math.sin(frame / 9) * 14}deg) rotateX(${8 - enter * 4}deg) scale(${0.7 + enter * 0.3 + exit * 0.5})`,
          opacity: 1 - exit,
        }}
      >
        <svg viewBox="0 0 1250 1000" width="100%" height="100%" style={{ overflow: 'visible' }}>
          <Defs />
          <rect x={0} y={0} width={1250} height={1000} rx={48} fill={C.bg2} stroke={accent} strokeWidth={6} filter="url(#glow)" />
          <rect x={0} y={0} width={1250} height={80} rx={48} fill={C.surface} />
          <rect x={0} y={40} width={1250} height={40} fill={C.surface} />
          {[C.rose, C.amber, C.emerald].map((c, i) => (
            <circle key={i} cx={50 + i * 40} cy={40} r={12} fill={c} />
          ))}
          <rect x={30} y={110} width={110} height={860} rx={28} fill={C.surface} />
          <LogoBadge cx={85} cy={170} size={64} />
          {[0, 1, 2, 3, 4].map((k) => (
            <rect key={k} x={60} y={250 + k * 90} width={50} height={50} rx={14} fill={k === 1 ? accent : C.surface2} />
          ))}
          {/* chat */}
          <rect x={700} y={150} width={480} height={110} rx={34} fill={accent} />
          <rect x={740} y={190} width={300} height={16} rx={8} fill="#ffffffbb" />
          <rect x={740} y={220} width={200} height={14} rx={7} fill="#ffffff88" />
          <rect x={190} y={310} width={620} height={250} rx={34} fill={C.surface} />
          <Spark x={240} y={360} r={22} rot={frame * 6} fill={accent} />
          {[0, 1, 2, 3].map((k) => (
            <rect key={k} x={290} y={345 + k * 45} width={460 - k * 70} height={16} rx={8} fill="#ffffff22" />
          ))}
          <rect x={190} y={850} width={1000} height={100} rx={34} fill={C.surface} stroke={C.border} strokeWidth={3} />
          <circle cx={1135} cy={900} r={32} fill={accent} />
          <path d="M1122 900 h26 M1137 888 l12 12 -12 12" stroke="#fff" strokeWidth={6} fill="none" strokeLinecap="round" />
          {PALETTE.map((c, i) => (
            <circle key={i} cx={260 + i * 70} cy={720} r={i === idx ? 26 : 16} fill={c} stroke={i === idx ? '#fff' : 'none'} strokeWidth={5} />
          ))}
        </svg>
      </div>
    </AbsoluteFill>
  )
}

/** Logo slam on the final hit, then the name, a tagline and the address. */
export const Outro: React.FC<P> = ({ duration }) => {
  const frame = useCurrentFrame()
  const { fps, width, height } = useVideoConfig()
  const o = useOrientation()
  const m = Math.min(width, height)
  const slam = spring({ frame, fps, config: { damping: 10, stiffness: 180, mass: 0.8 } })
  const sh = shake(frame, 0, 22)
  const fadeOut = interpolate(frame, [duration - 28, duration - 2], [1, 0], clamp)
  const logo = m * (o === 'landscape' ? 0.3 : 0.34)
  const breathe = 1 + pulse(frame, 5) * 0.03 * (frame < 90 ? 1 : 0)
  const tag = ease(frame, 34, 48)
  const url = ease(frame, 46, 60)
  const cy = height * (o === 'portrait' ? 0.38 : o === 'square' ? 0.34 : 0.33)
  return (
    <AbsoluteFill style={{ opacity: fadeOut, transform: `translate(${sh.x}px, ${sh.y}px)` }}>
      <svg width={width} height={height} style={{ position: 'absolute', inset: 0, overflow: 'visible' }}>
        <Defs />
        <defs>
          <radialGradient id="outroGlow">
            <stop offset="0" stopColor={C.brand} stopOpacity={0.55} />
            <stop offset="1" stopColor={C.brand} stopOpacity={0} />
          </radialGradient>
        </defs>
        {[0, 6, 12].map((d, i) => (
          <Ring
            key={i}
            cx={width / 2}
            cy={cy}
            r0={logo * 0.5}
            r1={m * (0.75 + i * 0.15)}
            t={interpolate(frame, [d, d + 30], [0, 1], clamp)}
            color={[C.brand, '#fff', C.cyan][i]}
            width={m * 0.015}
          />
        ))}
        {new Array(36).fill(0).map((_, i) => {
          const a = (i / 36) * Math.PI * 2
          const d = logo * 0.6 + ease(frame, 0, 40) * m * (0.25 + random(`od${i}`) * 0.45)
          const op = interpolate(frame, [0, 40], [1, 0], clamp)
          return (
            <Spark
              key={i}
              x={width / 2 + Math.cos(a) * d}
              y={cy + Math.sin(a) * d}
              r={m * (0.008 + random(`or${i}`) * 0.012)}
              rot={frame * 8}
              fill={PALETTE[i % PALETTE.length]}
              opacity={op}
            />
          )
        })}
        <g transform={`translate(${width / 2} ${cy}) scale(${interpolate(slam, [0, 1], [3, 1]) * breathe}) translate(${-width / 2} ${-cy})`} opacity={interpolate(slam, [0, 0.2], [0, 1], clamp)}>
          <circle cx={width / 2} cy={cy} r={logo * 1.1} fill="url(#outroGlow)" />
          <LogoBadge cx={width / 2} cy={cy} size={logo} glow />
        </g>
      </svg>
      <div
        style={{
          position: 'absolute',
          top: cy + logo * 0.72,
          width,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          gap: m * 0.03,
        }}
      >
        <KineticWord text="TionHarness" size={m * (o === 'landscape' ? 0.1 : 0.12)} accent={C.brand} duration={10000} delay={8} />
        <div
          style={{
            fontFamily: FONT,
            fontWeight: 500,
            fontSize: m * 0.042,
            color: C.dim,
            opacity: tag,
            transform: `translateY(${(1 - tag) * 30}px)`,
            letterSpacing: m * 0.001,
          }}
        >
          Your agents. Your machine.
        </div>
        <div
          style={{
            marginTop: m * 0.01,
            fontFamily: FONT,
            fontWeight: 700,
            fontSize: m * 0.036,
            color: '#fff',
            padding: `${m * 0.014}px ${m * 0.035}px`,
            borderRadius: m,
            background: `linear-gradient(90deg, ${C.brand}, ${C.fuchsia})`,
            boxShadow: `0 0 ${m * 0.05}px ${C.brand}88`,
            opacity: url,
            transform: `scale(${0.7 + url * 0.3})`,
          }}
        >
          tionharness.com
        </div>
      </div>
    </AbsoluteFill>
  )
}
