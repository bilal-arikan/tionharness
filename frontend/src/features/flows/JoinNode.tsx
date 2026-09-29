import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// JoinNode: a barrier that waits for a spawn node's async child runs, collects
// their outputs, then continues. One inbound + one outbound handle.
export function JoinNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="join"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {node.spawnRef ? t('nodes.joinOne', { spawn: node.spawnRef }) : t('nodes.joinAll')}
      </div>
      <Handle type="source" position={Position.Bottom} title={t('handles.next')} />
    </NodeShell>
  )
}
