import { useEffect, useState } from 'react'
import {
  X,
  Loader2,
  Copy,
  Check,
  ChevronRight,
  ChevronDown,
  ArrowRight,
  Waypoints,
} from 'lucide-react'
import { getActiveWorkspace } from '@/api/client'
import { api } from '@/api'
import { toast } from '@/shared/components'
import type { Agent, FlowMsg, FlowNode, FlowRun, FlowTraceEntry, TurnStep } from '@/types'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { UserBubble } from '@/features/chat/UserBubble'
import { TurnSteps } from '@/features/chat/TurnSteps'
import { subscribeFlowNodeStep } from '@/shared/lib/flowNodeStepBus'
import type { NodeStatus } from './flowGraph'

// CopyButton copies `text` to the clipboard, surfacing success through the
// app-wide toast (single feedback channel). A tiny local control so the node
// output can be lifted out without the full chat message footer (which is
// session/message-bound).
function CopyButton({ text, title }: { text: string; title: string }) {
  return (
    <button
      type="button"
      title={title}
      onClick={() => {
        void navigator.clipboard?.writeText(text).then(() => toast.info('Panoya kopyalandı'))
      }}
      className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
    >
      <Copy size={12} />
    </button>
  )
}

// AssistantBubble renders an assistant/model reply the same left-aligned way the
// chat does, with a hover copy action.
function AssistantBubble({ text }: { text: string }) {
  return (
    <div className="group relative rounded-2xl bg-[var(--color-surface-2)] px-4 py-3 text-sm leading-relaxed">
      <div className="absolute right-1.5 top-1.5 opacity-0 transition group-hover:opacity-100">
        <CopyButton text={text} title="Kopyala" />
      </div>
      <Markdown>{text}</Markdown>
    </div>
  )
}

// PriorThread shows the accumulated conversation an accumulate-mode agent node saw
// as prior context (thread[:threadLen]) — collapsed by default so the node's own
// exchange stays the focus, expandable to reveal what the agent actually read.
function PriorThread({ msgs, agents }: { msgs: FlowMsg[]; agents: Agent[] }) {
  const [open, setOpen] = useState(false)
  if (msgs.length === 0) return null
  return (
    <div className="rounded border border-[var(--color-border)]">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-1 px-2 py-1.5 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
      >
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        <span>Önceki bağlam (biriken konuşma)</span>
        <span className="ml-auto opacity-70">{msgs.length} mesaj</span>
      </button>
      {open && (
        <div className="space-y-2 px-2 pb-2">
          {msgs.map((m, i) =>
            m.role === 'assistant' ? (
              <AssistantBubble key={i} text={m.text} />
            ) : (
              <UserBubble key={i} text={m.text} agents={agents} />
            ),
          )}
        </div>
      )}
    </div>
  )
}

// BranchCard renders a branch node's routing decision: the value that was
// evaluated, the match mode, every arm, and which one matched (from the trace
// output label "→ <label>"). Turns "why did the flow go here?" into a glance.
function BranchCard({ node, entry }: { node: FlowNode; entry: FlowTraceEntry | undefined }) {
  const value = entry?.input ?? ''
  const label = (entry?.output ?? '').replace(/^→\s*/, '') // matched arm's `contains`, or "default"
  const mode = node.matchMode ?? 'contains'
  const arms = node.branches ?? []
  const modeLabel = mode === 'equals' ? 'eşittir' : mode === 'regex' ? 'regex' : 'içerir'
  return (
    <div className="space-y-3">
      <div className="rounded bg-[var(--color-surface-2)] p-3 text-sm">
        <div className="mb-1 text-xs text-[var(--color-text-dim)]">
          Değerlendirilen değer{node.jsonField ? ` (JSON alanı: ${node.jsonField})` : ''} · eşleşme:{' '}
          {modeLabel}
        </div>
        <div className="max-h-40 overflow-y-auto whitespace-pre-wrap break-words">
          {value || <span className="italic text-[var(--color-text-dim)]">boş</span>}
        </div>
      </div>
      <div className="space-y-1">
        {arms.map((b, i) => {
          const isDefault = b.contains === ''
          const matched = isDefault ? label === 'default' : b.contains === label
          return (
            <div
              key={i}
              className={`flex items-center gap-2 rounded border px-2 py-1.5 text-sm ${
                matched
                  ? 'border-[var(--color-success)] bg-[color:color-mix(in_srgb,var(--color-success)_12%,transparent)]'
                  : 'border-[var(--color-border)] opacity-70'
              }`}
            >
              {matched ? (
                <Check size={13} className="shrink-0 text-[var(--color-success)]" />
              ) : (
                <span className="w-[13px] shrink-0" />
              )}
              <span className="min-w-0 flex-1 truncate">
                {isDefault ? (
                  <span className="italic text-[var(--color-text-dim)]">varsayılan</span>
                ) : (
                  b.contains
                )}
              </span>
              {matched && b.next && (
                <span className="flex shrink-0 items-center gap-1 text-xs text-[var(--color-text-dim)]">
                  <ArrowRight size={12} /> {b.next}
                </span>
              )}
            </div>
          )
        })}
        {arms.length === 0 && (
          <div className="text-xs italic text-[var(--color-text-dim)]">Tanımlı dal yok.</div>
        )}
      </div>
    </div>
  )
}

// fmtDur renders a millisecond duration compactly (e.g. "820ms", "3.4s").
function fmtDur(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)}s`
}

// ParallelFanout shows a parallel node's children as a concurrency timeline: each
// child is a relative bar (start→end within the fan-out's window) so overlap and
// the critical path (longest bar) are obvious at a glance, plus a click to open
// each child's own chat view. Falls back to a plain list when no timestamps exist
// (older runs / children that never recorded bounds).
function ParallelFanout({
  node,
  traceByNode,
  onSelectNode,
}: {
  node: FlowNode
  traceByNode: Record<string, FlowTraceEntry>
  onSelectNode: (id: string) => void
}) {
  const children = node.parallel ?? []
  // The fan-out's time window across children that recorded bounds.
  const timed = children
    .map((cid) => traceByNode[cid])
    .filter((t): t is FlowTraceEntry => !!t && !!t.startMs && !!t.endMs)
  const t0 = timed.length ? Math.min(...timed.map((t) => t.startMs!)) : 0
  const t1 = timed.length ? Math.max(...timed.map((t) => t.endMs!)) : 0
  const span = Math.max(1, t1 - t0)

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
        <span>{children.length} eşzamanlı dal</span>
        {timed.length > 0 && <span className="ml-auto">toplam {fmtDur(span)}</span>}
      </div>
      {children.map((cid) => {
        const t = traceByNode[cid]
        const hasBar = !!t?.startMs && !!t?.endMs
        const dur = hasBar ? t!.endMs! - t!.startMs! : 0
        const leftPct = hasBar ? ((t!.startMs! - t0) / span) * 100 : 0
        const widthPct = hasBar ? Math.max(2, (dur / span) * 100) : 0
        // Critical path = the child whose end defines the window's end.
        const isCritical = hasBar && t!.endMs! === t1
        return (
          <button
            key={cid}
            type="button"
            onClick={() => onSelectNode(cid)}
            className="flex w-full items-center gap-2 rounded border border-[var(--color-border)] p-2 text-left text-sm transition hover:border-[var(--color-accent)]"
          >
            <span className="w-28 shrink-0 truncate font-medium">{t?.title || cid}</span>
            {hasBar ? (
              <span className="relative h-4 min-w-0 flex-1 rounded bg-[var(--color-surface-2)]">
                <span
                  className="absolute top-0 h-4 rounded"
                  style={{
                    left: `${leftPct}%`,
                    width: `${widthPct}%`,
                    background: isCritical ? 'var(--color-danger)' : 'var(--color-accent)',
                  }}
                  title={`${fmtDur(dur)}${isCritical ? ' — kritik yol' : ''}`}
                />
              </span>
            ) : (
              <span className="min-w-0 flex-1 truncate text-[var(--color-text-dim)]">
                {t?.output ?? <span className="italic">çıktı yok</span>}
              </span>
            )}
            {hasBar && (
              <span className="w-14 shrink-0 text-right text-xs text-[var(--color-text-dim)]">
                {fmtDur(dur)}
              </span>
            )}
            <ChevronRight size={14} className="shrink-0 text-[var(--color-text-dim)]" />
          </button>
        )
      })}
      {children.length === 0 && (
        <div className="text-xs italic text-[var(--color-text-dim)]">Tanımlı paralel dal yok.</div>
      )}
    </div>
  )
}

interface Props {
  run: FlowRun
  // The selected graph node (type/title/agentId) and its trace entry (input/output).
  node: FlowNode
  entry: FlowTraceEntry | undefined
  // The node's current run status (drives live vs. persisted step source).
  status: NodeStatus | undefined
  // Accumulated thread for the run (accumulate mode) — sliced to entry.threadLen
  // to show an agent node's prior context.
  thread: FlowMsg[] | undefined
  // Trace entries keyed by node id (for the parallel fan-out child lookup).
  traceByNode: Record<string, FlowTraceEntry>
  agents: Agent[]
  onClose: () => void
  // Switch the inspected node (used by the parallel fan-out to open a child).
  onSelectNode: (id: string) => void
}

// RunNodeInspector renders one selected flow-run node with a view tailored to its
// type: an agent node as a chat-like exchange (prior context + input bubble +
// tool/thinking steps + output), a branch node as a routing decision card, a
// parallel node as a fan-out of its children, and everything else as a plain
// output card. It reuses the chat screen's own UserBubble/TurnSteps/Markdown so
// agent turns look and behave like normal chat. (Distinct from NodeInspector,
// which is the flow *editor's* node-config panel.)
export function RunNodeInspector({
  run,
  node,
  entry,
  status,
  thread,
  traceByNode,
  agents,
  onClose,
  onSelectNode,
}: Props) {
  const isAgent = node.type === 'agent'
  const running = status === 'running'
  // `steps` = the persisted sidecar (authoritative once the node finishes).
  const [steps, setSteps] = useState<TurnStep[] | null>(null)
  // `liveSteps` = frames streamed while the node is still running (before its
  // sidecar exists). Reset whenever the node stops running / selection changes.
  const [liveSteps, setLiveSteps] = useState<TurnStep[]>([])
  const [loading, setLoading] = useState(false)

  // Fetch the node's captured steps per (run, node) and again when it finishes
  // (a mid-run open sees []; the completion refetch backfills the full trace).
  // Only agent nodes have a steps sidecar; other node types skip the request.
  useEffect(() => {
    if (!isAgent) {
      setSteps(null)
      return
    }
    let alive = true
    setLoading(true)
    api
      .flowRunNodeSteps(run.id, node.id)
      .then((s) => {
        if (alive) setSteps(s)
      })
      .catch(() => {
        // A missing sidecar (still running, or pre-capture run) is not an error;
        // the input/output bubbles still render.
        if (alive) setSteps([])
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
    // Re-run on `running` transition so a node that finishes while open backfills.
  }, [run.id, node.id, isAgent, running])

  // Live steps: while the node runs, append each streamed frame. Cleared when the
  // node is no longer running (the sidecar refetch above then owns the trace).
  useEffect(() => {
    if (!isAgent || !running) {
      setLiveSteps([])
      return
    }
    return subscribeFlowNodeStep(run.id, (frame) => {
      if (frame.nodeId === node.id) setLiveSteps((prev) => [...prev, frame.step])
    })
  }, [run.id, node.id, isAgent, running])

  const input = entry?.input ?? ''
  const output = entry?.output ?? ''
  // While running, the sidecar isn't written yet → show live frames. Once done,
  // the sidecar is authoritative (and covers frames missed before the inspector
  // opened), so prefer it.
  const shownSteps = running ? [...(steps ?? []), ...liveSteps] : (steps ?? [])
  // Prior context this agent node ran with (accumulate mode): thread[:threadLen].
  const priorMsgs = thread && entry?.threadLen ? thread.slice(0, entry.threadLen) : []

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Header: which node, plus a back button to the full step list. */}
      <div className="flex flex-shrink-0 items-center gap-2 px-4 py-2 text-xs">
        <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[var(--color-text-dim)]">
          {node.type}
        </span>
        <span className="truncate font-medium">{node.title || node.id}</span>
        {node.type === 'coordinator' && (
          <button
            type="button"
            onClick={() => {
              api
                .trajectoryByNode(run.id, node.id)
                .then((r) => {
                  const ws = getActiveWorkspace()
                  window.location.hash = `#/w/${ws ?? ''}/rota/${r.trajectoryId}`
                })
                .catch(() => toast.info('Bu koordinatör düğümünün henüz rotası yok'))
            }}
            title="Bu koordinatörün rotasını Rota ekranında aç"
            className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            data-testid="run-node-rota"
          >
            <Waypoints size={12} /> Rota
          </button>
        )}
        <button
          type="button"
          onClick={onClose}
          title="Tüm adımlara dön"
          className="ml-auto flex items-center gap-1 rounded px-2 py-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          <X size={13} /> Tüm adımlar
        </button>
      </div>

      <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 pb-4">
        {node.type === 'branch' ? (
          <BranchCard node={node} entry={entry} />
        ) : node.type === 'parallel' ? (
          <ParallelFanout node={node} traceByNode={traceByNode} onSelectNode={onSelectNode} />
        ) : node.type === 'coordinator' ? (
          // A coordinator node's tool/thinking steps live in its OWN coordinator
          // session (and its workers'), not in this run's per-node sidecar — so
          // show the delegated goal and the final report, and point at the session.
          <>
            {input ? (
              <UserBubble text={input} agents={agents} />
            ) : (
              <div className="text-xs italic text-[var(--color-text-dim)]">
                Girdi kaydı yok (bu düğüm henüz tamamlanmadı veya eski bir koşu).
              </div>
            )}
            {running && (
              <div className="flex items-center gap-2 text-xs text-[var(--color-accent)]">
                <Loader2 size={13} className="animate-spin" /> Koordinatör çalışıyor — workerlar
                bekleniyor…
              </div>
            )}
            {output && <AssistantBubble text={output} />}
            <p className="text-[11px] text-[var(--color-text-dim)]">
              Worker adımları koordinatörün kendi oturumunda; Oturumlar ekranından (“Akış
              Koordinatörü”) izlenebilir.
            </p>
          </>
        ) : !isAgent ? (
          // Other non-agent nodes (delay/transform/loop/…): plain output card.
          <div className="rounded bg-[var(--color-surface-2)] p-3 text-sm">
            {output ? (
              <div className="whitespace-pre-wrap break-words">{output}</div>
            ) : (
              <span className="italic text-[var(--color-text-dim)]">
                Bu düğüm tipinde ({node.type}) mesaj alışverişi yok.
              </span>
            )}
          </div>
        ) : (
          <>
            {/* Accumulate mode: the prior conversation the agent actually saw. */}
            {priorMsgs.length > 0 && <PriorThread msgs={priorMsgs} agents={agents} />}

            {/* Input as a user bubble (the rendered prompt the agent received). */}
            {input ? (
              <UserBubble text={input} agents={agents} />
            ) : (
              <div className="text-xs italic text-[var(--color-text-dim)]">
                Girdi kaydı yok (bu düğüm henüz tamamlanmadı veya eski bir koşu).
              </div>
            )}

            {/* Agent's tool/thinking steps, rendered exactly like a chat turn.
                Live while running, then from the persisted sidecar. */}
            {loading && shownSteps.length === 0 && (
              <div className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
                <Loader2 size={13} className="animate-spin" /> Adımlar yükleniyor…
              </div>
            )}
            {shownSteps.length > 0 && <TurnSteps steps={shownSteps} />}
            {running && (
              <div className="flex items-center gap-2 text-xs text-[var(--color-accent)]">
                <Loader2 size={13} className="animate-spin" /> Çalışıyor…
              </div>
            )}

            {/* Output as the assistant reply, with a copy action. */}
            {output && <AssistantBubble text={output} />}
          </>
        )}
      </div>
    </div>
  )
}
