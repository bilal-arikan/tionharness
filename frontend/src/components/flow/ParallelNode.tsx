import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from '../../lib/flowGraph'
import { NodeShell } from './NodeShell'

// ParallelNode: one inbound (top) handle, a fan-out source (bottom, id "fan")
// to the concurrent children, and a join source (right, id "join") to the node
// that runs after all children complete.
export function ParallelNode({ data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const count = (node.parallel ?? []).length
  return (
    <NodeShell type="parallel" title={node.title} isStart={isStart} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {count} eşzamanlı dal · join →
      </div>
      <Handle type="source" position={Position.Bottom} id="fan" />
      <Handle
        type="source"
        position={Position.Right}
        id="join"
        style={{ background: '#7c3aed' }}
      />
    </NodeShell>
  )
}
