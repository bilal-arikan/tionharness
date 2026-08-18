import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'

// AwaitInputNode: the run durably suspends here until external input arrives
// (a human in the Koşular tab, a peer agent, or an event), then continues to Next
// with the input as {{last}}. One inbound (top) + one outbound (bottom) handle.
export function AwaitInputNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="await-input"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title="Giriş" />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {status === 'waiting' ? '⏳ girdi bekleniyor…' : 'dış girdi bekler → {{last}}'}
      </div>
      <Handle type="source" position={Position.Bottom} title="Çıkış → girdi gelince sonraki node" />
    </NodeShell>
  )
}
