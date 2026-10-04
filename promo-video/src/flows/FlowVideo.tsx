import React from 'react'
import { AbsoluteFill, Html5Audio, Sequence, interpolateColors, staticFile, useCurrentFrame } from 'remotion'
import { CutFlash, FlowBackground, SceneDef, SceneWrap, TopBar, buildTimeline, lin, shake } from './kit'

export type FlowScene = SceneDef & { Comp: React.FC<{ duration: number }>; wrap?: boolean }

/** Sequences beat-length scenes over one soundtrack with shared chrome. */
export const FlowVideo: React.FC<{ scenes: FlowScene[]; beat: number; music: string; label: string; accent2: string; hits?: string[] }> = ({
  scenes,
  beat,
  music,
  label,
  accent2,
  hits = [],
}) => {
  const frame = useCurrentFrame()
  const tl = buildTimeline(scenes, beat)
  const total = tl[tl.length - 1].from + tl[tl.length - 1].duration
  const stops = tl.flatMap((s) => [s.from + 4, s.from + s.duration - 4])
  const accent = interpolateColors(frame, stops, tl.flatMap((s) => [s.accent, s.accent]))
  let sh = { x: 0, y: 0 }
  for (const id of hits) {
    const s = tl.find((x) => x.id === id)
    if (s) {
      const k = shake(frame, s.from, 12)
      sh = { x: sh.x + k.x, y: sh.y + k.y }
    }
  }
  const black = Math.max(lin(frame, 0, 8, [1, 0]), lin(frame, total - 8, total))
  return (
    <AbsoluteFill style={{ background: '#000' }}>
      <Html5Audio src={staticFile(`flows/${music}`)} />
      <FlowBackground accent={accent} accent2={accent2} beat={beat} />
      <AbsoluteFill style={{ transform: `translate(${sh.x}px, ${sh.y}px)` }}>
        {tl.map((s) => (
          <Sequence key={s.id} from={s.from} durationInFrames={s.duration} name={s.id}>
            {s.wrap === false ? <s.Comp duration={s.duration} /> : (
              <SceneWrap duration={s.duration}>
                <s.Comp duration={s.duration} />
              </SceneWrap>
            )}
          </Sequence>
        ))}
      </AbsoluteFill>
      <TopBar scenes={tl} accent={accent} label={label} />
      <CutFlash at={tl.slice(1).map((s) => s.from)} />
      <AbsoluteFill style={{ background: '#000', opacity: black }} />
    </AbsoluteFill>
  )
}
