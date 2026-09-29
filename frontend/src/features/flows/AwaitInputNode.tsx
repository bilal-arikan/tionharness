import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// AwaitInputNode: the run durably suspends here until external input arrives
// (a human in the Koşular tab, a peer agent, or an event), then continues to Next
// with the input as {{last}}. One inbound (top) + one outbound (bottom) handle.
export function AwaitInputNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
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
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {status === 'waiting' ? t('nodes.awaitInputWaiting') : t('nodes.awaitInputIdle')}
      </div>
      <Handle type="source" position={Position.Bottom} title={t('handles.resumeNext')} />
    </NodeShell>
  )
}
