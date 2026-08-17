import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronRight } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Flow, FlowRun } from '@/types'
import { subscribeFlowTree } from '@/shared/lib/flowNodeBus'
import { flowRunRootOf } from '@/shared/lib/flowRunTree'
import { useVisiblePoll } from '@/shared/hooks/useVisiblePoll'
import { RunView } from './RunView'
import { RunTreePanel } from './RunTreePanel'
import {
  applyChildFrame,
  childProgressFromTree,
  mergeChildProgress,
  runTreeBreadcrumb,
  type ChildProgressMap,
} from './runTree'

interface Props {
  // The run the surrounding screen selected. Kept as the entry point of the tree;
  // which run is actually DISPLAYED is this component's own state.
  run: FlowRun
  flows: Flow[]
  agents: Agent[]
  onRerun?: (run: FlowRun) => void
  rerunning?: boolean
  onResumed?: () => void
}

// How often the tree is re-read while any of its runs is still going. Matches the
// runs list's own cadence, so the root row and the tree never disagree by more
// than one tick.
const TREE_POLL_MS = 3000

// RunTreeView wraps RunView with the composed-run dimension: the tree of runs the
// selected one belongs to, a breadcrumb, and the ability to descend into a child.
//
// It exists as a wrapper rather than as changes inside RunView because RunView has
// three call sites (the Koşular tab and two chat-inline ones) and only one of them
// wants this. RunView keeps rendering exactly one run; this decides WHICH.
export function RunTreeView({ run, flows, agents, onRerun, rerunning, onResumed }: Props) {
  const rootRunId = flowRunRootOf(run)
  const [treeRuns, setTreeRuns] = useState<FlowRun[]>([])
  const [viewRunId, setViewRunId] = useState(run.id)
  // Live per-child progress, keyed by (parent run, parent node) — what the canvas
  // rolls up onto a subflow/spawn node while the run it launched executes. Only
  // the LIVE half lives in state; it is layered over the tree-derived seed below,
  // so a run that finished before this view opened still shows its rollups.
  const [liveProgress, setLiveProgress] = useState<ChildProgressMap>({})

  // Selecting a different run in the surrounding list resets the whole view: a
  // tree from the previous selection must not linger, and the newly selected run
  // is by definition the one to display.
  useEffect(() => {
    setViewRunId(run.id)
    setLiveProgress({})
    setTreeRuns([])
  }, [run.id])

  const loadTree = useCallback(() => {
    let alive = true
    api
      .flowRunTree(run.id)
      .then((rs) => alive && setTreeRuns(rs))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [run.id])

  // Fetch once, then keep polling while anything in the tree is still moving.
  // A finished tree stops costing requests; the root's own freshness keeps coming
  // from the `run` prop, which the surrounding screen polls.
  const active =
    run.status === 'running' ||
    run.status === 'waiting' ||
    treeRuns.some((r) => r.status === 'running' || r.status === 'waiting')
  useEffect(loadTree, [loadTree])
  useVisiblePoll(loadTree, TREE_POLL_MS, [loadTree], active)

  // Live frames from every run in the tree — including children that did not
  // exist when this subscription was made, which is why it is keyed by the root.
  // A frame from a run the tree does not know about means a child was just born,
  // so re-read the tree; that is the only way a new row appears before the next
  // poll tick.
  const knownIDs = useMemo(() => new Set(treeRuns.map((r) => r.id)), [treeRuns])
  const pendingReload = useRef(false)
  useEffect(() => {
    return subscribeFlowTree(rootRunId, (frame) => {
      setLiveProgress((prev) => applyChildFrame(prev, frame))
      if (knownIDs.size > 0 && !knownIDs.has(frame.runId) && !pendingReload.current) {
        pendingReload.current = true
        // Coalesce the burst of frames a newborn child emits into one re-read.
        setTimeout(() => {
          pendingReload.current = false
          loadTree()
        }, 250)
      }
    })
  }, [rootRunId, knownIDs, loadTree])

  // Persisted rollups from the tree, refreshed with every tree read, overlaid
  // with whatever the live stream has said since.
  const childProgress = useMemo(
    () => mergeChildProgress(childProgressFromTree(treeRuns), liveProgress),
    [treeRuns, liveProgress],
  )

  // The displayed run. While showing the entry run itself, prefer the prop: the
  // surrounding screen polls it, so it is never staler than our own copy.
  const viewRun = viewRunId === run.id ? run : (treeRuns.find((r) => r.id === viewRunId) ?? run)
  const viewFlow = flows.find((f) => f.id === viewRun.flowId)
  const trail = runTreeBreadcrumb(treeRuns, viewRun.id)
  const atEntry = viewRun.id === run.id

  // Double-clicking a subflow/spawn node descends into the run it launched —
  // available only once that child has emitted a frame, because that frame is
  // what tells us which run the node produced.
  const descend = useCallback(
    (nodeId: string) => {
      const child = childProgress[viewRun.id]?.[nodeId]
      if (child) setViewRunId(child.runId)
    },
    [childProgress, viewRun.id],
  )

  const refresh = useCallback(() => {
    onResumed?.()
    loadTree()
  }, [onResumed, loadTree])

  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      <RunTreePanel runs={treeRuns} flows={flows} viewRunId={viewRun.id} onSelect={setViewRunId} />
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        {/* Breadcrumb: only meaningful once we have descended — at the entry run
            it would just restate the title the header already shows. */}
        {!atEntry && trail.length > 1 && (
          <nav className="flex flex-wrap items-center gap-0.5 border-b border-[var(--color-border)] px-3 py-1.5 text-xs text-[var(--color-text-dim)]">
            {trail.map((r, i) => (
              <span key={r.id} className="flex items-center gap-0.5">
                {i > 0 && <ChevronRight size={12} className="opacity-60" />}
                <button
                  type="button"
                  onClick={() => setViewRunId(r.id)}
                  disabled={r.id === viewRun.id}
                  className={
                    r.id === viewRun.id
                      ? 'text-[var(--color-text)]'
                      : 'transition hover:text-[var(--color-accent)]'
                  }
                >
                  {flows.find((f) => f.id === r.flowId)?.name ?? '（silinmiş akış）'}
                </button>
              </span>
            ))}
          </nav>
        )}
        <RunView
          run={viewRun}
          flow={viewFlow}
          agents={agents}
          // Re-running is offered for the entry run only: a child run's "same
          // input again" would start a detached root run of the child flow, which
          // is not what the button appears to promise inside a tree.
          onRerun={atEntry ? onRerun : undefined}
          rerunning={rerunning}
          hideSummary={atEntry}
          inputInTrace={atEntry}
          onResumed={refresh}
          childProgress={childProgress[viewRun.id]}
          onDescend={descend}
        />
      </div>
    </div>
  )
}
