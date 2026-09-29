import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useTranslation } from 'react-i18next'

// StartNode: the required entry marker. No inbound handle (it is the beginning);
// one outbound handle to the first real node.
export function StartNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, status } = data
  return (
    <NodeShell
      id={id}
      type="start"
      title={node.title || t('nodes.startDefault')}
      isStart
      isEnd={false}
      selected={selected}
      status={status}
    >
      <div className="text-[11px] text-[var(--color-text-dim)]">{t('nodes.startHere')}</div>
      <Handle type="source" position={Position.Bottom} title={t('handles.startNext')} />
    </NodeShell>
  )
}
