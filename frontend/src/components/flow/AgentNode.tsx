import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from '../../lib/flowGraph'
import { NodeShell } from './NodeShell'
import { useAgent, useIsEndNode } from './nodeStyles'

// AgentNode: one inbound (top) + one outbound (bottom) handle. The body shows
// the assigned agent and a prompt preview.
export function AgentNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const agent = useAgent(node.agentId)
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell type="agent" title={node.title} isStart={isStart} isEnd={isEnd} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} />
      <div className="text-xs font-medium">{agent?.name ?? '— ajan seçilmedi —'}</div>
      {node.prompt && (
        <div className="mt-1 line-clamp-2 text-[11px] text-[var(--color-text-dim)]">
          {node.prompt}
        </div>
      )}
      <Handle type="source" position={Position.Bottom} />
    </NodeShell>
  )
}
