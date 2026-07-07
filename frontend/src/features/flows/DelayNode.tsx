import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'

// DelayNode: waits its configured duration, then continues. One inbound (top)
// and one outbound (bottom) handle, like a plain step.
export function DelayNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  const ms = node.delayMs ?? 0
  const label = ms >= 1000 ? `${(ms / 1000).toFixed(ms % 1000 ? 1 : 0)} sn` : `${ms} ms`
  return (
    <NodeShell id={id} type="delay" title={node.title} isStart={isStart} isEnd={isEnd} selected={selected} status={status}>
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="text-xs">{label} bekle</div>
      <Handle type="source" position={Position.Bottom} title="Çıkış → sonraki node" />
    </NodeShell>
  )
}
