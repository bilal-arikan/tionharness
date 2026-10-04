// Video 3 -- "JEV + otomasyon": criteria gate, run grading, proposal gate, trigger node,
// flow-kind automations. Music: signal.wav (100 BPM, 18 f/beat).
import React from 'react'
import { useCurrentFrame, useVideoConfig } from 'remotion'
import { FlowScene, FlowVideo } from './FlowVideo'
import {
  Caption,
  Chip,
  ChipRow,
  Edge,
  EdgeLayer,
  Eyebrow,
  FONT,
  Headline,
  Hl,
  Icon,
  MONO,
  NodeCard,
  Outro,
  Star,
  Status,
  T,
  Token,
  W,
  curve,
  ease,
  lin,
  pop,
} from './kit'

const BEAT = 18
const GOLD = T.warning

const Box: React.FC<{ top: number; left?: number; width?: number; height?: number; t?: number; color?: string; children: React.ReactNode; style?: React.CSSProperties }> = ({
  top,
  left = 70,
  width = 940,
  height,
  t = 1,
  color = T.border,
  children,
  style,
}) => (
  <div
    style={{
      position: 'absolute',
      left,
      top,
      width,
      height,
      borderRadius: 28,
      background: `linear-gradient(180deg, ${T.surface2}, ${T.surface})`,
      border: `2px solid ${color}`,
      boxShadow: '0 30px 80px -30px #000',
      padding: '24px 30px',
      fontFamily: FONT,
      opacity: Math.min(1, t * 1.4),
      transform: `translateY(${(1 - t) * 60}px)`,
      ...style,
    }}
  >
    {children}
  </div>
)

// --- 1. Hook ---------------------------------------------------------------------
const Hook: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const tick = Math.floor(frame / 9) % 2 ? 1 : -1
  const tilt = tick * 7 * Math.exp(-((frame % 9) / 9) * 2)
  const charge = lin(frame, 118, 144)
  return (
    <div style={{ position: 'absolute', inset: 0, transform: `scale(${1 + charge * 0.08})` }}>
      <Eyebrow text="JEV DECISION LAYER" color={GOLD} at={0} top={420} />
      <div style={{ position: 'absolute', top: 560, width: W, display: 'flex', justifyContent: 'center', transform: `scale(${pop(frame, fps, 4, 10)}) rotate(${tilt}deg)` }}>
        <div style={{ width: 230, height: 230, borderRadius: 60, background: `${GOLD}18`, border: `3px solid ${GOLD}77`, display: 'flex', alignItems: 'center', justifyContent: 'center', boxShadow: `0 0 80px ${GOLD}44` }}>
          <Icon name="scale" size={140} color={GOLD} stroke={1.6} />
        </div>
      </div>
      <Headline lines={['Flow decisions,', <>made by <Hl c={GOLD}>JEV</Hl></>]} at={20} gap={14} top={880} size={104} />
      <ChipRow
        top={1150}
        at={56}
        step={8}
        size={32}
        items={[
          { text: 'yes / no', color: GOLD, icon: 'check' },
          { text: 'pick one', color: GOLD, icon: 'branch' },
          { text: 'score', color: GOLD, icon: 'gauge' },
        ]}
      />
      <div style={{ position: 'absolute', top: 1290, left: 140, width: 800, display: 'flex', gap: 30 }}>
        {[
          ['~0.5 s', 'per decision'],
          ['~$0.00003', 'per call'],
        ].map(([big, small], i) => {
          const t = pop(frame, fps, 84 + i * 9)
          return (
            <div key={big} style={{ flex: 1, textAlign: 'center', borderRadius: 26, border: `2px solid ${T.border}`, background: T.surface, padding: '24px 0', transform: `scale(${t})`, opacity: t, fontFamily: FONT }}>
              <div style={{ fontFamily: MONO, fontWeight: 700, fontSize: 52, color: T.text }}>{big}</div>
              <div style={{ fontSize: 28, color: T.dim, marginTop: 6 }}>{small}</div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

// --- 2. Criteria gate ------------------------------------------------------------
const CRIT = ['Answers the question directly', 'Cites a source', 'No made-up facts']
const PASS1 = [0.94, 0.31, 0.88]
const PASS2 = [0.95, 0.91, 0.9]

const Criteria: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const second = frame >= 118
  const base = second ? 128 : 26
  const fill = ease(frame, base, base + 26)
  const vals = second ? PASS2 : PASS1
  const decided = frame >= base + 30
  const passed = second && decided
  const failed = !second && decided
  return (
    <>
      <Eyebrow text="CRITERIA GATE" color={GOLD} />
      <Headline lines={['N criteria,', <Hl c={GOLD}>one call</Hl>]} size={88} />
      <Box top={510} t={ease(frame, 2, 16)} height={150}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 24, fontWeight: 700, letterSpacing: 2, color: T.accent }}>
          <Icon name="bot" size={28} color={T.accent} /> DRAFT {second ? '· ROUND 2' : ''}
        </div>
        <div style={{ marginTop: 12, fontSize: 31, color: T.text, lineHeight: 1.35 }}>
          Every turn runs through the agent’s own flow.{second ? <span style={{ color: T.success }}> [source: _Docs/93]</span> : null}
        </div>
      </Box>
      <Box top={700} t={ease(frame, 8, 22)} color={`${GOLD}66`} height={370}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 24, fontWeight: 700, letterSpacing: 2, color: GOLD }}>
          <Icon name="branch" size={28} color={GOLD} /> ROUTE · CRITERIA
          <div style={{ flex: 1 }} />
          <span style={{ fontFamily: MONO, letterSpacing: 0, color: T.dim }}>flow-criteria</span>
        </div>
        {CRIT.map((c, i) => {
          const v = vals[i] * fill
          const ok = vals[i] >= 0.5
          return (
            <div key={c} style={{ marginTop: 26, display: 'flex', flexDirection: 'column', gap: 10 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 14, fontSize: 31, fontWeight: 600, color: T.text }}>
                {c}
                <div style={{ flex: 1 }} />
                <span style={{ fontFamily: MONO, fontSize: 28, color: T.dim }}>{v.toFixed(2)}</span>
                <div style={{ width: 40 }}>{decided ? <Icon name={ok ? 'check' : 'x'} size={36} color={ok ? T.success : T.danger} stroke={3} /> : null}</div>
              </div>
              <div style={{ height: 14, borderRadius: 7, background: T.bg, overflow: 'hidden' }}>
                <div style={{ width: `${v * 100}%`, height: '100%', borderRadius: 7, background: decided ? (ok ? T.success : T.danger) : GOLD }} />
              </div>
            </div>
          )
        })}
      </Box>
      <div style={{ position: 'absolute', top: 1110, left: 70, width: 940, display: 'flex', gap: 24 }}>
        {[
          { k: 'fail', to: 'Revise', c: T.danger, on: failed },
          { k: 'pass', to: 'Output', c: T.success, on: passed },
        ].map((a) => (
          <div
            key={a.k}
            style={{
              flex: 1,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 14,
              padding: '22px 0',
              borderRadius: 22,
              fontFamily: FONT,
              fontSize: 34,
              fontWeight: 800,
              color: a.on ? T.bg : a.c,
              background: a.on ? a.c : `${a.c}12`,
              border: `2px solid ${a.c}${a.on ? '' : '55'}`,
              boxShadow: a.on ? `0 0 40px ${a.c}88` : 'none',
              transform: `scale(${a.on ? 1 + 0.06 * Math.exp(-(frame - base - 30) / 6) : 1})`,
            }}
          >
            <span style={{ fontFamily: MONO }}>{a.k}</span> → {a.to}
          </div>
        ))}
      </div>
      <Caption at={60} top={1250} size={33}>
        {failed ? (
          <>
            The failed criterion lands in the step log: <span style={{ color: T.danger, fontWeight: 700 }}>Cites a source</span>
          </>
        ) : passed ? (
          <>All met → the <span style={{ color: T.success, fontWeight: 700 }}>pass</span> arm. Undecided → default arm.</>
        ) : (
          <>The decider checks every criterion in a single call.</>
        )}
      </Caption>
    </>
  )
}

// --- 3. Run grading --------------------------------------------------------------
const GRADED = [
  { id: '#241', q: 'Write release notes', g: 5, conf: 91 },
  { id: '#242', q: 'Summarize the PR', g: 4, conf: 84 },
  { id: '#243', q: 'Draft the changelog', g: 2, conf: 88, down: true },
  { id: '#244', q: 'Weekly report', g: 5, conf: 93 },
]

const Grade: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  return (
    <>
      <Eyebrow text="RUN GRADING" color={GOLD} />
      <Headline lines={['Every run gets', <Hl c={GOLD}>a grade</Hl>]} size={92} />
      {GRADED.map((r, i) => {
        const t = pop(frame, fps, 6 + i * 9, 13)
        const g0 = 18 + i * 9
        return (
          <Box key={r.id} top={530 + i * 180} height={150} t={t} color={r.down ? `${T.danger}66` : T.border} style={{ display: 'flex', alignItems: 'center', gap: 22 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, flex: 1, minWidth: 0 }}>
              <div style={{ fontFamily: MONO, fontSize: 26, color: T.dim }}>{r.id}</div>
              <div style={{ fontSize: 34, fontWeight: 700, color: T.text, whiteSpace: 'nowrap' }}>{r.q}</div>
            </div>
            {r.down ? <Icon name="thumbDown" size={36} color={T.danger} /> : null}
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 8 }}>
              <div style={{ display: 'flex', gap: 4 }}>
                {[0, 1, 2, 3, 4].map((k) => (
                  <Star key={k} size={40} fill={k < r.g ? lin(frame, g0 + k * 3, g0 + k * 3 + 4) : 0} color={r.g <= 2 ? T.danger : GOLD} />
                ))}
              </div>
              <div style={{ fontFamily: MONO, fontSize: 24, color: T.dim, opacity: ease(frame, g0 + 16, g0 + 24) }}>
                {r.g}/5 · {r.conf}% conf.
              </div>
            </div>
          </Box>
        )
      })}
      <Caption at={50} top={1270} size={34}>
        <span style={{ fontFamily: MONO, color: GOLD }}>flow-grade</span>: request + reply → 1..5. The observer reads grades as evidence.
      </Caption>
    </>
  )
}

// --- 4. Proposal gate ------------------------------------------------------------
const Gate: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const into = ease(frame, 22, 44)
  const verdict = frame >= 52
  const applied = frame >= 66
  return (
    <>
      <Eyebrow text="PROPOSAL GATE" color={GOLD} />
      <Headline lines={['Does the proposal', <Hl c={GOLD}>actually help?</Hl>]} size={84} />
      <Box top={520 + into * 120} left={190} width={700} height={120} t={ease(frame, 0, 12) * (1 - into)} color={`${T.fuchsia}88`} style={{ display: 'flex', alignItems: 'center', gap: 16, transform: `scale(${1 - into * 0.4})` }}>
        <Icon name="sparkle" size={36} color={T.fuchsia} />
        <span style={{ fontSize: 32, fontWeight: 700, color: T.text }}>Proposal #12</span>
        <span style={{ fontFamily: MONO, fontSize: 26, color: T.dim }}>+1 node · conf. 0.82</span>
      </Box>
      <div style={{ position: 'absolute', top: 640, width: W, display: 'flex', justifyContent: 'center', transform: `scale(${pop(frame, fps, 6) * (1 + 0.08 * Math.exp(-Math.max(0, frame - 44) / 5) * (frame >= 44 ? 1 : 0))})` }}>
        <div style={{ position: 'relative', width: 230, height: 230, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Icon name="shield" size={230} color={verdict ? T.success : GOLD} stroke={1.4} fill={verdict ? `${T.success}22` : `${GOLD}14`} />
          <div style={{ position: 'absolute', fontFamily: MONO, fontWeight: 700, fontSize: 40, color: verdict ? T.success : GOLD }}>{verdict ? '0.78' : '?'}</div>
        </div>
      </div>
      <div style={{ position: 'absolute', top: 900, width: W, textAlign: 'center', fontFamily: FONT, fontSize: 36, fontWeight: 700, color: T.text, opacity: ease(frame, 30, 40) }}>
        {verdict ? <span style={{ color: T.success }}>yes → apply</span> : 'does it help?'}
      </div>
      <div style={{ position: 'absolute', top: 990, left: 70, width: 940, display: 'flex', gap: 18 }}>
        {[
          ['off', 'apply'],
          ['shadow', 'apply + log'],
          ['on', '“no” → holds'],
        ].map(([m, d], i) => {
          const t = pop(frame, fps, 10 + i * 5)
          const cur = m === 'on'
          return (
            <div
              key={m}
              style={{
                flex: 1,
                borderRadius: 22,
                padding: '20px 14px',
                textAlign: 'center',
                fontFamily: FONT,
                background: cur ? `${GOLD}1c` : T.surface,
                border: `2px solid ${cur ? GOLD : T.border}`,
                transform: `scale(${t})`,
              }}
            >
              <div style={{ fontFamily: MONO, fontWeight: 700, fontSize: 32, color: cur ? GOLD : T.text }}>{m}</div>
              <div style={{ fontSize: 25, color: T.dim, marginTop: 8 }}>{d}</div>
            </div>
          )
        })}
      </div>
      <div style={{ position: 'absolute', top: 1180, width: W, display: 'flex', justifyContent: 'center' }}>
        <Chip text="v6 applied" color={T.success} icon="check" size={32} t={applied ? pop(frame, fps, 66) : 0} solid />
      </div>
      <div style={{ position: 'absolute', top: 1300, left: 70, width: 940, display: 'flex', alignItems: 'center', gap: 18, fontFamily: FONT, opacity: ease(frame, 84, 96) }}>
        <div style={{ display: 'flex', gap: 8 }}>
          {[0, 1, 2, 3, 4].map((k) => (
            <div key={k} style={{ width: 26, height: 26, borderRadius: 13, background: T.success, transform: `scale(${pop(frame, fps, 86 + k * 3)})`, boxShadow: `0 0 12px ${T.success}` }} />
          ))}
        </div>
        <span style={{ fontSize: 31, color: T.dim, lineHeight: 1.3 }}>
          Recent runs healthy? <b style={{ color: T.text }}>The observer is skipped</b>.
        </span>
      </div>
    </>
  )
}

// --- 5. Trigger node -------------------------------------------------------------
const TX = 330
const TY = { input: 600, llm: 790, trig: 980, out: 1170 }
const TE = {
  a: curve({ x: TX, y: TY.input + 56 }, { x: TX, y: TY.llm - 58 }),
  b: curve({ x: TX, y: TY.llm + 56 }, { x: TX, y: TY.trig - 58 }),
  c: curve({ x: TX, y: TY.trig + 56 }, { x: TX, y: TY.out - 58 }),
  fire: curve({ x: TX + 232, y: TY.trig }, { x: 805, y: 880 }, { x: 720, y: TY.trig }, { x: 805, y: TY.trig }),
}

const Trigger: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const st = (a: number, b: number): Status => (frame < a ? 'idle' : frame < b ? 'running' : 'done')
  const fired = frame >= 118
  const flash = Math.max(0, 1 - Math.abs(frame - 96) / 10)
  const card = pop(frame, fps, 14)
  return (
    <>
      <Eyebrow text="TRIGGER NODE" color={T.fuchsia} />
      <Headline lines={['Fire automations', <><Hl c={T.fuchsia}>from inside</Hl> a flow</>]} size={84} />
      <EdgeLayer>
        <Edge c={TE.a} p={lin(frame, 10, 22)} active={frame > 30 && frame < 42 ? 1 : 0} />
        <Edge c={TE.b} p={lin(frame, 14, 26)} active={frame > 80 && frame < 92 ? 1 : 0} />
        <Edge c={TE.c} p={lin(frame, 18, 30)} active={frame > 100 && frame < 114 ? 1 : 0} label="unchanged" labelAt={0.5} labelColor={T.dim} color={T.faint} />
        <Edge c={TE.fire} p={lin(frame, 92, 112)} color={T.fuchsia} active={frame > 92 && frame < 118 ? 1 : 0} />
        <Token c={TE.a} t={lin(frame, 30, 42)} color={T.success} />
        <Token c={TE.b} t={lin(frame, 80, 92)} color={T.accent} />
        <Token c={TE.c} t={lin(frame, 100, 114)} color={T.accent} />
        <Token c={TE.fire} t={lin(frame, 94, 116)} color={T.fuchsia} r={15} />
        {flash > 0 ? <circle cx={TX} cy={TY.trig} r={140 + (1 - flash) * 200} fill="none" stroke={T.fuchsia} strokeWidth={10 * flash} opacity={flash} /> : null}
      </EdgeLayer>
      <NodeCard type="input" title="Friday 5 PM" x={TX} y={TY.input} w={460} t={pop(frame, fps, 2)} status={frame >= 30 ? 'done' : 'idle'} />
      <NodeCard type="llm" title="Write report" x={TX} y={TY.llm} w={460} t={pop(frame, fps, 6)} status={st(42, 80)} />
      <NodeCard type="trigger" title="Distribute" x={TX} y={TY.trig} w={460} t={pop(frame, fps, 10)} status={st(92, 100)} />
      <NodeCard type="output" title="{{last}}" x={TX} y={TY.out} w={460} t={pop(frame, fps, 14)} status={frame >= 114 ? 'done' : 'idle'} />
      <div
        style={{
          position: 'absolute',
          left: 600,
          top: 640,
          width: 410,
          borderRadius: 28,
          padding: 26,
          background: fired ? `linear-gradient(160deg, ${T.fuchsia}2a, ${T.surface})` : T.surface,
          border: `2px solid ${fired ? T.fuchsia : T.border}`,
          boxShadow: fired ? `0 0 50px ${T.fuchsia}66` : '0 30px 70px -30px #000',
          fontFamily: FONT,
          opacity: Math.min(1, card * 1.4),
          transform: `scale(${card * (fired ? 1 + 0.05 * Math.exp(-(frame - 118) / 6) : 1)})`,
          display: 'flex',
          flexDirection: 'column',
          gap: 12,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Icon name="repeat" size={32} color={T.fuchsia} />
          <span style={{ fontSize: 22, fontWeight: 800, letterSpacing: 2, color: T.fuchsia }}>AUTOMATION</span>
        </div>
        <div style={{ fontSize: 34, fontWeight: 700, color: T.text }}>Send to team</div>
        <div style={{ fontFamily: MONO, fontSize: 24, color: T.dim }}>prompt: {'{{result}}'}</div>
        <div style={{ fontSize: 27, fontWeight: 700, color: fired ? T.success : T.faint, display: 'flex', alignItems: 'center', gap: 8 }}>
          {fired ? <Icon name="check" size={28} color={T.success} stroke={3} /> : <Icon name="clock" size={28} color={T.faint} />}
          {fired ? 'fired' : 'waiting'}
        </div>
      </div>
      <ChipRow
        top={1290}
        at={120}
        step={5}
        size={26}
        items={[
          { text: 'on / off', color: T.fuchsia },
          { text: 'cooldown', color: T.fuchsia },
          { text: 'iteration cap', color: T.fuchsia },
          { text: 'autonomy brake', color: T.fuchsia },
        ]}
      />
      <Caption at={136} top={1440} size={33}>
        If it can’t fire, the turn still completes; the reason goes into the step detail.
      </Caption>
    </>
  )
}

// --- 6. Flow-kind automation -----------------------------------------------------
const FILTERS = [
  ['agent', '=', 'Writer'],
  ['outcome', '=', 'failure'],
  ['grade', '≤', '2'],
]

const FlowAuto: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const fired = frame >= 76
  return (
    <>
      <Eyebrow text="FLOW-KIND AUTOMATION" color={T.fuchsia} />
      <Headline lines={['When a run ends,', <><Hl c={T.fuchsia}>automations</Hl> start</>]} size={84} />
      <Box top={520} height={130} t={ease(frame, 0, 12)} color={`${T.danger}66`} style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
        <Icon name="flag" size={40} color={T.danger} />
        <span style={{ fontSize: 36, fontWeight: 700, color: T.text }}>Run #243 finished</span>
        <div style={{ flex: 1 }} />
        <Chip text="failure" color={T.danger} size={24} />
        <Chip text="★ 2/5" color={T.danger} size={24} />
      </Box>
      <svg width={W} height={1920} style={{ position: 'absolute', left: 0, top: 0 }}>
        <line x1={540} y1={650} x2={540} y2={650 + 450 * ease(frame, 10, 70)} stroke={fired ? T.fuchsia : T.faint} strokeWidth={5} strokeDasharray="4 12" strokeLinecap="round" />
      </svg>
      {FILTERS.map(([k, op, v], i) => {
        const t = pop(frame, fps, 8 + i * 5)
        const ok = frame >= 30 + i * 12
        return (
          <div
            key={k}
            style={{
              position: 'absolute',
              left: 190,
              top: 700 + i * 120,
              width: 700,
              height: 96,
              borderRadius: 22,
              background: T.surface,
              border: `2px solid ${ok ? T.success : T.border}`,
              display: 'flex',
              alignItems: 'center',
              gap: 18,
              padding: '0 28px',
              fontFamily: FONT,
              transform: `scale(${t})`,
            }}
          >
            <Icon name="funnel" size={32} color={T.dim} />
            <span style={{ fontFamily: MONO, fontSize: 30, color: T.dim }}>{k}</span>
            <span style={{ fontSize: 32, fontWeight: 700, color: T.text }}>{op} {v}</span>
            <div style={{ flex: 1 }} />
            {ok ? <Icon name="check" size={36} color={T.success} stroke={3} /> : null}
          </div>
        )
      })}
      <Box
        top={1080}
        left={140}
        width={800}
        height={150}
        t={pop(frame, fps, 64)}
        color={fired ? T.fuchsia : T.border}
        style={{ display: 'flex', alignItems: 'center', gap: 20, boxShadow: fired ? `0 0 60px ${T.fuchsia}66` : undefined }}
      >
        <div style={{ width: 80, height: 80, borderRadius: 22, background: `${T.fuchsia}22`, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Icon name="zap" size={44} color={T.fuchsia} />
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
          <span style={{ fontSize: 36, fontWeight: 800, color: T.text }}>Open a review task</span>
          <span style={{ fontSize: 26, fontWeight: 600, color: fired ? T.success : T.faint }}>{fired ? '✓ fired' : 'trigger: flow'}</span>
        </div>
      </Box>
      <Caption at={84} top={1290} size={33}>
        Never re-fires on its own session’s runs; <span style={{ fontFamily: MONO, color: T.text }}>MaxIterations</span> breaks the chain.
      </Caption>
    </>
  )
}

// --- 7. Outro --------------------------------------------------------------------
const End: React.FC<{ duration: number }> = ({ duration }) => (
  <Outro
    duration={duration}
    title={<>JEV <Hl c={GOLD}>decides</Hl>,<br />code applies</>}
    tagline="Smart gates, automatic chains."
    accent={GOLD}
    chips={['criteria gate', 'run grades', 'proposal gate', 'trigger node']}
  />
)

const SCENES: FlowScene[] = [
  { id: 'hook', beats: 8, accent: GOLD, Comp: Hook },
  { id: 'criteria', beats: 12, accent: GOLD, Comp: Criteria },
  { id: 'grade', beats: 8, accent: T.accent, Comp: Grade },
  { id: 'gate', beats: 8, accent: T.success, Comp: Gate },
  { id: 'trigger', beats: 12, accent: T.fuchsia, Comp: Trigger },
  { id: 'flowauto', beats: 8, accent: T.fuchsia, Comp: FlowAuto },
  { id: 'outro', beats: 8, accent: GOLD, Comp: End, wrap: false },
]
export const JEV_FRAMES = SCENES.reduce((a, s) => a + s.beats, 0) * BEAT

export const FlowsJev: React.FC = () => <FlowVideo scenes={SCENES} beat={BEAT} music="signal.wav" label="JEV + Automation" accent2={T.accent} hits={['criteria', 'outro']} />
