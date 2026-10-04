// Video 2 -- "Evolution": versions, self-editing agents, Flow Observer, safety rails.
// Music: dusty.wav (90 BPM, 20 f/beat).
import React from 'react'
import { useCurrentFrame, useVideoConfig } from 'remotion'
import { FlowScene, FlowVideo } from './FlowVideo'
import { Caption, ChipRow, Eyebrow, FONT, Headline, Hl, Icon, IconName, MONO, NODE, NodeType, Outro, Panel, T, W, ease, pop, typed } from './kit'

const BEAT = 20
const TEAL = '#2dd4bf'

// --- 1. Hook ---------------------------------------------------------------------
const Hook: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  // a small graph that grows: 3 dots -> insert one -> add a side loop
  const grow = pop(frame, fps, 84, 14)
  const side = pop(frame, fps, 112, 14)
  const ver = frame < 84 ? 1 : frame < 112 ? 2 : 3
  const pts = [1150, 1150 + 130 * (1 - grow) + 105 * grow, 1150 + 260 * (1 - grow) + 315 * grow]
  const mid = 1150 + 210 * grow
  return (
    <>
      <Eyebrow text="EVOLUTION" color={TEAL} at={0} top={380} />
      <Headline lines={['Flows don’t', 'stand still.', <Hl c={TEAL}>They evolve.</Hl>]} at={6} gap={20} top={480} size={112} />
      <svg width={W} height={1920} style={{ position: 'absolute', left: 0, top: 0 }}>
        <line x1={540} y1={1150} x2={540} y2={pts[2]} stroke={TEAL} strokeWidth={6} opacity={0.5 * ease(frame, 50, 70)} />
        {side > 0.01 ? (
          <path d={`M540,${mid} C720,${mid} 720,${pts[2]} 540,${pts[2]}`} fill="none" stroke={T.warning} strokeWidth={5} strokeDasharray="14 12" opacity={side} />
        ) : null}
        {[pts[0], pts[1], pts[2]].map((y, i) => {
          const t = pop(frame, fps, 50 + i * 8)
          const c = [T.success, T.accent, T.success][i]
          return (
            <g key={i} transform={`translate(540 ${y}) scale(${t})`}>
              <circle r={40} fill={c} opacity={0.18} />
              <circle r={22} fill={c} />
            </g>
          )
        })}
        <g transform={`translate(540 ${mid}) scale(${grow})`}>
          <circle r={60 * (1 + (1 - grow))} fill={TEAL} opacity={0.15} />
          <circle r={22} fill={TEAL} />
        </g>
      </svg>
      <div
        style={{
          position: 'absolute',
          left: 340,
          top: mid - 34,
          fontFamily: MONO,
          fontWeight: 700,
          fontSize: 54,
          color: TEAL,
          opacity: ease(frame, 60, 74),
          transform: `scale(${1 + 0.15 * Math.max(0, 1 - Math.abs(frame - (ver === 2 ? 84 : 112)) / 10)})`,
        }}
      >
        v{ver}
      </div>
    </>
  )
}

// --- 2. Versions -----------------------------------------------------------------
const AUTHORS: Record<string, string> = { system: T.dim, user: T.info, agent: T.accent, observer: T.fuchsia }
const VERSIONS = [
  { v: 'v1', who: 'system', what: 'Default flow', diff: 'input → reply → output' },
  { v: 'v2', who: 'user', what: '+ Critic loop', diff: '+2 nodes · +3 edges' },
  { v: 'v3', who: 'agent', what: '+ Source check', diff: '+1 node · reason: 👎 ×2' },
  { v: 'v4', who: 'observer', what: 'Prompt tightened', diff: 'confidence 0.82 · applied' },
]

const Versions: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  return (
    <>
      <Eyebrow text="VERSIONS" color={TEAL} />
      <Headline lines={['Every change is', <>a new <Hl c={TEAL}>version</Hl></>]} size={86} />
      <svg width={W} height={1920} style={{ position: 'absolute', left: 0, top: 0 }}>
        <line x1={140} y1={1340} x2={140} y2={1340 - 780 * ease(frame, 6, 80)} stroke={T.border} strokeWidth={6} strokeLinecap="round" />
      </svg>
      {VERSIONS.map((v, i) => {
        const t = pop(frame, fps, 8 + i * 18, 13)
        const y = 1250 - i * 200
        const c = AUTHORS[v.who]
        const head = i === VERSIONS.length - 1
        return (
          <React.Fragment key={v.v}>
            <div
              style={{
                position: 'absolute',
                left: 140 - 26,
                top: y + 60,
                width: 52,
                height: 52,
                borderRadius: 26,
                background: head ? TEAL : T.surface2,
                border: `4px solid ${head ? TEAL : c}`,
                boxShadow: head ? `0 0 30px ${TEAL}` : 'none',
                transform: `scale(${t})`,
              }}
            />
            <div
              style={{
                position: 'absolute',
                left: 200,
                top: y,
                width: 820,
                height: 172,
                borderRadius: 26,
                background: `linear-gradient(180deg, ${T.surface2}, ${T.surface})`,
                border: `2px solid ${head ? TEAL : T.border}`,
                padding: '24px 30px',
                fontFamily: FONT,
                opacity: Math.min(1, t * 1.4),
                transform: `translateX(${(1 - t) * 80}px)`,
                display: 'flex',
                flexDirection: 'column',
                gap: 12,
              }}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                <span style={{ fontFamily: MONO, fontWeight: 700, fontSize: 40, color: T.text }}>{v.v}</span>
                <span style={{ fontSize: 26, fontWeight: 700, color: c, background: `${c}1c`, border: `2px solid ${c}55`, borderRadius: 99, padding: '4px 16px' }}>{v.who}</span>
                <div style={{ flex: 1 }} />
                {head ? <span style={{ fontSize: 24, fontWeight: 800, letterSpacing: 2, color: TEAL }}>HEAD</span> : null}
              </div>
              <div style={{ fontSize: 36, fontWeight: 700, color: T.text }}>{v.what}</div>
              <div style={{ fontSize: 26, color: T.dim, fontFamily: MONO }}>{v.diff}</div>
            </div>
          </React.Fragment>
        )
      })}
      <Caption at={70} top={1460} size={35}>
        Immutable versions with author, reason and diff. Layout-only moves don’t open one.
      </Caption>
    </>
  )
}

// --- 3. Agent edits itself -------------------------------------------------------
const EDIT_CALL = `edit_flow({
  op: "insert_between",
  from: "draft", to: "output",
  node: { type: "llm", title: "Source check" },
  reason: "2 replies lacked sources"
})`

const ToolRow: React.FC<{ top: number; at: number; call: string; result: string; resultAt: number; ok?: boolean; cps?: number; tall?: boolean }> = ({ top, at, call, result, resultAt, cps = 3, tall }) => {
  const frame = useCurrentFrame()
  if (frame < at) return null
  const t = ease(frame, at, at + 10)
  const done = frame >= resultAt
  return (
    <div style={{ position: 'absolute', left: 28, top, width: 884, opacity: t, transform: `translateY(${(1 - t) * 20}px)` }}>
      <div
        style={{
          borderRadius: 18,
          background: T.bg,
          border: `2px solid ${done ? T.border : T.accent}`,
          padding: '16px 22px',
          fontFamily: MONO,
          fontSize: 27,
          lineHeight: 1.4,
          color: T.text,
          whiteSpace: 'pre',
          minHeight: tall ? 260 : undefined,
        }}
      >
        <span style={{ color: T.accent }}>{typed(call, frame, at, cps)}</span>
        {!done ? <span style={{ color: T.accent, opacity: frame % 14 < 7 ? 1 : 0 }}>▍</span> : null}
      </div>
      {done ? (
        <div style={{ marginTop: 10, display: 'flex', alignItems: 'center', gap: 12, fontFamily: FONT, fontSize: 28, fontWeight: 600, color: TEAL, opacity: ease(frame, resultAt, resultAt + 8) }}>
          <Icon name="check" size={30} color={TEAL} stroke={3} />
          {result}
        </div>
      ) : null}
    </div>
  )
}

const Pill: React.FC<{ x: number; type: NodeType; label: string; t?: number; glow?: boolean }> = ({ x, type, label, t = 1, glow }) => {
  const n = NODE[type]
  return (
    <div
      style={{
        position: 'absolute',
        left: x - 105,
        top: 1215,
        width: 210,
        height: 96,
        borderRadius: 20,
        background: T.surface2,
        border: `2px solid ${glow ? TEAL : `${n.color}66`}`,
        boxShadow: glow ? `0 0 ${36}px ${TEAL}aa` : 'none',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 4,
        fontFamily: FONT,
        transform: `scale(${t})`,
        opacity: Math.min(1, t * 1.5),
      }}
    >
      <Icon name={n.icon} size={30} color={glow ? TEAL : n.color} />
      <span style={{ fontSize: 25, fontWeight: 700, color: T.text }}>{label}</span>
    </div>
  )
}

const SelfEdit: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const ins = ease(frame, 126, 146)
  const xs3 = [220, 540, 860]
  const xs4 = [150, 393, 687, 930]
  const x = (i3: number, i4: number) => xs3[i3] + (xs4[i4] - xs3[i3]) * ins
  const newT = pop(frame, fps, 134, 11)
  return (
    <>
      <Eyebrow text="SELF-MANAGEMENT" color={T.accent} />
      <Headline lines={['Agents can edit', <Hl>their own flow</Hl>]} size={80} />
      <Panel x={70} y={500} w={940} h={660} t={ease(frame, 0, 14)} title="Writer · tool calls">
        <ToolRow top={100} at={10} call="get_flow()" result="v3 · 4 nodes · 18 runs" resultAt={34} />
        <ToolRow top={230} at={44} call={EDIT_CALL} result="v4 opened · budget 5/16" resultAt={120} cps={3.4} tall />
        <ToolRow top={600 - 30} at={150} call='update_my_prompt({ soul: "…" })' result="prompt v2" resultAt={176} cps={2.6} />
      </Panel>
      <svg width={W} height={1920} style={{ position: 'absolute', left: 0, top: 0, opacity: ease(frame, 20, 40) }}>
        <line x1={120} y1={1263} x2={960} y2={1263} stroke={T.faint} strokeWidth={4} />
      </svg>
      <div style={{ opacity: ease(frame, 20, 40) }}>
        <Pill x={x(0, 0)} type="input" label="Input" />
        <Pill x={x(1, 1)} type="llm" label="Draft" />
        {newT > 0.01 ? <Pill x={xs4[2]} type="llm" label="Sources" t={newT} glow={frame < 200} /> : null}
        <Pill x={x(2, 3)} type="output" label="Output" />
      </div>
      <ChipRow
        top={1360}
        at={60}
        step={4}
        size={24}
        items={['get_flow', 'edit_flow', 'revert_flow', 'list_flow_runs', 'update_my_prompt'].map((t) => ({ text: t, color: T.accent, mono: true }))}
      />
      <Caption at={90} top={1490} size={34}>
        Agents, the observer and the API share one op vocabulary; every result is validated.
      </Caption>
    </>
  )
}

// --- 4. Flow Observer ------------------------------------------------------------
const RUNS: { ok: boolean; fb?: 'up' | 'down' }[] = [{ ok: true, fb: 'up' }, { ok: true }, { ok: false, fb: 'down' }, { ok: true }, { ok: false, fb: 'down' }]

const Observer: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const scan = ease(frame, 64, 100)
  const card = ease(frame, 104, 124)
  const press = frame >= 168 && frame < 176 ? 0.92 : 1
  const applied = frame >= 176
  const pos = 1 + ease(frame, 196, 210) // slider: "propose" -> "auto"
  return (
    <>
      <Eyebrow text="FLOW OBSERVER" color={T.fuchsia} />
      <Headline lines={['The observer', <><Hl c={T.fuchsia}>watches</Hl> and proposes</>]} size={78} />
      {/* run strip */}
      {RUNS.map((r, i) => {
        const t = pop(frame, fps, 8 + i * 10)
        const x = 90 + i * 186
        const seen = scan * 5 > i + 0.5
        return (
          <div
            key={i}
            style={{
              position: 'absolute',
              left: x,
              top: 600,
              width: 156,
              height: 170,
              borderRadius: 24,
              background: T.surface2,
              border: `2px solid ${seen ? T.fuchsia : T.border}`,
              boxShadow: seen ? `0 0 24px ${T.fuchsia}55` : 'none',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 12,
              transform: `scale(${t})`,
              fontFamily: FONT,
            }}
          >
            <span style={{ fontFamily: MONO, fontSize: 24, color: T.dim }}>#{120 + i}</span>
            <Icon name={r.ok ? 'check' : 'x'} size={44} color={r.ok ? T.success : T.danger} stroke={3} />
            {r.fb ? <Icon name={r.fb === 'up' ? 'thumbUp' : 'thumbDown'} size={30} color={r.fb === 'up' ? T.success : T.danger} /> : <div style={{ height: 30 }} />}
          </div>
        )
      })}
      <div
        style={{
          position: 'absolute',
          left: 90 + scan * 4 * 186 + 78 - 40,
          top: 500,
          width: 80,
          height: 80,
          borderRadius: 40,
          background: `${T.fuchsia}26`,
          border: `2px solid ${T.fuchsia}`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          opacity: ease(frame, 56, 64) * (1 - ease(frame, 104, 112) * 0.6),
        }}
      >
        <Icon name="eye" size={44} color={T.fuchsia} />
      </div>
      <div style={{ position: 'absolute', top: 800, width: W, textAlign: 'center', fontFamily: MONO, fontSize: 30, color: T.dim, opacity: ease(frame, 60, 70) }}>
        one look every <span style={{ color: T.fuchsia, fontWeight: 700 }}>5</span> runs
      </div>
      {/* proposal */}
      <div
        style={{
          position: 'absolute',
          left: 70,
          top: 880,
          width: 940,
          borderRadius: 30,
          background: `linear-gradient(180deg, ${T.surface2}, ${T.surface})`,
          border: `2px solid ${applied ? TEAL : T.fuchsia}88`,
          boxShadow: `0 40px 100px -30px #000, 0 0 40px ${applied ? TEAL : T.fuchsia}22`,
          padding: 34,
          fontFamily: FONT,
          opacity: card,
          transform: `translateY(${(1 - card) * 120}px)`,
          display: 'flex',
          flexDirection: 'column',
          gap: 18,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
          <Icon name="sparkle" size={38} color={T.fuchsia} />
          <span style={{ fontSize: 36, fontWeight: 800, color: T.text }}>Proposal #12</span>
          <div style={{ flex: 1 }} />
          <span style={{ fontFamily: MONO, fontSize: 26, fontWeight: 700, color: T.fuchsia, background: `${T.fuchsia}1c`, borderRadius: 99, padding: '6px 16px' }}>confidence 0.82</span>
        </div>
        <div style={{ fontSize: 30, color: T.dim, lineHeight: 1.35 }}>
          <b style={{ color: T.text }}>Why:</b> 2 runs got negative feedback for unsourced replies.
        </div>
        <div style={{ fontFamily: MONO, fontSize: 27, color: TEAL, background: T.bg, borderRadius: 14, padding: '14px 18px', border: `2px solid ${T.border}` }}>
          + insert_between → "Source check"
        </div>
        <div style={{ display: 'flex', gap: 16, marginTop: 6 }}>
          <div
            style={{
              flex: 1,
              textAlign: 'center',
              padding: '18px 0',
              borderRadius: 16,
              background: TEAL,
              color: T.bg,
              fontWeight: 800,
              fontSize: 30,
              transform: `scale(${press})`,
              boxShadow: frame >= 160 && !applied ? `0 0 0 6px ${TEAL}55` : 'none',
            }}
          >
            {applied ? '✓ v4 applied' : 'Apply'}
          </div>
          <div style={{ flex: 1, textAlign: 'center', padding: '18px 0', borderRadius: 16, border: `2px solid ${T.border}`, color: T.dim, fontWeight: 700, fontSize: 30, opacity: applied ? 0.4 : 1 }}>
            Reject
          </div>
        </div>
      </div>
      {/* policy */}
      <div style={{ position: 'absolute', left: 140, top: 1340, width: 800, opacity: ease(frame, 130, 146) }}>
        <div style={{ fontFamily: FONT, fontSize: 26, fontWeight: 700, letterSpacing: 3, color: T.dim, textAlign: 'center', marginBottom: 14 }}>POLICY</div>
        <div style={{ position: 'relative', display: 'flex', borderRadius: 20, background: T.surface, border: `2px solid ${T.border}`, padding: 6, height: 84 }}>
          <div
            style={{
              position: 'absolute',
              top: 6,
              left: 6 + pos * (788 / 3),
              width: 788 / 3 - 4,
              height: 70,
              borderRadius: 16,
              background: T.fuchsia,
              boxShadow: `0 0 30px ${T.fuchsia}88`,
            }}
          />
          {['off', 'propose', 'auto'].map((m, i) => (
            <div key={m} style={{ position: 'relative', flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: FONT, fontSize: 30, fontWeight: 800, color: i === Math.round(pos) ? T.bg : T.dim }}>
              {m}
            </div>
          ))}
        </div>
      </div>
      <Caption at={150} top={1500} size={34}>
        “No change” is a perfectly good answer.
      </Caption>
    </>
  )
}

// --- 5. Safety rails -------------------------------------------------------------
const RailCard: React.FC<{ top: number; at: number; icon: IconName; color: string; title: string; children: React.ReactNode }> = ({ top, at, icon, color, title, children }) => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const t = pop(frame, fps, at, 14)
  return (
    <div
      style={{
        position: 'absolute',
        left: 70,
        top,
        width: 940,
        height: 250,
        borderRadius: 30,
        background: `linear-gradient(160deg, ${color}14, ${T.surface} 50%)`,
        border: `2px solid ${color}55`,
        padding: '28px 34px',
        fontFamily: FONT,
        opacity: Math.min(1, t * 1.4),
        transform: `translateX(${(1 - t) * -80}px)`,
        display: 'flex',
        flexDirection: 'column',
        gap: 20,
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
        <Icon name={icon} size={40} color={color} />
        <span style={{ fontSize: 38, fontWeight: 800, color: T.text }}>{title}</span>
      </div>
      {children}
    </div>
  )
}

const Safety: React.FC<{ duration: number }> = () => {
  const frame = useCurrentFrame()
  const { fps } = useVideoConfig()
  const nodes = Math.round(5 + 4 * ease(frame, 20, 70))
  const checks = ['one input / one output', 'reachability', 'default arm on loops', '≤ 8 ops per proposal']
  const reverted = frame >= 112
  const vers = reverted ? ['v1', 'v2', 'v3', 'v4', 'v5'] : ['v1', 'v2', 'v3', 'v4']
  return (
    <>
      <Eyebrow text="YOU STAY IN CONTROL" color={TEAL} />
      <Headline lines={['Budgets, validation,', <><Hl c={TEAL}>one-click</Hl> revert</>]} size={80} />
      <RailCard top={520} at={6} icon="gauge" color={T.warning} title="Growth budget">
        <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
          <div style={{ flex: 1, height: 26, borderRadius: 13, background: T.bg, border: `2px solid ${T.border}`, overflow: 'hidden' }}>
            <div style={{ width: `${(nodes / 16) * 100}%`, height: '100%', background: `linear-gradient(90deg, ${T.success}, ${T.warning})` }} />
          </div>
          <span style={{ fontFamily: MONO, fontSize: 32, fontWeight: 700, color: T.text }}>{nodes}/16</span>
        </div>
        <div style={{ fontSize: 26, color: T.dim }}>Agent and observer stay within it · hard cap 48 nodes</div>
      </RailCard>
      <RailCard top={800} at={22} icon="shield" color={T.info} title="Validation">
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '12px 20px' }}>
          {checks.map((c, i) => {
            const t = ease(frame, 40 + i * 8, 50 + i * 8)
            return (
              <div key={c} style={{ display: 'flex', alignItems: 'center', gap: 10, fontSize: 27, color: T.text, opacity: t }}>
                <Icon name="check" size={28} color={T.success} stroke={3} />
                {c}
              </div>
            )
          })}
        </div>
      </RailCard>
      <RailCard top={1080} at={38} icon="undo" color={TEAL} title="Revert">
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          {vers.map((v, i) => {
            const isHead = i === vers.length - 1
            const t = i === 4 ? pop(frame, fps, 112, 11) : 1
            return (
              <div
                key={v}
                style={{
                  fontFamily: MONO,
                  fontSize: 28,
                  fontWeight: 700,
                  padding: '10px 18px',
                  borderRadius: 14,
                  color: isHead ? T.bg : T.text,
                  background: isHead ? TEAL : T.bg,
                  border: `2px solid ${isHead ? TEAL : i === 2 && reverted ? TEAL : T.border}`,
                  transform: `scale(${t})`,
                }}
              >
                {v}
                {i === 4 ? <span style={{ fontWeight: 500 }}> = v3</span> : null}
              </div>
            )
          })}
          <div style={{ flex: 1 }} />
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              padding: '12px 22px',
              borderRadius: 14,
              border: `2px solid ${TEAL}`,
              color: TEAL,
              fontSize: 28,
              fontWeight: 800,
              transform: `scale(${frame >= 100 && frame < 110 ? 0.9 : 1})`,
              background: frame >= 100 && frame < 112 ? `${TEAL}33` : 'transparent',
            }}
          >
            <Icon name="undo" size={28} color={TEAL} stroke={2.6} /> Revert
          </div>
        </div>
        <div style={{ fontSize: 26, color: T.dim }}>A revert is a new version too · prompt versions restore the same way</div>
      </RailCard>
    </>
  )
}

// --- 6. Outro --------------------------------------------------------------------
const End: React.FC<{ duration: number }> = ({ duration }) => (
  <Outro duration={duration} title={<><Hl c={TEAL}>Self-improving</Hl> flows</>} tagline="You steer. Your agents learn." accent={TEAL} chips={['versions', 'Flow Observer', 'one-click revert']} />
)

const SCENES: FlowScene[] = [
  { id: 'hook', beats: 8, accent: TEAL, Comp: Hook },
  { id: 'versions', beats: 8, accent: T.info, Comp: Versions },
  { id: 'self', beats: 12, accent: T.accent, Comp: SelfEdit },
  { id: 'observer', beats: 12, accent: T.fuchsia, Comp: Observer },
  { id: 'safety', beats: 8, accent: TEAL, Comp: Safety },
  { id: 'outro', beats: 8, accent: TEAL, Comp: End, wrap: false },
]
export const EVOLUTION_FRAMES = SCENES.reduce((a, s) => a + s.beats, 0) * BEAT

export const FlowsEvolution: React.FC = () => <FlowVideo scenes={SCENES} beat={BEAT} music="dusty.wav" label="Evolution" accent2={T.accent} />

