import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'

// ParallelNode: one inbound (top) handle, a fan-out source (bottom, id "fan")
// to the concurrent children, and a join source (right, id "join") to the node
// that runs after all children complete.
export function ParallelNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const count = (node.parallel ?? []).length
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="parallel"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="text-[11px] text-[var(--color-text-dim)]">{count} eşzamanlı dal · join →</div>
      <Handle
        type="source"
        position={Position.Bottom}
        id="fan"
        title="Paralel dallar (eşzamanlı çalışır)"
        style={{ background: '#0ea5e9' }}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="join"
        title="Join → tüm dallar bitince çalışır"
        style={{ background: '#7c3aed' }}
      />
    </NodeShell>
  )
}
