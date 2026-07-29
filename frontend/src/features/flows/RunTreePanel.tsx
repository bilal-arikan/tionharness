import type { Flow, FlowRun } from '@/types'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { buildRunTreeRows } from './runTree'
import { STATUS_LABEL, statusColor } from './RunView'

interface Props {
  // The tree's members in backend order (root first, parent before children).
  runs: FlowRun[]
  // Flows by id, for names/emoji — a child usually belongs to a DIFFERENT flow
  // than the root, which is the whole point of showing the tree.
  flows: Flow[]
  viewRunId: string
  onSelect: (runId: string) => void
}

// RunTreePanel lists every run in a composed run's tree, indented by depth, and
// lets the viewer switch between them. It exists because a composed flow's real
// work happens in runs the main canvas cannot show: each subflow/spawn child is
// its own run with its own graph.
//
// A single-run tree renders nothing — an unbranching flow should not pay a column
// of chrome to be told it has no children.
export function RunTreePanel({ runs, flows, viewRunId, onSelect }: Props) {
  if (runs.length < 2) return null
  const rows = buildRunTreeRows(runs)
  const flowByID = new Map(flows.map((f) => [f.id, f]))

  return (
    <div className="flex w-48 shrink-0 flex-col overflow-y-auto border-r border-[var(--color-border)] p-2">
      <div className="px-1 pb-1.5 text-[11px] uppercase tracking-wide text-[var(--color-text-dim)]">
        Koşu ağacı
      </div>
      <ul className="space-y-0.5">
        {rows.map(({ run, depth }) => {
          const flow = flowByID.get(run.flowId)
          const emoji = normalizeAvatar(flow?.emoji)
          return (
            <li key={run.id}>
              <button
                type="button"
                onClick={() => onSelect(run.id)}
                // Indent by ancestry, not by list position, so siblings line up.
                style={{ paddingLeft: `${6 + depth * 12}px` }}
                title={`${flow?.name ?? '（silinmiş akış）'} — ${STATUS_LABEL[run.status] ?? run.status}`}
                className={`flex w-full items-center gap-1 rounded-md py-1 pr-1.5 text-left text-xs ${
                  run.id === viewRunId
                    ? 'bg-[var(--color-accent)] text-white'
                    : 'hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span
                  className={
                    run.id === viewRunId ? 'shrink-0 opacity-90' : `shrink-0 ${statusColor(run.status)}`
                  }
                >
                  {(STATUS_LABEL[run.status] ?? '•').charAt(0)}
                </span>
                {emoji && <span className="shrink-0 leading-none">{emoji}</span>}
                <span className="truncate">{flow?.name ?? '（silinmiş akış）'}</span>
              </button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
