import type { ReactNode } from 'react'
import { chromeFor, statusRing, START_TINT, END_TINT, type NodeChrome } from './nodeStyles'
import type { NodeStatus } from '../../lib/flowGraph'

interface Props {
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
export function NodeShell({ type, title, isStart, isEnd, selected, status, children }: Props) {
  const chrome: NodeChrome = chromeFor(type)
  const bodyBg = isStart ? START_TINT : isEnd ? END_TINT : 'transparent'
  return (
    <div
      className="min-w-[180px] max-w-[220px] rounded-lg border bg-[var(--color-surface)] text-[var(--color-text)]"
      style={{
        borderColor: selected ? chrome.accent : 'var(--color-border)',
        boxShadow: statusRing(status),
      }}
    >
      <div
        className="flex items-center gap-1.5 rounded-t-lg px-2 py-1 text-[11px] font-semibold"
        style={{ background: chrome.accent, color: '#fff' }}
      >
        <span>{chrome.icon}</span>
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
