import type { ReactNode } from 'react'
import { CollapsibleListShell } from './CollapsibleListShell'
import { ResizeHandle } from './SidebarChrome'
import { useResizableSidebar } from '../../hooks/useResizableSidebar'

interface Props {
  // Collapse state (owned by the screen so its header collapse button can call it).
  open: boolean
  onToggle: () => void
  // localStorage key for the persisted, drag-resizable width.
  widthKey: string
  defaultWidth?: number
  minWidth?: number
  // Label shown on the collapsed reopen rail (e.g. "Artifactlar").
  label: string
  testId?: string
  // Hide the collapsed reopen rail (an external PaneHeader toggle reopens instead).
  hideRail?: boolean
  // The list header + body. The outer column chrome (surface bg, right border,
  // width, drawer/collapse, resize handle) is supplied by ListPane.
  children: ReactNode
}

// ListPane is the ONE standard left-list column shared by every two-pane screen
// (Artifacts, Skills, Tools, Market, Flows, Executions, Agents …). It composes:
//   • CollapsibleListShell — collapse to a slim reopen rail; mobile left drawer.
//   • a solid surface <aside> with a right border, matching the chat sessions sidebar.
//   • useResizableSidebar — a persisted drag-resizable width.
//   • ResizeHandle — the drag strip on the right edge.
// Screens only provide their header + list body via children (and wire their own
// header collapse button to onToggle). This removes the per-panel divergence in
// width handling (custom vs fixed vs resizable), background, borders and markup.
export function ListPane({
  open,
  onToggle,
  widthKey,
  defaultWidth = 288,
  minWidth,
  label,
  testId,
  hideRail,
  children,
}: Props) {
  const { width, startDrag } = useResizableSidebar({ storageKey: widthKey, defaultWidth, min: minWidth })
  return (
    <CollapsibleListShell open={open} onToggle={onToggle} label={label} testId={testId} hideRail={hideRail}>
      <aside
        style={{ width }}
        className="relative flex h-full flex-shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)] max-md:w-[85vw] max-md:max-w-sm"
      >
        {children}
        <ResizeHandle onMouseDown={startDrag} />
      </aside>
    </CollapsibleListShell>
  )
}
