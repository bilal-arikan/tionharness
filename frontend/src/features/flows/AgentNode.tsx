import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useAgent, useIsEndNode } from './nodeStyles'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'

// AgentNode: one inbound (top) + one outbound (bottom) handle. The body shows
// the assigned agent and a prompt preview.
export function AgentNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status, output } = data
  const agent = useAgent(node.agentId)
  const isEnd = useIsEndNode(id)
  // On a finished node in a run view, show the reply as an inline preview so the
  // agent's OUTPUT is visible on the canvas (not just its prompt).
  const showOutput = status === 'done' && !!output?.trim()
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
