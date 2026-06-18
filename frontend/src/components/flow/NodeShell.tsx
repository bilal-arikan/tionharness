import type { ReactNode } from 'react'
import { chromeFor, statusRing, type NodeChrome } from './nodeStyles'
import type { NodeStatus } from '../../lib/flowGraph'

interface Props {
  type: string
  title?: string
  isStart: boolean
  selected?: boolean
  status?: NodeStatus
  children: ReactNode
}

// NodeShell is the common visual frame for every flow node: a type-colored
// header (with start badge) + body, plus a run-status ring. Handles are
// rendered by the concrete node component as children.
export function NodeShell({ type, title, isStart, selected, status, children }: Props) {
  const chrome: NodeChrome = chromeFor(type)
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
        {isStart && <span className="ml-auto rounded bg-black/25 px-1">başlangıç</span>}
      </div>
      <div className="px-2.5 py-2">{children}</div>
    </div>
  )
}
