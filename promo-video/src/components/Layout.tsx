import React from 'react'
import { AbsoluteFill, interpolate, random, useCurrentFrame, useVideoConfig } from 'remotion'
import { BEAT, C, FONT, clamp, ease, pulse } from '../theme'

export type Orientation = 'landscape' | 'portrait' | 'square'

export const useOrientation = (): Orientation => {
  const { width, height } = useVideoConfig()
  const r = width / height
  if (r > 1.2) return 'landscape'
  if (r < 0.83) return 'portrait'
  return 'square'
}

/** Dark canvas with drifting grid, accent glows and beat-synced breathing. */
export const Background: React.FC<{ accent: string; accent2?: string }> = ({ accent, accent2 = C.brand }) => {
  const frame = useCurrentFrame()
  const { width, height } = useVideoConfig()
  const p = pulse(frame, 4)
  const cell = Math.min(width, height) / 12
  const drift = (frame * 1.2) % cell
  return (
    <AbsoluteFill style={{ background: C.bg, overflow: 'hidden' }}>
      <AbsoluteFill
        style={{
          backgroundImage: `linear-gradient(${C.border}55 1px, transparent 1px), linear-gradient(90deg, ${C.border}55 1px, transparent 1px)`,
          backgroundSize: `${cell}px ${cell}px`,
          backgroundPosition: `${drift}px ${drift * 0.6}px`,
          maskImage: 'radial-gradient(ellipse at center, black 20%, transparent 75%)',
          WebkitMaskImage: 'radial-gradient(ellipse at center, black 20%, transparent 75%)',
          opacity: 0.55 + p * 0.25,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: Math.max(width, height) * 0.9,
          height: Math.max(width, height) * 0.9,
          left: width * 0.62 - Math.max(width, height) * 0.45 + Math.sin(frame / 40) * width * 0.05,
          top: height * 0.4 - Math.max(width, height) * 0.45 + Math.cos(frame / 50) * height * 0.05,
          background: `radial-gradient(circle, ${accent}55 0%, ${accent}00 60%)`,
          transform: `scale(${1 + p * 0.06})`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: Math.max(width, height) * 0.7,
          height: Math.max(width, height) * 0.7,
          left: width * 0.15 - Math.max(width, height) * 0.35 + Math.cos(frame / 35) * width * 0.06,
          top: height * 0.8 - Math.max(width, height) * 0.35,
          background: `radial-gradient(circle, ${accent2}40 0%, ${accent2}00 60%)`,
        }}
      />
      <Dust />
      <AbsoluteFill
        style={{ background: 'radial-gradient(ellipse at center, transparent 55%, rgba(0,0,0,0.65) 100%)' }}
      />
    </AbsoluteFill>
  )
}

const Dust: React.FC = () => {
  const frame = useCurrentFrame()
  const { width, height } = useVideoConfig()
  return (
    <AbsoluteFill>
      {new Array(40).fill(0).map((_, i) => {
        const x = random(`dx${i}`) * width
        const speed = 0.3 + random(`ds${i}`) * 1.2
        const y = (((random(`dy${i}`) * height - frame * speed) % height) + height) % height
        const s = 1.5 + random(`dz${i}`) * 3
        return (
          <div
            key={i}
            style={{
              position: 'absolute',
              left: x,
              top: y,
              width: s,
              height: s,
              borderRadius: s,
              background: '#fff',
              opacity: 0.08 + 0.25 * random(`do${i}`) * (0.5 + 0.5 * Math.sin(frame / 9 + i)),
            }}
          />
        )
      })}
    </AbsoluteFill>
  )
}

/**
 * Places a 1000x1000 SVG artboard and an optional single word so all three
 * formats work from one scene definition.
 */
export const SceneFrame: React.FC<{
  duration: number
  word?: string
  accent: string
  children: React.ReactNode
}> = ({ duration, word, accent, children }) => {
  const frame = useCurrentFrame()
  const { width, height } = useVideoConfig()
  const o = useOrientation()

  const enter = ease(frame, 0, 10)
  const exit = interpolate(frame, [duration - 5, duration], [0, 1], clamp)
  const scale = interpolate(enter, [0, 1], [1.25, 1]) * interpolate(exit, [0, 1], [1, 0.88])
  const blur = (1 - enter) * 14 + exit * 10
  const opacity = enter * (1 - exit)

  let art: React.CSSProperties
  let wordBox: React.CSSProperties
  let fontSize: number
  if (o === 'landscape') {
    const s = height * 0.9
    art = { left: width * 0.69 - s / 2, top: (height - s) / 2, width: s, height: s }
    wordBox = { left: width * 0.065, top: 0, height, width: width * 0.36, alignItems: 'center', justifyContent: 'flex-start' }
    fontSize = height * 0.115
  } else if (o === 'portrait') {
    const s = width * 1.0
    art = { left: (width - s) / 2, top: height * 0.4 - s / 2, width: s, height: s }
    wordBox = { left: 0, top: height * 0.73, width, height: height * 0.16, alignItems: 'center', justifyContent: 'center' }
    fontSize = width * 0.15
  } else {
    const s = height * 0.8
    art = { left: (width - s) / 2, top: height * 0.03, width: s, height: s }
    wordBox = { left: 0, top: height * 0.8, width, height: height * 0.17, alignItems: 'center', justifyContent: 'center' }
    fontSize = height * 0.1
  }

  return (
    <AbsoluteFill>
      <div
        style={{
          position: 'absolute',
          ...art,
          transform: `scale(${scale})`,
          filter: `blur(${blur}px)`,
          opacity,
        }}
      >
        <svg viewBox="0 0 1000 1000" width="100%" height="100%" style={{ overflow: 'visible' }}>
          {children}
        </svg>
      </div>
      {word ? (
        <div style={{ position: 'absolute', display: 'flex', ...wordBox }}>
          <KineticWord text={word} size={fontSize} accent={accent} duration={duration} align={o === 'landscape' ? 'left' : 'center'} />
        </div>
      ) : null}
    </AbsoluteFill>
  )
}

export const KineticWord: React.FC<{
  text: string
  size: number
  accent: string
  duration: number
  delay?: number
  align?: 'left' | 'center'
}> = ({ text, size, accent, duration, delay = 4, align = 'center' }) => {
  const frame = useCurrentFrame()
  const letters = text.split('')
  const exit = interpolate(frame, [duration - 6, duration], [0, 1], clamp)
  const bar = ease(frame, delay + 6, delay + 16)
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: align === 'left' ? 'flex-start' : 'center' }}>
      <div style={{ display: 'flex', fontFamily: FONT, fontWeight: 800, fontSize: size, letterSpacing: -size * 0.03, lineHeight: 1 }}>
        {letters.map((ch, i) => {
          const t = ease(frame, delay + i * 1.4, delay + i * 1.4 + 10)
          const out = ease(frame, duration - 7 + i * 0.4, duration - 1 + i * 0.4)
          return (
            <span
              key={i}
              style={{
                display: 'inline-block',
                whiteSpace: 'pre',
                transform: `translateY(${(1 - t) * size * 0.9 - out * size * 0.6}px) rotate(${(1 - t) * 18}deg) scale(${0.6 + t * 0.4})`,
                opacity: t * (1 - out),
                color: C.text,
                textShadow: `0 0 ${size * 0.35}px ${accent}66`,
              }}
            >
              {ch}
            </span>
          )
        })}
      </div>
      <div
        style={{
          marginTop: size * 0.18,
          height: size * 0.07,
          width: size * 1.6 * bar * (1 - exit),
          borderRadius: size,
          background: `linear-gradient(90deg, ${accent}, ${accent}00)`,
        }}
      />
    </div>
  )
}

/** White flash on scene cuts. */
export const CutFlash: React.FC<{ at: number[] }> = ({ at }) => {
  const frame = useCurrentFrame()
  let o = 0
  for (const a of at) {
    const d = frame - a
    if (d >= 0 && d < 6) o = Math.max(o, (1 - d / 6) * 0.35)
  }
  return <AbsoluteFill style={{ background: '#fff', opacity: o, mixBlendMode: 'overlay' }} />
}

/** Camera shake helper for the big hits. */
export const shake = (frame: number, at: number, strength = 14) => {
  const d = frame - at
  if (d < 0 || d > 12) return { x: 0, y: 0 }
  const k = (1 - d / 12) * strength
  return { x: Math.sin(d * 7.3) * k, y: Math.cos(d * 9.1) * k }
}

export const beatIndex = (frame: number) => Math.floor(frame / BEAT)
