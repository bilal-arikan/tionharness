import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { ChildRunBadge } from './ChildRunBadge'
import { useTranslation } from 'react-i18next'

// SubflowNode: runs another flow (flowRef) to completion with a rendered input,
// captures its output as {{last}}, then continues. One inbound + one outbound handle.
export function SubflowNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status, child } = data
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="subflow"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {node.flowRef ? t('nodes.subflowRef', { id: node.flowRef }) : t('nodes.subflowNone')}
      </div>
      <ChildRunBadge child={child} />
      <Handle type="source" position={Position.Bottom} title={t('handles.next')} />
    </NodeShell>
  )
}
