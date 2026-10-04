import React from 'react'
import { AbsoluteFill, Html5Audio, Sequence, interpolate, interpolateColors, staticFile, useCurrentFrame } from 'remotion'
import { loadFont } from '@remotion/google-fonts/Inter'
import { Background, CutFlash, shake } from './components/Layout'
import { Intro } from './scenes/Intro'
import { Agents, Delegate, Flows } from './scenes/Features1'
import { Board, Tools, Workspaces } from './scenes/Features2'
import { Control, Models } from './scenes/Features3'
import { Outro, Stats, Themes } from './scenes/Finale'
import { BEAT, C, PALETTE, SCENES, TOTAL_FRAMES, clamp } from './theme'

loadFont('normal', { weights: ['500', '600', '700', '800', '900'], subsets: ['latin'] })

const COMPONENTS: Record<(typeof SCENES)[number]['id'], React.FC<{ duration: number }>> = {
  intro: Intro,
  agents: Agents,
  delegate: Delegate,
  flows: Flows,
  board: Board,
  workspaces: Workspaces,
  tools: Tools,
  control: Control,
  models: Models,
  stats: Stats,
  themes: Themes,
  outro: Outro,
}

const ACCENTS: Record<(typeof SCENES)[number]['id'], string> = {
  intro: C.brand,
  agents: C.violet,
  delegate: C.cyan,
  flows: C.blue,
  board: C.emerald,
  workspaces: C.amber,
  tools: C.orange,
  control: C.lime,
  models: C.fuchsia,
  stats: C.cyan,
  themes: C.rose,
  outro: C.brand,
}

const timeline = (() => {
  let at = 0
  return SCENES.map((s) => {
    const from = at
    at += s.beats * BEAT
    return { ...s, from, duration: s.beats * BEAT }
  })
})()

export const Promo: React.FC = () => {
  const frame = useCurrentFrame()
  const cuts = timeline.slice(1).map((s) => s.from)

  // Accent glides into the next scene's colour across each cut.
  const stops = timeline.flatMap((s) => [s.from + 4, s.from + s.duration - 4])
  const colors = timeline.flatMap((s) => [ACCENTS[s.id], ACCENTS[s.id]])
  const accent = interpolateColors(frame, stops, colors)
  const accent2 = PALETTE[(Math.floor(frame / (BEAT * 6)) + 3) % PALETTE.length]

  const drop = timeline.find((s) => s.id === 'agents')!.from
  const sh = shake(frame, drop, 18)
  const blackIn = interpolate(frame, [0, 8], [1, 0], clamp)
  const blackOut = interpolate(frame, [TOTAL_FRAMES - 10, TOTAL_FRAMES], [0, 1], clamp)

  return (
    <AbsoluteFill style={{ background: C.bg }}>
      <Html5Audio src={staticFile('music.wav')} />
      <Background accent={accent} accent2={accent2} />
      <AbsoluteFill style={{ transform: `translate(${sh.x}px, ${sh.y}px)` }}>
        {timeline.map((s) => {
          const Comp = COMPONENTS[s.id]
          return (
            <Sequence key={s.id} from={s.from} durationInFrames={s.duration} name={s.id}>
              <Comp duration={s.duration} />
            </Sequence>
          )
        })}
      </AbsoluteFill>
      <CutFlash at={cuts} />
      <AbsoluteFill style={{ background: '#000', opacity: Math.max(blackIn, blackOut) }} />
    </AbsoluteFill>
  )
}
