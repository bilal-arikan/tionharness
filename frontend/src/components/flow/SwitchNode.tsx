import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from '../../lib/flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'

// SwitchNode: like BranchNode but routes on exact (case-insensitive) match of
// the last output. One inbound (top) handle + one outbound (right) handle per
// case, id `b<index>` so the adapter can re-target the matching arm.
export function SwitchNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const arms = node.branches ?? []
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell id={id} type="switch" title={node.title} isStart={isStart} isEnd={isEnd} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} title="Giriş" />
      <ul className="space-y-1">
        {arms.map((b, i) => (
          <li key={i} className="relative pr-3 text-[11px]">
            <span className="text-[var(--color-text-dim)]">
              {b.contains ? `= ${b.contains}` : 'varsayılan'}
            </span>
            <Handle
              type="source"
              position={Position.Right}
              id={`b${i}`}
              title={`Eşleşirse → ${b.contains || 'varsayılan'}`}
              style={{ position: 'absolute', right: -6, top: '50%', transform: 'translateY(-50%)' }}
            />
          </li>
        ))}
        {arms.length === 0 && <li className="text-[11px] text-[var(--color-text-dim)]">case yok</li>}
      </ul>
    </NodeShell>
  )
}
