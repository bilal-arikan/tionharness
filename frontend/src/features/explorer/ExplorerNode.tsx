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
  FileText,
  Zap,
  GraduationCap,
  Lightbulb,
  ScrollText,
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
  // TSK66 map extension: artifacts/automations/skills/insights are category
  // members, logs is the inline-tail leaf.
  artifact: FileText,
  automation: Zap,
  skill: GraduationCap,
  insight: Lightbulb,
  logs: ScrollText,
}

// KIND_COLOR tints the icon per kind so the extension layers stand out from the
// structural core. Only the semantic palette is used (theme vars), so the colors
// hold in both light and dark themes; existing kinds keep the accent.
const KIND_COLOR: Partial<Record<ViewKind, string>> = {
  artifact: 'var(--color-success)',
  automation: 'var(--color-warning)',
  skill: 'var(--color-info)',
  insight: 'var(--color-warning)',
  logs: 'var(--color-text-dim)',
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

  const border = data.selected
    ? 'var(--color-warning)'
    : data.focus
      ? 'var(--color-accent)'
      : 'var(--color-border)'
  const ring = data.selected
    ? 'ring-2 ring-[var(--color-warning)]'
    : data.focus
      ? 'ring-2 ring-[var(--color-accent-soft)]'
      : ''
  const surface = data.focus
    ? 'bg-[var(--color-accent-soft)]'
    : data.overflow
      ? 'bg-[var(--color-surface-2)]'
      : 'bg-[var(--color-surface)]'

  return (
    <div
      className={`flex items-center gap-2 rounded-xl border px-3 py-2 text-xs shadow-[var(--shadow-sm)] transition ${surface} ${ring}`}
      // Dimmed nodes fade to background (focus+context) but stay clickable — never
      // hidden, so the map's shape is preserved.
      style={{
        borderColor: border,
        width: data.focus ? 260 : 230,
        minHeight: data.focus ? 72 : 54,
        opacity: data.dimmed ? 0.35 : 1,
      }}
      title={data.label}
      aria-label={`${data.focus ? 'Odak: ' : data.selected ? 'Seçili: ' : ''}${data.label}${data.overflow ? ', kalan ilişkileri listele' : ''}`}
    >
      <Handle
        type="target"
        position={Position.Left}
        style={{ background: 'var(--color-border)' }}
      />

      <Icon
        size={data.focus ? 19 : 15}
        className="shrink-0 text-[var(--color-accent)]"
        strokeWidth={2}
        style={KIND_COLOR[data.ref.kind] ? { color: KIND_COLOR[data.ref.kind] } : undefined}
      />
      <span
        className={`truncate ${data.focus ? 'text-sm font-bold' : data.selected ? 'font-semibold' : ''}`}
      >
        {data.label}
      </span>

      {!dense && (
        <span className="ml-auto flex shrink-0 items-center gap-1 text-[var(--color-text-dim)]">
          {data.overflow ? (
            <span className="font-semibold text-[var(--color-accent)]">Tümünü gör</span>
          ) : data.childCount != null && data.childCount > 0 ? (
            <span className="tabular-nums">{data.childCount}</span>
          ) : null}
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
