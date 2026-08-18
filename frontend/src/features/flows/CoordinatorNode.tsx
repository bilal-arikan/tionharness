import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useAgent, useIsEndNode } from './nodeStyles'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'

// CoordinatorNode: runs its agent as a coordinator that decides at RUNTIME how
// many workers to spawn (unlike a parallel node's design-time fan-out), blocking
// until every worker has finished. One inbound + one outbound handle.
export function CoordinatorNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status, output } = data
  const agent = useAgent(node.agentId)
  const isEnd = useIsEndNode(id)
  // Mirror AgentNode: on a finished node show the coordinator's final reply
  // inline so the canvas carries the RESULT, not only the delegated goal.
  const showOutput = status === 'done' && !!output?.trim()
  return (
    <NodeShell
      id={id}
      type="coordinator"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="flex items-center gap-1.5 text-xs font-medium">
        {agent ? (
          <AgentAvatar agent={agent} size={18} />
        ) : (
          <span className="inline-block h-[18px] w-[18px] shrink-0 rounded-full bg-[var(--color-surface-2)]" />
        )}
        <span className="truncate">{agent?.name ?? '— ajan seçilmedi —'}</span>
      </div>
      <div className="mt-0.5 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
        dinamik worker fan-out
      </div>
      {node.prompt && (
        <div className="mt-1 line-clamp-2 text-[11px] text-[var(--color-text-dim)]">
          {node.prompt}
        </div>
      )}
      {showOutput && (
        <div
          data-testid="flow-node-output"
          title={output}
          className="mt-1.5 line-clamp-3 rounded border-l-2 border-[color:var(--color-success)] bg-[var(--color-surface-2)] px-1.5 py-1 text-[11px] text-[var(--color-text)]"
        >
          {output}
        </div>
      )}
      <Handle type="source" position={Position.Bottom} title="Çıkış → sonraki node" />
    </NodeShell>
  )
}
