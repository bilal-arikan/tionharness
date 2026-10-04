// Video 1 -- "Flows": every agent now has one main flow. Music: neon.wav (120 BPM, 15 f/beat).
import React from 'react'
import { useCurrentFrame, useVideoConfig } from 'remotion'
import { FlowScene, FlowVideo } from './FlowVideo'
import {
  Caption,
  ChipRow,
  Edge,
  EdgeLayer,
  Eyebrow,
  FONT,
  Headline,
  Hl,
  Icon,
  MONO,
  NODE,
  NodeCard,
  NodeType,
  Outro,
  Panel,
  Star,
  Status,
  T,
  Token,
  W,
  curve,
  ease,
  lin,
  pop,
  typed,
} from './kit'

const BEAT = 15

// --- 1. Hook ---------------------------------------------------------------------
const Hook: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const charge = lin(frame, 96, 120)
  const dots = [0, 1, 2].map((i) => pop(frame, fps, 52 + i * 8))
  const line = ease(frame, 58, 84)
  return (
    <>
      <Eyebrow text="NEW FEATURE" color={T.brand} at={0} top={420} />
      <div style={{ position: 'absolute', inset: 0, transform: `scale(${1 + charge * 0.06})` }}>
        <Headline lines={['Every agent', 'now has', <>its own <Hl>flow</Hl>.</>]} at={4} gap={15} top={560} size={124} />
        <svg width={W} height={1920} style={{ position: 'absolute', left: 0, top: 0 }}>
          <line x1={540} y1={1110} x2={540} y2={1110 + 300 * line} stroke={T.accent} strokeWidth={6} strokeLinecap="round" opacity={0.8} />
          {dots.map((t, i) => {
            const c = [T.success, T.accent, T.success][i]
            return (
              <g key={i} transform={`translate(540 ${1110 + i * 150}) scale(${t})`}>
                <circle r={46} fill={c} opacity={0.18} />
                <circle r={26} fill={c} />
              </g>
            )
          })}
          {charge > 0 ? <circle cx={540} cy={1260} r={60 + charge * 260} fill="none" stroke="#fff" strokeWidth={4} opacity={charge * 0.6} /> : null}
        </svg>
        <Caption at={66} top={1500} size={46}>
          …and <span style={{ color: T.text, fontWeight: 700 }}>every turn</span> runs through it.
        </Caption>
      </div>
    </>
  )
}

// --- 2. Default flow -------------------------------------------------------------
const DefaultFlow: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const e1 = curve({ x: 540, y: 812 }, { x: 540, y: 960 })
  const e2 = curve({ x: 540, y: 1124 }, { x: 540, y: 1268 })
  const st = (from: number, to: number): Status => (frame < from ? 'idle' : frame < to ? 'running' : 'done')
  return (
    <>
      <Eyebrow text="DEFAULT FLOW" color={T.success} />
      <Headline
        lines={[
          'Starts simple:',
          <>
            <Hl c={T.success}>input</Hl> → <Hl>reply</Hl> → <Hl c={T.success}>output</Hl>
          </>,
        ]}
        size={78}
      />
      <EdgeLayer>
        <Edge c={e1} p={lin(frame, 18, 32)} active={frame > 44 && frame < 64 ? 1 : 0} />
        <Edge c={e2} p={lin(frame, 24, 38)} active={frame > 100 && frame < 120 ? 1 : 0} />
        <Token c={e1} t={lin(frame, 44, 64)} color={T.success} />
        <Token c={e2} t={lin(frame, 100, 120)} color={T.accent} />
      </EdgeLayer>
      <NodeCard type="input" title="User message" x={540} y={760} t={pop(frame, fps, 6)} status={frame >= 44 ? 'done' : 'idle'} />
      <NodeCard type="llm" title="Reply" chips={['thread', 'tools on']} x={540} y={1042} t={pop(frame, fps, 12)} status={st(64, 100)} />
      <NodeCard type="output" title="{{last}}" x={540} y={1320} t={pop(frame, fps, 18)} status={frame >= 120 ? 'done' : 'idle'} />
      <ChipRow
        top={1430}
        at={60}
        items={[
          { text: 'v1', color: T.accent, icon: 'commit' },
          { text: 'identical to a plain turn', color: T.success, icon: 'check' },
        ]}
      />
      <Caption at={80} top={1530}>
        Nothing breaks. When you want more, the canvas is ready.
      </Caption>
    </>
  )
}

// --- 3. Node types ---------------------------------------------------------------
const TYPES: { type: NodeType; name: string; desc: string; mono?: string }[] = [
  { type: 'input', name: 'Input', desc: "The turn's input", mono: '{{input}}' },
  { type: 'llm', name: 'Model', desc: 'Context, tools, schema, model' },
  { type: 'route', name: 'Route', desc: 'contains · regex · JSON · judge · criteria' },
  { type: 'transform', name: 'Transform', desc: 'Template, no model call', mono: '{{node.id}}' },
  { type: 'trigger', name: 'Trigger', desc: 'Fires any automation' },
  { type: 'output', name: 'Output', desc: "The turn's reply", mono: '{{last}}' },
]

const NodeTypes: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  return (
    <>
      <Eyebrow text="NODES" color={T.accent} />
      <Headline lines={['6 node types,', <><Hl>endless</Hl> combinations</>]} size={82} />
      {TYPES.map((d, i) => {
        const n = NODE[d.type]
        const t = pop(frame, fps, 10 + i * 12)
        const col = i % 2
        const row = Math.floor(i / 2)
        const lit = Math.max(0, 1 - Math.abs(frame - (10 + i * 12)) / 14)
        return (
          <div
            key={d.type}
            style={{
              position: 'absolute',
              left: 60 + col * 490,
              top: 540 + row * 300,
              width: 470,
              height: 270,
              borderRadius: 30,
              background: `linear-gradient(160deg, ${n.color}1f, ${T.surface} 55%)`,
              border: `2px solid ${n.color}${lit > 0.2 ? 'cc' : '44'}`,
              boxShadow: `0 0 ${50 * lit}px ${n.color}66, 0 30px 60px -30px #000`,
              padding: 30,
              fontFamily: FONT,
              opacity: Math.min(1, t * 1.4),
              transform: `translateY(${(1 - t) * 60}px) scale(${0.85 + 0.15 * t})`,
              display: 'flex',
              flexDirection: 'column',
              gap: 14,
            }}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
              <div style={{ width: 70, height: 70, borderRadius: 20, background: `${n.color}26`, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Icon name={n.icon} size={38} color={n.color} stroke={2.3} />
              </div>
              <div style={{ fontSize: 44, fontWeight: 800, color: T.text }}>{d.name}</div>
            </div>
            <div style={{ fontSize: 30, fontWeight: 500, color: T.dim, lineHeight: 1.3 }}>{d.desc}</div>
            {d.mono ? <div style={{ fontFamily: MONO, fontSize: 26, color: n.color }}>{d.mono}</div> : null}
          </div>
        )
      })}
      <Caption at={90} top={1450} size={36}>
        A stage can even run on another agent — say, a <span style={{ color: T.text, fontWeight: 700 }}>critic</span>.
      </Caption>
    </>
  )
}

// --- 4. Loops --------------------------------------------------------------------
const X = 470
const Y = { input: 590, draft: 770, critic: 950, route: 1130, output: 1310 }
const E = {
  in: curve({ x: X, y: Y.input + 56 }, { x: X, y: Y.draft - 58 }),
  dc: curve({ x: X, y: Y.draft + 56 }, { x: X, y: Y.critic - 58 }),
  cr: curve({ x: X, y: Y.critic + 56 }, { x: X, y: Y.route - 58 }),
  loop: curve({ x: X + 262, y: Y.route }, { x: X + 270, y: Y.draft }, { x: X + 520, y: Y.route }, { x: X + 520, y: Y.draft }),
  out: curve({ x: X, y: Y.route + 56 }, { x: X, y: Y.output - 58 }),
}
type Seg = { k: 'e' | 'n'; id: string; d: number }
const SEGS: Seg[] = [
  { k: 'e', id: 'in', d: 10 },
  { k: 'n', id: 'draft', d: 16 },
  { k: 'e', id: 'dc', d: 8 },
  { k: 'n', id: 'critic', d: 14 },
  { k: 'e', id: 'cr', d: 8 },
  { k: 'n', id: 'route', d: 12 },
  { k: 'e', id: 'loop', d: 18 },
  { k: 'n', id: 'draft', d: 12 },
  { k: 'e', id: 'dc', d: 7 },
  { k: 'n', id: 'critic', d: 10 },
  { k: 'e', id: 'cr', d: 7 },
  { k: 'n', id: 'route', d: 10 },
  { k: 'e', id: 'out', d: 9 },
  { k: 'n', id: 'output', d: 6 },
]
const SEG_START = 34
const timed = (() => {
  let a = SEG_START
  return SEGS.map((s) => {
    const from = a
    a += s.d
    return { ...s, from, to: a }
  })
})()

const Loops: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const cur = timed.find((s) => frame >= s.from && frame < s.to)
  const status = (id: string): Status => {
    if (cur?.k === 'n' && cur.id === id) return 'running'
    return timed.some((s) => s.k === 'n' && s.id === id && frame >= s.to) ? 'done' : 'idle'
  }
  const routeVisits = timed.filter((s) => s.k === 'n' && s.id === 'route' && frame >= s.from).length
  const routeDone = timed.filter((s) => s.k === 'n' && s.id === 'route' && frame >= s.to).length
  const tok = (id: keyof typeof E) => {
    const s = [...timed].reverse().find((x) => x.k === 'e' && x.id === id && frame >= x.from)
    return s ? lin(frame, s.from, s.to) : 0
  }
  const act = (id: keyof typeof E) => (cur?.k === 'e' && cur.id === id ? 1 : 0)
  const draw = (i: number) => lin(frame, 12 + i * 4, 26 + i * 4)
  return (
    <>
      <Eyebrow text="LOOPS" color={T.warning} />
      <Headline lines={['Build loops.', <>Never <Hl c={T.warning}>infinite</Hl>.</>]} size={84} />
      <EdgeLayer>
        <Edge c={E.in} p={draw(0)} active={act('in')} />
        <Edge c={E.dc} p={draw(1)} active={act('dc')} />
        <Edge c={E.cr} p={draw(2)} active={act('cr')} />
        <Edge c={E.loop} p={draw(3)} dashed color={T.warning} active={act('loop')} label="revise" labelAt={0.5} />
        <Edge c={E.out} p={draw(4)} active={act('out')} color={routeDone >= 2 ? T.success : T.faint} label="pass" labelAt={0.5} labelColor={T.success} />
        {(Object.keys(E) as (keyof typeof E)[]).map((id) => (
          <Token key={id} c={E[id]} t={act(id) ? tok(id) : 0} color={id === 'loop' ? T.warning : T.accent} />
        ))}
      </EdgeLayer>
      <NodeCard type="input" title="Task" x={X} y={Y.input} w={520} t={pop(frame, fps, 4)} status={frame >= SEG_START ? 'done' : 'idle'} />
      <NodeCard type="llm" title="Draft" x={X} y={Y.draft} w={520} t={pop(frame, fps, 8)} status={status('draft')} />
      <NodeCard type="llm" title="Critic" x={X} y={Y.critic} w={520} t={pop(frame, fps, 12)} status={status('critic')} />
      <NodeCard
        type="route"
        title="Good enough?"
        x={X}
        y={Y.route}
        w={520}
        t={pop(frame, fps, 16)}
        status={status('route') === 'done' ? 'idle' : status('route')}
        badge={
          <div style={{ fontFamily: MONO, fontSize: 26, fontWeight: 700, color: T.warning, background: `${T.warning}1c`, border: `2px solid ${T.warning}66`, borderRadius: 14, padding: '6px 12px' }}>
            {Math.max(1, routeVisits)}/3
          </div>
        }
      />
      <NodeCard type="output" title="Result" x={X} y={Y.output} w={520} t={pop(frame, fps, 20)} status={status('output')} />
      <ChipRow
        top={1420}
        at={44}
        step={5}
        size={27}
        items={[
          { text: 'maxSteps 24', color: T.warning, mono: true },
          { text: 'maxVisits 3', color: T.warning, mono: true },
          { text: 'default arm', color: T.success },
          { text: 'Validate ✓', color: T.info },
        ]}
      />
      <Caption at={70} top={1510} size={35}>
        Every loop must pass a route with a default arm — termination is enforced <span style={{ color: T.text, fontWeight: 700 }}>in code</span>.
      </Caption>
    </>
  )
}

// --- 5. Live in chat -------------------------------------------------------------
const STEPS: { type: NodeType; title: string; from: number; to: number; meta: string }[] = [
  { type: 'llm', title: 'Draft', from: 24, to: 58, meta: '2.1 s' },
  { type: 'llm', title: 'Critic', from: 58, to: 86, meta: '1.4 s' },
  { type: 'route', title: 'Good enough?', from: 86, to: 100, meta: '→ pass' },
  { type: 'output', title: 'Output', from: 100, to: 106, meta: '' },
]
const ANSWER = 'v2.4 notes: flows are now versioned per agent, loops are safe and every stage shows up in chat.'

const Live: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  return (
    <>
      <Eyebrow text="LIVE" color={T.info} />
      <Headline lines={['Every stage shows', <><Hl c={T.info}>live</Hl> in chat</>]} size={80} />
      <Panel x={70} y={520} w={940} h={880} t={ease(frame, 0, 16)} title="Chat · Writer">
        <div style={{ position: 'absolute', right: 30, top: 110, maxWidth: 640, opacity: ease(frame, 8, 18), transform: `translateY(${(1 - ease(frame, 8, 18)) * 20}px)` }}>
          <div style={{ background: '#24242f', border: '2px solid #3b3b4a', borderRadius: '26px 26px 6px 26px', padding: '20px 26px', fontSize: 32, color: T.text, fontWeight: 500 }}>
            Write release notes, then review.
          </div>
        </div>
        {STEPS.map((s, i) => {
          if (frame < s.from) return null
          const n = NODE[s.type]
          const running = frame < s.to
          const t = pop(frame, fps, s.from, 14)
          return (
            <div
              key={i}
              style={{
                position: 'absolute',
                left: 30,
                top: 240 + i * 100,
                width: 700,
                height: 84,
                borderRadius: 18,
                background: T.surface2,
                border: `2px solid ${running ? T.accent : T.border}`,
                boxShadow: running ? `0 0 26px ${T.accent}66` : 'none',
                display: 'flex',
                alignItems: 'center',
                gap: 16,
                padding: '0 22px',
                opacity: t,
                transform: `translateX(${(1 - t) * -40}px)`,
              }}
            >
              <Icon name={n.icon} size={32} color={n.color} />
              <div style={{ fontSize: 30, fontWeight: 700, color: T.text }}>{s.title}</div>
              <div style={{ fontSize: 24, fontWeight: 600, color: n.color, textTransform: 'uppercase', letterSpacing: 1.5 }}>{n.label}</div>
              <div style={{ flex: 1 }} />
              {running ? (
                <div style={{ width: 30, height: 30, borderRadius: 15, border: `4px solid ${T.accent}44`, borderTopColor: T.accent, transform: `rotate(${frame * 18}deg)` }} />
              ) : (
                <>
                  <div style={{ fontSize: 26, color: T.dim, fontFamily: MONO }}>{s.meta}</div>
                  <Icon name="check" size={32} color={T.success} stroke={3} />
                </>
              )}
            </div>
          )
        })}
        {frame >= 106 ? (
          <div style={{ position: 'absolute', left: 30, top: 660, width: 860, display: 'flex', gap: 18 }}>
            <div style={{ width: 56, height: 56, borderRadius: 28, background: `${T.accent}33`, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
              <Icon name="bot" size={30} color={T.accent} />
            </div>
            <div style={{ fontSize: 32, lineHeight: 1.4, color: T.text, fontWeight: 500 }}>
              {typed(ANSWER, frame, 108, 2.2)}
              <span style={{ opacity: frame % 16 < 8 ? 1 : 0, color: T.accent }}>▍</span>
            </div>
          </div>
        ) : null}
      </Panel>
      <Caption at={30} top={1440} size={36}>
        Every turn is saved as a <span style={{ color: T.text, fontWeight: 700 }}>run</span>: input, output, node trace, duration and tokens.
      </Caption>
    </>
  )
}

// --- 6. Flows screen -------------------------------------------------------------
const TABS = ['Canvas', 'Runs', 'Evolution', 'Test']

const MiniNode: React.FC<{ x: number; y: number; type: NodeType; label: string }> = ({ x, y, type, label }) => {
  const n = NODE[type]
  return (
    <div style={{ position: 'absolute', left: x - 150, top: y - 36, width: 300, height: 72, borderRadius: 16, background: T.surface2, border: `2px solid ${n.color}66`, display: 'flex', alignItems: 'center', gap: 12, padding: '0 18px' }}>
      <Icon name={n.icon} size={28} color={n.color} />
      <span style={{ fontSize: 28, fontWeight: 700, color: T.text }}>{label}</span>
    </div>
  )
}

const Screen: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const tab = Math.max(0, Math.min(3, Math.floor((frame - 14) / 24)))
  const tt = ease(frame, 14 + tab * 24, 24 + tab * 24)
  const body: React.ReactNode[] = [
    <div key="c" style={{ position: 'absolute', inset: 0 }}>
      <svg width={940} height={700} style={{ position: 'absolute', left: 0, top: 0 }}>
        {[
          [470, 106, 470, 194],
          [470, 266, 470, 354],
          [470, 426, 470, 514],
        ].map(([x1, y1, x2, y2], i) => (
          <line key={i} x1={x1} y1={y1} x2={x2} y2={y2} stroke={T.faint} strokeWidth={4} />
        ))}
        <path d="M620,390 C760,390 760,230 620,230" fill="none" stroke={T.warning} strokeWidth={4} strokeDasharray="12 10" />
      </svg>
      <MiniNode x={470} y={70} type="input" label="Input" />
      <MiniNode x={470} y={230} type="llm" label="Draft" />
      <MiniNode x={470} y={390} type="route" label="Good enough?" />
      <MiniNode x={470} y={550} type="output" label="Output" />
    </div>,
    <div key="r" style={{ padding: '10px 30px', display: 'flex', flexDirection: 'column', gap: 14 }}>
      {[
        ['#128', true, 5, '4 steps'],
        ['#127', true, 4, '6 steps'],
        ['#126', false, 2, '3 steps'],
        ['#125', true, 5, '4 steps'],
        ['#124', true, 4, '4 steps'],
      ].map(([id, ok, g, steps], i) => (
        <div key={i} style={{ height: 92, borderRadius: 18, background: T.surface2, border: `2px solid ${T.border}`, display: 'flex', alignItems: 'center', gap: 18, padding: '0 24px', fontSize: 30, color: T.text }}>
          <Icon name={ok ? 'check' : 'x'} size={32} color={ok ? T.success : T.danger} stroke={3} />
          <span style={{ fontFamily: MONO, fontWeight: 700 }}>{id as string}</span>
          <span style={{ color: T.dim }}>{steps as string}</span>
          <div style={{ flex: 1 }} />
          {[1, 2, 3, 4, 5].map((k) => (
            <Star key={k} size={28} fill={k <= (g as number) ? 1 : 0} />
          ))}
        </div>
      ))}
    </div>,
    <div key="e" style={{ padding: '10px 30px', display: 'flex', flexDirection: 'column', gap: 14 }}>
      <div style={{ height: 92, borderRadius: 18, border: `2px solid ${T.accent}`, background: `${T.accent}1a`, display: 'flex', alignItems: 'center', gap: 16, padding: '0 24px', fontSize: 30, color: T.text, fontWeight: 700 }}>
        <Icon name="sparkle" size={32} color={T.accent} /> 1 pending proposal
      </div>
      {[
        ['v3', 'agent', '+ Critic'],
        ['v2', 'user', '+ Good enough?'],
        ['v1', 'system', 'default'],
      ].map(([v, who, what]) => (
        <div key={v} style={{ height: 92, borderRadius: 18, background: T.surface2, border: `2px solid ${T.border}`, display: 'flex', alignItems: 'center', gap: 18, padding: '0 24px', fontSize: 30, color: T.text }}>
          <Icon name="commit" size={32} color={T.dim} />
          <span style={{ fontFamily: MONO, fontWeight: 700 }}>{v}</span>
          <span style={{ color: T.accent, fontWeight: 600 }}>{who}</span>
          <span style={{ color: T.dim }}>{what}</span>
        </div>
      ))}
    </div>,
    <div key="t" style={{ padding: '10px 30px', display: 'flex', flexDirection: 'column', gap: 20 }}>
      <div style={{ height: 200, borderRadius: 18, background: T.bg, border: `2px solid ${T.border}`, padding: 24, fontSize: 32, color: T.text, fontFamily: FONT }}>
        {typed('What changed this week?', frame, 92, 1.4)}
        <span style={{ color: T.accent }}>▍</span>
      </div>
      <div style={{ alignSelf: 'flex-end', display: 'flex', alignItems: 'center', gap: 12, background: T.accent, color: '#fff', fontWeight: 700, fontSize: 30, padding: '16px 30px', borderRadius: 16 }}>
        <Icon name="play" size={28} color="#fff" fill="#fff" /> Run
      </div>
      <div style={{ fontSize: 26, color: T.dim }}>The input runs as a new turn in a flow-test session.</div>
    </div>,
  ]
  return (
    <>
      <Eyebrow text="FLOWS SCREEN" color={T.accent} />
      <Headline lines={['One screen,', <><Hl>four tabs</Hl></>]} size={84} />
      <Panel x={70} y={520} w={940} h={860} t={ease(frame, 0, 14)} title="Flows · Writer · v3 · 5 nodes">
        <div style={{ display: 'flex', gap: 8, padding: '18px 24px', borderBottom: `2px solid ${T.border}` }}>
          {TABS.map((n, i) => (
            <div
              key={n}
              style={{
                flex: 1,
                textAlign: 'center',
                padding: '16px 0',
                borderRadius: 14,
                fontSize: 30,
                fontWeight: 700,
                color: i === tab ? '#fff' : T.dim,
                background: i === tab ? T.accent : 'transparent',
                boxShadow: i === tab ? `0 0 30px ${T.accent}66` : 'none',
              }}
            >
              {n}
            </div>
          ))}
        </div>
        <div style={{ position: 'absolute', left: 0, top: 190, width: 940, height: 670, opacity: tt, transform: `translateX(${(1 - tt) * 60}px)` }}>{body[tab]}</div>
      </Panel>
      <Caption at={30} top={1440} size={36}>
        On narrow and portrait screens: a drawer list and a bottom sheet.
      </Caption>
    </>
  )
}

// --- 7. Outro --------------------------------------------------------------------
const End: React.FC<{ duration: number }> = ({ duration }) => (
  <Outro duration={duration} title={<>Evolving <Hl>Flows</Hl></>} tagline="Every agent, its own flow." accent={T.accent} chips={['versioned', 'self-improving', 'safe loops']} />
)

const SCENES: FlowScene[] = [
  { id: 'hook', beats: 8, accent: T.brand, Comp: Hook },
  { id: 'default', beats: 12, accent: T.success, Comp: DefaultFlow },
  { id: 'types', beats: 12, accent: T.accent, Comp: NodeTypes },
  { id: 'loops', beats: 14, accent: T.warning, Comp: Loops },
  { id: 'live', beats: 12, accent: T.info, Comp: Live },
  { id: 'screen', beats: 8, accent: T.accent, Comp: Screen },
  { id: 'outro', beats: 6, accent: T.brand, Comp: End, wrap: false },
]
export const OVERVIEW_FRAMES = SCENES.reduce((a, s) => a + s.beats, 0) * BEAT

export const FlowsOverview: React.FC = () => (
  <FlowVideo scenes={SCENES} beat={BEAT} music="neon.wav" label="Flows" accent2={T.info} hits={['default', 'screen', 'outro']} />
)
