import { Handle, Position, useStore, type NodeProps } from '@xyflow/react'
import {
  Boxes,
  FolderTree,
  Users,
  LayoutGrid,
  Wallet,
  Plug,
  MessageSquare,
  GitBranch,
  Clock,
  ChevronRight,
  ChevronDown,
  Loader2,
  type LucideIcon,
} from 'lucide-react'
import type { ViewKind } from '@/types'
import type { ExplorerRFNode } from './explorerModel'

// Icon per node kind so the map reads at a glance which layer a node belongs to.
const KIND_ICON: Record<ViewKind, LucideIcon> = {
  workspace: Boxes,
  category: FolderTree,
  agent: Users,
  board: LayoutGrid,
  budget: Wallet,
  tools: Plug,
  session: MessageSquare,
  flowrun: GitBranch,
  schedule: Clock,
}

// ExplorerNode is one map card: an icon + label, an expand chevron for drillable
// nodes, and a child-count badge once fetched.
//
// Semantic zoom: zoomed far out the card collapses to a single dense line (icon +
// label), matching the "tiny row vs card" contract — the map stays legible when a
// whole workspace is on screen.
export function ExplorerNode({ data }: NodeProps<ExplorerRFNode>) {
  const zoom = useStore((s) => s.transform[2])
  const dense = zoom < 0.65
  const Icon = KIND_ICON[data.ref.kind] ?? Boxes

  const border = data.selected ? 'var(--color-accent)' : 'var(--color-border)'
  const ring = data.selected ? 'ring-2 ring-[var(--color-accent-soft)]' : ''

  return (
    <div
      className={`flex items-center gap-2 rounded-lg border bg-[var(--color-surface)] px-2.5 py-1.5 text-xs shadow-[var(--shadow-sm)] transition ${ring}`}
      // Dimmed nodes fade to background (focus+context) but stay clickable — never
      // hidden, so the map's shape is preserved.
      style={{ borderColor: border, maxWidth: 230, opacity: data.dimmed ? 0.35 : 1 }}
      title={data.label}
    >
      <Handle
        type="target"
        position={Position.Left}
        style={{ background: 'var(--color-border)' }}
      />

      <Icon size={15} className="shrink-0 text-[var(--color-accent)]" strokeWidth={2} />
      <span className={`truncate ${data.selected ? 'font-medium' : ''}`}>{data.label}</span>

      {!dense && (
        <span className="ml-auto flex shrink-0 items-center gap-1 text-[var(--color-text-dim)]">
          {data.childCount != null && data.childCount > 0 && (
            <span className="tabular-nums">{data.childCount}</span>
          )}
          {data.loading ? (
            <Loader2 size={13} className="animate-spin" />
          ) : (
            data.drillable &&
            (data.expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />)
          )}
        </span>
      )}

      <Handle
        type="source"
        position={Position.Right}
        style={{ background: 'var(--color-border)' }}
      />
    </div>
  )
}
