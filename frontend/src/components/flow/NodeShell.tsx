import { useContext, type ReactNode } from 'react'
import { NodeToolbar, Position } from '@xyflow/react'
import {
  chromeFor,
  statusRing,
  START_TINT,
  END_TINT,
  NodeActionsContext,
  type NodeChrome,
} from './nodeStyles'
import type { NodeStatus } from '../../lib/flowGraph'

interface Props {
  id: string
  type: string
  title?: string
  isStart: boolean
  isEnd?: boolean
  selected?: boolean
  status?: NodeStatus
  children: ReactNode
}

// NodeShell is the common visual frame for every flow node: a type-colored
// header (with start/end badges) + body, plus a run-status ring. The body is
// faintly tinted green for the start node and blue for an end node (no outgoing
// edge) so the flow's endpoints stand out. Handles are rendered by the concrete
// node component as children.
export function NodeShell({ id, type, title, isStart, isEnd, selected, status, children }: Props) {
  const chrome: NodeChrome = chromeFor(type)
  const bodyBg = isStart ? START_TINT : isEnd ? END_TINT : 'transparent'
  const actions = useContext(NodeActionsContext)
  return (
    <div
      className="min-w-[180px] max-w-[220px] rounded-lg border bg-[var(--color-surface)] text-[var(--color-text)]"
      style={{
        borderColor: selected ? chrome.accent : 'var(--color-border)',
        boxShadow: statusRing(status),
      }}
    >
      {actions && (
        <NodeToolbar isVisible={selected} position={Position.Top} offset={8}>
          <div className="flex gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 text-[11px] shadow-lg">
            {!isStart && (
              <button
                onClick={() => actions.onMakeStart(id)}
                className="rounded px-1.5 py-0.5 hover:bg-[var(--color-surface-2)]"
                title="Başlangıç yap"
              >
                ▶ Başlangıç
              </button>
            )}
            <button
              onClick={() => actions.onDuplicate(id)}
              className="rounded px-1.5 py-0.5 hover:bg-[var(--color-surface-2)]"
              title="Çoğalt"
            >
              ⧉ Çoğalt
            </button>
            <button
              onClick={() => actions.onDelete(id)}
              className="rounded px-1.5 py-0.5 text-[var(--color-danger)] hover:bg-[var(--color-surface-2)]"
              title="Sil"
            >
              ✕ Sil
            </button>
          </div>
        </NodeToolbar>
      )}
      <div
        className="flex items-center gap-1.5 rounded-t-lg px-2 py-1 text-[11px] font-semibold"
        style={{ background: chrome.accent, color: '#fff' }}
      >
        <chrome.Icon size={13} className="shrink-0" />
        <span className="truncate">{title || chrome.label}</span>
        <span className="ml-auto flex gap-1">
          {isStart && <span className="rounded bg-black/25 px-1">başlangıç</span>}
          {isEnd && <span className="rounded bg-black/25 px-1">bitiş</span>}
        </span>
      </div>
      <div className="rounded-b-lg px-2.5 py-2" style={{ background: bodyBg }}>
        {children}
      </div>
    </div>
  )
}
