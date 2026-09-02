// Per-trajectory zoom (Rota F1b): phase columns × lanes over the pure
// trajectoryLayout. Declared phases are the columns (the plan), lanes are the
// sessions / runs / gates that actually happened under them (the facts), ghost
// nodes are dashed. Plain SVG like the workspace canvas; the graph is re-read
// whenever the stream announces a new revision (useTrajectory).
import { useMemo, useState } from 'react'
import { ArrowLeft, GitFork, MessageSquare, Sparkles } from 'lucide-react'
import { api } from '@/api'
import { Badge, toast } from '@/shared/components'
import { SKIP_REASON_LABEL } from '@/features/schedules/fireMeta'
import type { Trajectory, TrajectoryEdge, TrajectoryNode } from '@/types/trajectory'
import type { RotaSelection } from './RotaCanvas'
import {
  layoutTrajectory,
  phaseGlyph,
  phaseId,
  trajectoryProgress,
  type TrajPlaced,
} from './trajectoryLayout'
import { STATUS_LABEL, STATUS_TONE } from './trajectoryStatus'
import { fmtDurationSec, fmtTokens } from './trajectoryFormat'
import { useTrajectory } from './useTrajectory'
import { PhaseActions } from './PhaseActions'
import { ForkModal } from './ForkModal'

interface Props {
  trajectoryId: string
  width: number
  selected: RotaSelection | null
  onSelect: (sel: RotaSelection | null) => void
  onOpenSession?: (sessionId: string) => void
  onOpenFlowRun?: (flowId: string) => void
  onBack: () => void
}

const COL_W = 210
const ROW_H = 46
const LABEL_W = 150
const HEAD_H = 54
const NODE_H = 26
const PAD = 12

const EDGE_STYLE: Record<TrajectoryEdge['kind'], { stroke: string; dash?: string }> = {
  next: { stroke: 'var(--color-border)' },
  spawned: { stroke: '#f97316' },
  reported: { stroke: '#a855f7', dash: '4 3' },
  fired: { stroke: '#f59e0b' },
  feeds: { stroke: 'var(--color-text-dim)', dash: '2 3' },
  blocked_by: { stroke: '#ef4444', dash: '1 3' },
  forked_from: { stroke: '#a855f7' },
}

function nodeFill(n: TrajectoryNode): string {
  switch (n.state) {
    case 'active':
      return 'var(--color-accent)'
    case 'done':
      return '#10b981'
    case 'failed':
      return '#ef4444'
    case 'skipped':
      return 'var(--color-border)'
    case 'ghost':
      return 'transparent'
    default:
      return 'var(--color-text-dim)'
  }
}

function nodeGlyph(n: TrajectoryNode): string {
  switch (n.kind) {
    case 'flowrun':
      return '▶'
    case 'gate':
      return '⏸'
    case 'automation':
      return '⚡'
    case 'optimizer':
      return '✦'
    default:
      return n.state === 'active' ? '●' : '▣'
  }
}

function nodeLabel(n: TrajectoryNode): string {
  if (n.label) return n.label
  if (n.refId) return n.refId
  return n.id
}

// nodeText is the in-cell caption: the label, plus — for an automation that
// did not fire — the reason ("neden ateşlenmedi") in the user's words.
function nodeText(n: TrajectoryNode): string {
  const label = nodeLabel(n)
  if (n.kind === 'automation' && (n.state === 'skipped' || n.state === 'failed') && n.reason) {
    return `${label} · ${SKIP_REASON_LABEL[n.reason] ?? n.reason}`
  }
  if (n.kind === 'automation' && n.state === 'ghost') return `${label} · bekliyor`
  return label
}

// selectionFor maps a graph node to what the side panel can project: a gate
// stands for the session that asked; an optimizer has no projection yet.
function selectionFor(n: TrajectoryNode, t: Trajectory): RotaSelection | null {
  switch (n.kind) {
    case 'session':
      return n.refId ? { kind: 'session', id: n.refId } : null
    case 'flowrun':
      return n.refId ? { kind: 'flowrun', id: n.refId } : null
    case 'automation':
      return n.refId ? { kind: 'automation', id: n.refId } : null
    case 'gate': {
      const asker = t.edges.find((e) => e.to === n.id && e.kind === 'blocked_by')
      const from = asker && t.nodes.find((x) => x.id === asker.from)
      return from?.refId ? { kind: 'session', id: from.refId } : null
    }
    default:
      return null
  }
}

export function RotaTrajectoryView({
  trajectoryId,
  width,
  selected,
  onSelect,
  onOpenSession,
  onOpenFlowRun,
  onBack,
}: Props) {
  const { trajectory: t, loading, error } = useTrajectory({ id: trajectoryId })
  const layout = useMemo(() => (t ? layoutTrajectory(t) : null), [t])
  // Canvas actions (F5): the phase column the user picked, and the fork modal.
  const [pickedPhase, setPickedPhase] = useState<string | null>(null)
  const [fork, setFork] = useState<{ id: string; title?: string } | null>(null)
  const selectedSession =
    selected?.kind === 'session' && t
      ? t.nodes.find((n) => n.kind === 'session' && n.refId === selected.id)
      : undefined

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-3 py-1.5 text-xs">
        <button
          type="button"
          onClick={onBack}
          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          title="Workspace şeritlerine dön"
        >
          <ArrowLeft size={13} /> workspace
        </button>
        <span className="font-mono font-medium">{trajectoryId}</span>
        {t && (
          <>
            <span className="text-[var(--color-text-dim)]">{t.templateRef || 'plansız'}</span>
            <Badge tone={STATUS_TONE[t.status]}>{STATUS_LABEL[t.status]}</Badge>
            <span className="text-[var(--color-text-dim)]">rev {t.revision}</span>
            <PhaseProgress t={t} />
            {t.summary && <SummaryChips s={t.summary} />}
            {layout && layout.ghosts > 0 && (
              <span
                className="text-[var(--color-text-dim)]"
                title="İlan edilmiş ama henüz gerçekleşmemiş düğümler"
              >
                {layout.ghosts} hayalet
              </span>
            )}
            <span className="ml-auto flex items-center gap-1.5">
              {t.templateRef && (
                <button
                  type="button"
                  onClick={() => {
                    const slug = t.templateRef!.replace(/@[^@]*$/, '')
                    api
                      .optimizeRecipe(slug)
                      .then((r) =>
                        toast.info(
                          r.ran
                            ? `✦ Optimizer: ${r.proposals.length} öneri (${r.dropped} elendi) — İçgörü ▸ recipe-opt`
                            : `Optimizer çalışmadı: ${r.skipped ?? '—'}`,
                        ),
                      )
                      .catch((e) => toast.error(e instanceof Error ? e.message : String(e)))
                  }}
                  className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                  title="Bu rotanın reçetesi için optimizer'ı şimdi çalıştır (öneri üretir, uygulamaz)"
                >
                  <Sparkles size={12} /> optimize et
                </button>
              )}
              {selected?.kind === 'session' && (
                <button
                  type="button"
                  onClick={() => setFork({ id: selected.id, title: selectedSession?.label })}
                  className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                  title="Seçili oturumun altında worker aç (buradan çatalla)"
                  data-testid="fork-here"
                >
                  <GitFork size={12} /> buradan çatalla
                </button>
              )}
              <button
                type="button"
                onClick={() => onOpenSession?.(t.rootSessionId)}
                className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                title="Kök oturumun sohbetini aç"
              >
                <MessageSquare size={12} /> kök sohbet
              </button>
            </span>
          </>
        )}
        {loading && !t && <span className="text-[var(--color-text-dim)]">yükleniyor…</span>}
        {error && <span className="text-[var(--color-danger)]">{error}</span>}
      </div>
      {t && <PhaseActions trajectory={t} phase={pickedPhase} />}
      {fork && (
        <ForkModal sessionId={fork.id} sessionTitle={fork.title} onClose={() => setFork(null)} />
      )}
      <div className="min-h-0 flex-1 overflow-auto">
        {t && layout && (
          <TrajectorySvg
            t={t}
            layout={layout}
            width={width}
            selected={selected}
            onSelect={onSelect}
            onOpenSession={onOpenSession}
            onOpenFlowRun={onOpenFlowRun}
            trajectoryId={trajectoryId}
            onPickPhase={setPickedPhase}
          />
        )}
      </div>
    </div>
  )
}

// SummaryChips renders the deterministic end-of-run digest (F3) inline:
// duration, tokens / cost, workers (failed), gate wait, what the plan declared
// but never happened.
function SummaryChips({ s }: { s: NonNullable<Trajectory['summary']> }) {
  const chip =
    'rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]'
  return (
    <span className="flex flex-wrap items-center gap-1" data-testid="trajectory-summary">
      <span className={chip} title="Kök oturum açılışından bitişe">
        ⏱ {fmtDurationSec(s.durationSec)}
      </span>
      <span
        className={chip}
        title={`Bağlı oturumların toplam tokenı${s.priced ? '' : ' (bazı modeller fiyatsız)'}`}
      >
        {fmtTokens(s.tokens)} token
        {s.costUsd > 0 ? ` · $${s.costUsd.toFixed(2)}${s.priced ? '' : '~'}` : ''}
      </span>
      <span className={chip} title="Worker oturumları (başarısız)">
        {s.sessions} worker{s.failedSessions ? ` · ${s.failedSessions} ✗` : ''}
      </span>
      {s.flowRuns > 0 && (
        <span className={chip}>
          {s.flowRuns} koşu{s.failedRuns ? ` · ${s.failedRuns} ✗` : ''}
        </span>
      )}
      {s.gates > 0 && (
        <span className={chip} title="İnsan kapılarında geçen süre">
          ⏸ {s.gates} kapı · {fmtDurationSec(s.gateWaitSec)}
        </span>
      )}
      {s.unannounced > 0 && (
        <span className={chip} title="Bir faza bağlı olmadan açılan oturum / koşular">
          {s.unannounced} plansız
        </span>
      )}
      {s.ghostPhases && s.ghostPhases.length > 0 && (
        <span className={chip} title="İlan edilip hiç başlamayan fazlar">
          ◌ {s.ghostPhases.join(', ')}
        </span>
      )}
      {s.unfiredWatchers && s.unfiredWatchers.length > 0 && (
        <span
          className="rounded bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-warning)]"
          title="İlan edilip hiç ateşlenmeyen izleyiciler"
        >
          ⚡ sessiz: {s.unfiredWatchers.join(', ')}
        </span>
      )}
    </span>
  )
}

function PhaseProgress({ t }: { t: Trajectory }) {
  const p = trajectoryProgress(t)
  if (p.total === 0) return <span className="text-[var(--color-text-dim)]">faz ilan edilmedi</span>
  return (
    <span
      className="text-[var(--color-text-dim)]"
      title={p.active ? `Aktif faz: ${p.active}` : undefined}
    >
      {p.done}/{p.total} faz{p.active ? ` · ${p.active}` : ''}
    </span>
  )
}

interface SvgProps {
  t: Trajectory
  layout: ReturnType<typeof layoutTrajectory>
  width: number
  selected: RotaSelection | null
  onSelect: (sel: RotaSelection | null) => void
  onOpenSession?: (sessionId: string) => void
  onOpenFlowRun?: (flowId: string) => void
  trajectoryId: string
  onPickPhase: (phase: string | null) => void
}

function TrajectorySvg({
  t,
  layout,
  width,
  selected,
  onSelect,
  onOpenSession,
  onOpenFlowRun,
  trajectoryId,
  onPickPhase,
}: SvgProps) {
  const { columns, lanes, nodes, edges, root, rootToCol, activeCol } = layout
  const colW = Math.max(COL_W, Math.floor((width - LABEL_W - PAD) / Math.max(1, columns.length)))
  const svgW = LABEL_W + columns.length * colW + PAD
  const height = HEAD_H + lanes.length * ROW_H + PAD
  const laneY = useMemo(
    () => new Map(lanes.map((l, i) => [l, HEAD_H + i * ROW_H + ROW_H / 2])),
    [lanes],
  )
  const colX = (c: number) => LABEL_W + c * colW

  // Cell occupancy: nodes sharing (col, lane) split the cell width.
  const cellSize = useMemo(() => {
    const m = new Map<string, number>()
    for (const n of nodes) {
      const k = `${n.col}:${n.lane}`
      m.set(k, (m.get(k) ?? 0) + 1)
    }
    return m
  }, [nodes])
  const nodeBox = (p: TrajPlaced) => {
    const count = cellSize.get(`${p.col}:${p.lane}`) ?? 1
    const inner = colW - 16
    const w = Math.max(36, (inner - (count - 1) * 4) / count)
    const x = colX(p.col) + 8 + p.slot * (w + 4)
    const y = (laneY.get(p.lane) ?? 0) - NODE_H / 2
    return { x, y, w, h: NODE_H, cx: x + w / 2, cy: y + NODE_H / 2 }
  }
  const boxById = useMemo(() => {
    const m = new Map<string, ReturnType<typeof nodeBox>>()
    for (const p of nodes) m.set(p.node.id, nodeBox(p))
    return m
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes, colW, laneY, cellSize])

  const rootY = laneY.get(0) ?? HEAD_H + ROW_H / 2
  // Edges touching the root band anchor on the band at the other end's column.
  const anchor = (id: string, otherId: string) => {
    const b = boxById.get(id)
    if (b) return { x: b.cx, y: b.cy }
    if (root && id === root.id) {
      const other = boxById.get(otherId)
      return { x: other ? other.cx : colX(0) + colW / 2, y: rootY }
    }
    return null
  }
  // Lane labels: the first node bound on that lane names it.
  const laneLabel = (lane: number): string => {
    if (lane === 0) return root ? nodeLabel(root) : 'kök'
    const first = nodes.find((n) => n.lane === lane)
    return first ? nodeLabel(first.node) : `şerit ${lane}`
  }
  const isSel = (sel: RotaSelection | null) =>
    !!sel && !!selected && sel.kind === selected.kind && sel.id === selected.id

  return (
    <svg
      width={svgW}
      height={height}
      className="select-none text-[11px]"
      role="img"
      aria-label={`Rota ${trajectoryId} faz grafiği`}
      onClick={(e) => {
        if (e.target === e.currentTarget) onSelect(null)
      }}
    >
      {/* Column bands + headers */}
      {columns.map((c) => (
        <g key={c.id}>
          <rect
            x={colX(c.index)}
            y={0}
            width={colW}
            height={height}
            fill={
              c.index === activeCol
                ? 'var(--color-accent-soft)'
                : c.index % 2
                  ? 'var(--color-surface-2)'
                  : 'transparent'
            }
            opacity={c.index === activeCol ? 0.35 : 0.5}
          />
          <line
            x1={colX(c.index)}
            x2={colX(c.index)}
            y1={0}
            y2={height}
            stroke="var(--color-border)"
          />
          <g
            className={c.phase ? 'cursor-pointer' : undefined}
            onClick={(ev) => {
              ev.stopPropagation()
              onSelect({ kind: 'trajectory', id: trajectoryId })
              onPickPhase(c.phase ? phaseId(c.id) : null)
            }}
          >
            <title>
              {c.phase
                ? `${phaseId(c.id)} · ${c.state}${c.profile ? ` · ${c.profile}` : ''}${c.gate ? ` · kapı ${c.gate.kind}${c.gate.value ? ` "${c.gate.value}"` : ''}` : ''}${c.optional ? ' · isteğe bağlı' : ''}${c.phase.reason ? ` · ${c.phase.reason}` : ''}`
                : 'Bir faza bağlı olmayan düğümler'}
            </title>
            <text
              x={colX(c.index) + 8}
              y={20}
              fill="var(--color-text)"
              className="font-medium"
              fontSize={12}
            >
              <tspan
                fill={
                  c.state === 'failed'
                    ? '#ef4444'
                    : c.state === 'active'
                      ? 'var(--color-accent)'
                      : c.state === 'done'
                        ? '#10b981'
                        : 'var(--color-text-dim)'
                }
              >
                {phaseGlyph(c.state)}
              </tspan>{' '}
              {c.label}
              {c.optional && <tspan fill="var(--color-text-dim)"> (isteğe bağlı)</tspan>}
            </text>
            <text x={colX(c.index) + 8} y={36} fill="var(--color-text-dim)">
              {c.profile ?? ''}
              {c.gate
                ? `${c.profile ? ' · ' : ''}⛩ ${c.gate.kind}${c.gate.value ? ` ${c.gate.value.slice(0, 18)}` : ''}`
                : ''}
            </text>
          </g>
        </g>
      ))}
      <line x1={0} x2={svgW} y1={HEAD_H - 4} y2={HEAD_H - 4} stroke="var(--color-border)" />

      {/* Lane rows + labels */}
      {lanes.map((l, i) => (
        <g key={l}>
          <rect
            x={0}
            y={HEAD_H + i * ROW_H}
            width={svgW}
            height={ROW_H}
            fill={i % 2 ? 'var(--color-surface-2)' : 'transparent'}
            opacity={0.3}
          />
          <text
            x={8}
            y={(laneY.get(l) ?? 0) + 4}
            fill={l === 0 ? 'var(--color-text)' : 'var(--color-text-dim)'}
            className={l === 0 ? 'font-medium' : ''}
          >
            <title>{l === 0 ? `kök · ${t.rootSessionId}` : `şerit ${l}`}</title>
            {laneLabel(l).slice(0, 20)}
          </text>
        </g>
      ))}

      {/* Root band across the columns the plan has reached */}
      {root && (
        <g
          className="cursor-pointer"
          onClick={(ev) => {
            ev.stopPropagation()
            onSelect({ kind: 'session', id: root.refId ?? t.rootSessionId })
          }}
          onDoubleClick={(ev) => {
            ev.stopPropagation()
            onOpenSession?.(root.refId ?? t.rootSessionId)
          }}
        >
          <title>{`${nodeLabel(root)} · ${root.state}${root.reason ? ` · ${root.reason}` : ''}`}</title>
          <rect
            x={colX(0) + 4}
            y={rootY - 7}
            width={colX(rootToCol) + colW - 8 - colX(0)}
            height={14}
            rx={7}
            fill={nodeFill(root)}
            opacity={0.85}
            stroke={
              isSel({ kind: 'session', id: root.refId ?? t.rootSessionId })
                ? 'var(--color-text)'
                : 'none'
            }
            strokeWidth={1.5}
          />
          {root.state === 'active' && (
            <circle cx={colX(rootToCol) + colW - 12} cy={rootY} r={3} fill="#fff" opacity={0.9}>
              <animate
                attributeName="opacity"
                values="0.9;0.2;0.9"
                dur="1.6s"
                repeatCount="indefinite"
              />
            </circle>
          )}
        </g>
      )}

      {/* Edges */}
      {edges.map((e, i) => {
        const a = anchor(e.from, e.to)
        const b = anchor(e.to, e.from)
        if (!a || !b) return null
        const st = EDGE_STYLE[e.kind]
        const dx = Math.max(24, Math.abs(b.x - a.x) / 2)
        const back = e.kind === 'reported'
        const d = back
          ? `M ${a.x} ${a.y} C ${a.x + dx} ${a.y}, ${b.x + dx} ${b.y}, ${b.x} ${b.y}`
          : `M ${a.x} ${a.y} C ${a.x} ${(a.y + b.y) / 2}, ${b.x} ${(a.y + b.y) / 2}, ${b.x} ${b.y}`
        return (
          <path
            key={i}
            d={d}
            fill="none"
            stroke={st.stroke}
            strokeWidth={1.5}
            strokeDasharray={st.dash}
            opacity={e.origin === 'declared' ? 0.5 : 0.9}
          >
            <title>{`${e.kind}: ${e.from} → ${e.to}`}</title>
          </path>
        )
      })}

      {/* Nodes */}
      {nodes.map((p) => {
        const b = boxById.get(p.node.id)!
        const sel = selectionFor(p.node, t)
        const n = p.node
        const run = n.kind === 'flowrun'
        return (
          <g
            key={n.id}
            className={sel ? 'cursor-pointer' : undefined}
            onClick={(ev) => {
              ev.stopPropagation()
              onSelect(sel)
            }}
            onDoubleClick={(ev) => {
              ev.stopPropagation()
              if (n.kind === 'session' && n.refId) onOpenSession?.(n.refId)
              else if (run && n.refId) onOpenFlowRun?.(n.refId)
            }}
          >
            <title>{`${n.kind} · ${nodeLabel(n)} · ${n.state}${n.origin === 'declared' ? ' · ilan' : ''}${n.reason ? ` · ${n.reason}` : ''}`}</title>
            <rect
              x={b.x}
              y={run ? b.y + 6 : b.y}
              width={b.w}
              height={run ? b.h - 12 : b.h}
              rx={n.kind === 'gate' ? 13 : 5}
              fill={nodeFill(n)}
              opacity={p.ghost ? 1 : 0.9}
              stroke={
                isSel(sel)
                  ? 'var(--color-text)'
                  : p.ghost
                    ? 'var(--color-text-dim)'
                    : n.kind === 'gate'
                      ? '#f59e0b'
                      : 'none'
              }
              strokeWidth={isSel(sel) ? 1.5 : 1}
              strokeDasharray={p.ghost ? '4 3' : undefined}
            />
            <text
              x={b.x + 6}
              y={b.cy + 4}
              fill={
                p.ghost
                  ? 'var(--color-text-dim)'
                  : n.state === 'pending' || n.state === 'skipped'
                    ? 'var(--color-text)'
                    : '#fff'
              }
              clipPath={`inset(0 0 0 0)`}
            >
              {nodeGlyph(n)} {nodeText(n).slice(0, Math.max(4, Math.floor(b.w / 7) - 2))}
            </text>
            {n.state === 'active' && n.kind === 'session' && (
              <circle cx={b.x + b.w - 8} cy={b.cy} r={3} fill="#fff" opacity={0.9}>
                <animate
                  attributeName="opacity"
                  values="0.9;0.2;0.9"
                  dur="1.6s"
                  repeatCount="indefinite"
                />
              </circle>
            )}
          </g>
        )
      })}
    </svg>
  )
}
