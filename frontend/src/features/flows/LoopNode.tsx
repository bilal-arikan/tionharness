import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'

// LoopNode: one inbound (top) handle, a body source (bottom, id "body") to the
// sub-chain that repeats, and an exit source (right, id "loop") to the node that
// runs after the loop stops. Exit is bounded by maxIters and/or an until match.
export function LoopNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  const cap = node.maxIters && node.maxIters > 0 ? `≤${node.maxIters}×` : '∞'
  const until = node.until ? ` · çıkış: "${node.until}"` : ''
  return (
    <NodeShell
      id={id}
      type="loop"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {cap} yinele{until}
      </div>
      <Handle
        type="source"
        position={Position.Bottom}
        id="body"
        title="Gövde → her iterasyonda çalışan alt-zincir"
        style={{ background: '#db2777' }}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="loop"
        title="Çıkış → döngü bitince çalışır"
        style={{ background: '#3b82f6' }}
      />
    </NodeShell>
  )
}
