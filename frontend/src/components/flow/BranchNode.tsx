import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from '../../lib/flowGraph'
import { NodeShell } from './NodeShell'

// BranchNode: one inbound (top) handle + one outbound (right) handle per arm,
// stacked vertically. Each arm's source handle id is `b<index>` so the adapter
// can re-target the matching branch.
export function BranchNode({ data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const arms = node.branches ?? []
  return (
    <NodeShell type="branch" title={node.title} isStart={isStart} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} />
      <ul className="space-y-1">
        {arms.map((b, i) => (
          <li key={i} className="relative pr-3 text-[11px]">
            <span className="text-[var(--color-text-dim)]">{b.contains || 'varsayılan'}</span>
            <Handle
              type="source"
              position={Position.Right}
              id={`b${i}`}
              style={{ position: 'absolute', right: -8, top: '50%', transform: 'translateY(-50%)' }}
            />
          </li>
        ))}
        {arms.length === 0 && (
          <li className="text-[11px] text-[var(--color-text-dim)]">dal yok</li>
        )}
      </ul>
    </NodeShell>
  )
}
