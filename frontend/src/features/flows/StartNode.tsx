import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'

// StartNode: the required entry marker. No inbound handle (it is the beginning);
// one outbound handle to the first real node.
export function StartNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, status } = data
  return (
    <NodeShell id={id} type="start" title={node.title || 'Başlangıç'} isStart isEnd={false} selected={selected} status={status}>
      <div className="text-[11px] text-[var(--color-text-dim)]">akış buradan başlar</div>
      <Handle type="source" position={Position.Bottom} title="Çıkış → ilk node" />
    </NodeShell>
  )
}
