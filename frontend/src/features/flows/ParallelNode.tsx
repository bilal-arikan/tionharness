import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// ParallelNode: one inbound (top) handle, a fan-out source (bottom, id "fan")
// to the concurrent children, and a join source (right, id "join") to the node
// that runs after all children complete.
export function ParallelNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status } = data
  const count = (node.parallel ?? []).length
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="parallel"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {t('nodes.parallelCount', { count })}
      </div>
      <Handle
        type="source"
        position={Position.Bottom}
        id="fan"
        title={t('handles.parallel')}
        style={{ background: '#0ea5e9' }}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="join"
        title={t('handles.join')}
        style={{ background: '#7c3aed' }}
      />
    </NodeShell>
  )
}
