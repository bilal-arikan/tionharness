import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useAgent, useIsEndNode } from './nodeStyles'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'

// AgentNode: one inbound (top) + one outbound (bottom) handle. The body shows
// the assigned agent and a prompt preview.
export function AgentNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const agent = useAgent(node.agentId)
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell id={id} type="agent" title={node.title} isStart={isStart} isEnd={isEnd} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="flex items-center gap-1.5 text-xs font-medium">
        {agent ? (
          <AgentAvatar agent={agent} size={18} />
        ) : (
          <span className="inline-block h-[18px] w-[18px] shrink-0 rounded-full bg-[var(--color-surface-2)]" />
        )}
        <span className="truncate">{agent?.name ?? '— ajan seçilmedi —'}</span>
      </div>
      {node.prompt && (
        <div className="mt-1 line-clamp-2 text-[11px] text-[var(--color-text-dim)]">
          {node.prompt}
        </div>
      )}
      <Handle type="source" position={Position.Bottom} title="Çıkış → sonraki node" />
    </NodeShell>
  )
}
