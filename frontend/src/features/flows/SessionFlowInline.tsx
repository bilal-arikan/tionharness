import { useEffect, useMemo, useState } from 'react'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Flow, FlowRun, Message } from '@/types'
import { RunView } from './RunView'
import { sessionToFlowRun } from './sessionToFlow'

interface Props {
  messages: Message[]
  agents: Agent[]
  fallbackAgentId: string
  sessionId: string
  sessionTitle: string
  // The session's origin. When kind==='flow' with a sourceId (the flow id), this
  // session is a real flow run's transcript — we resolve and show that run's REAL
  // graph/layout instead of reifying the transcript into a synthetic linear chain.
  sessionKind: string
  sourceId?: string
  // Session creation time — used to pair a flow session with its run when the run
  // predates the explicit run↔session link (each per-run session is created at
  // ~the same second as its run, so nearest createdAt identifies it).
  sessionCreatedAt?: number
  onBack: () => void
}

// resolveState: 'loading' while we look up the real flow+run for a flow session;
// a {flow,run} pair once found; null to fall back to transcript reification (not
// a flow session, or the run predates the run↔session link / flow was deleted).
type Resolved = 'loading' | { flow: Flow; run: FlowRun } | null

// SessionFlowInline shows the current session as a flow run, inline in the chat
// area (not a modal). For a REAL flow-run transcript (Session.kind "flow") it
// resolves the actual flow + run and renders their true graph — same layout as
// the Koşular tab / editor. For any other chat session it reifies the transcript
// into a linear agent-node chain (RunView with node prompts on the canvas and
// replies in the step trace). "← Sohbete dön" returns to the transcript.
export function SessionFlowInline({
  messages,
  agents,
  fallbackAgentId,
  sessionId,
  sessionTitle,
  sessionKind,
  sourceId,
  sessionCreatedAt,
  onBack,
}: Props) {
  const isFlowSession = sessionKind === 'flow' && !!sourceId
  // Resolve the real flow + run behind a flow session. Starts 'loading' for a
  // flow session (so we don't flash the reified view first), null otherwise.
  const [resolved, setResolved] = useState<Resolved>(isFlowSession ? 'loading' : null)
  useEffect(() => {
    if (!isFlowSession || !sourceId) {
      setResolved(null)
      return
    }
    let cancelled = false
    setResolved('loading')
    // listFlows (not getFlow) so this resolves on any backend that already ships
    // the flow-runs API — no new endpoint / restart required for it to work.
    Promise.all([api.listFlows(), api.listFlowRuns(sourceId)])
      .then(([flows, runs]) => {
        if (cancelled) return
        const flow = flows.find((f) => f.id === sourceId) ?? null
        if (!flow) {
          setResolved(null) // flow deleted → reify from the transcript
          return
        }
        let run = runs.find((r) => r.sessionId === sessionId) ?? null
        // The run↔session link is stamped the moment the run row is created (R1,
        // _Docs/77), so an exact match is the only path for anything recorded
        // since. The nearest-createdAt pairing survives ONLY for a store whose
        // runs of this flow all predate the link (no run carries a sessionId);
        // once any run is linked, a miss means "deleted" and falls back to reify.
        const linkedStore = runs.some((r) => !!r.sessionId)
        if (!run && !linkedStore && sessionCreatedAt) {
          let bestDiff = Infinity
          for (const r of runs) {
            const diff = Math.abs((r.createdAt ?? 0) - sessionCreatedAt)
            if (diff < bestDiff) {
              bestDiff = diff
              run = r
            }
          }
          if (bestDiff > 5) run = null // no run within 5s → not this session's run
        }
        // No matching run (deleted, or none) → fall back to reification.
        setResolved(run ? { flow, run } : null)
      })
      .catch(() => {
        // Flow deleted or fetch failed → reify from the transcript instead.
        if (!cancelled) setResolved(null)
      })
    return () => {
      cancelled = true
    }
  }, [isFlowSession, sourceId, sessionId, sessionCreatedAt])

  // Transcript reification — only needed when we're NOT showing a real flow run.
  const { graph, flow, run } = useMemo(
    () => sessionToFlowRun(messages, fallbackAgentId, sessionId, sessionTitle),
    [messages, fallbackAgentId, sessionId, sessionTitle],
  )
  // The graph always carries a start node; "empty" means no real (non-start) nodes.
  const realNodes = graph.nodes.filter((n) => n.type !== 'start')
  const empty = realNodes.length === 0

  // A resolved real flow run: show its true graph/layout. This is the exact same
  // RunView as the Koşular tab.
  if (resolved && resolved !== 'loading') {
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="flex items-center justify-between gap-2 border-b border-[var(--color-border)] px-4 py-2">
          <button
            onClick={onBack}
            className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            title="Sohbet transkriptine dön"
          >
            <ArrowLeft size={15} className="shrink-0" />
            <span>Sohbete dön</span>
          </button>
          <span className="truncate text-xs text-[var(--color-text-dim)]">
            Kayıtlı akış görünümü — {resolved.flow.name}
          </span>
          <span className="w-[92px]" aria-hidden />
        </div>
        <RunView run={resolved.run} flow={resolved.flow} agents={agents} hideSummary inputInTrace />
      </div>
    )
  }

  // Still resolving a flow session — hold the frame so the reified view (a
  // different layout) never flashes before the real graph loads.
  if (resolved === 'loading') {
    return (
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-2">
          <button
            onClick={onBack}
            className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            title="Sohbet transkriptine dön"
          >
            <ArrowLeft size={15} className="shrink-0" />
            <span>Sohbete dön</span>
          </button>
        </div>
        <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
          <Loader2 size={16} className="mr-2 animate-spin" /> Akış yükleniyor…
        </div>
      </div>
    )
  }

  // Fallback: reify the transcript into a linear flow (arbitrary chat session, or
  // a flow run with no linked graph available).
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex items-center justify-between gap-2 border-b border-[var(--color-border)] px-4 py-2">
        <button
          onClick={onBack}
          className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          title="Sohbet transkriptine dön"
        >
          <ArrowLeft size={15} className="shrink-0" />
          <span>Sohbete dön</span>
        </button>
        <span className="truncate text-xs text-[var(--color-text-dim)]">
          Anlık akış görünümü ({realNodes.length} adım)
        </span>
        <span className="w-[92px]" aria-hidden />
      </div>
      {empty ? (
        <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
          Bu oturumda akışa dönüştürülecek mesaj yok.
        </div>
      ) : (
        <RunView run={run} flow={flow} agents={agents} hideSummary inputInTrace />
      )}
    </div>
  )
}
