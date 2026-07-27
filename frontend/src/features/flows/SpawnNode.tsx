import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'

// SpawnNode: launches its spawnFlows as async child runs (non-blocking), then
// continues immediately. A downstream join node awaits them. One inbound + one
// outbound handle.
export function SpawnNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  const n = node.spawnFlows?.length ?? 0
  return (
    <NodeShell id={id} type="spawn" title={node.title} isStart={isStart} isEnd={isEnd} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {n > 0 ? `🚀 ${n} akış (async)` : 'akış seçilmedi'}
      </div>
      <Handle type="source" position={Position.Bottom} title="Çıkış → sonraki node" />
    </NodeShell>
  )
}
