import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// DelayNode: waits its configured duration, then continues. One inbound (top)
// and one outbound (bottom) handle, like a plain step.
export function DelayNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  const ms = node.delayMs ?? 0
  const label =
    ms >= 1000
      ? t('nodes.delaySeconds', { value: (ms / 1000).toFixed(ms % 1000 ? 1 : 0) })
      : `${ms} ms`
  return (
    <NodeShell
      id={id}
      type="delay"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-xs">{t('nodes.delayWait', { value: label })}</div>
      <Handle type="source" position={Position.Bottom} title={t('handles.next')} />
    </NodeShell>
  )
}
