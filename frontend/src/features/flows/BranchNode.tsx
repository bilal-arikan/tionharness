import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// BranchNode: one inbound (top) handle + one outbound (right) handle per arm,
// stacked vertically. Each arm's source handle id is `b<index>` so the adapter
// can re-target the matching branch.
export function BranchNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status } = data
  const arms = node.branches ?? []
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="branch"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <ul className="space-y-1">
        {arms.map((b, i) => (
          <li key={i} className="relative pr-3 text-[11px]">
            <span className="text-[var(--color-text-dim)]">
              {b.contains || t('nodes.defaultBranch')}
            </span>
            <Handle
              type="source"
              position={Position.Right}
              id={`b${i}`}
              title={t('handles.branch', { label: b.contains || t('nodes.defaultBranch') })}
              style={{ position: 'absolute', right: -6, top: '50%', transform: 'translateY(-50%)' }}
            />
          </li>
        ))}
        {arms.length === 0 && (
          <li className="text-[11px] text-[var(--color-text-dim)]">{t('nodes.branchEmpty')}</li>
        )}
      </ul>
    </NodeShell>
  )
}
