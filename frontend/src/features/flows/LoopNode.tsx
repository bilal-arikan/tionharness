import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// LoopNode: one inbound (top) handle, a body source (bottom, id "body") to the
// sub-chain that repeats, and an exit source (right, id "loop") to the node that
// runs after the loop stops. Exit is bounded by maxIters and/or an until match.
export function LoopNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  const cap = node.maxIters && node.maxIters > 0 ? `≤${node.maxIters}×` : '∞'
  const until = node.until ? t('nodes.loopExit', { condition: node.until }) : ''
  return (
    <NodeShell
      id={id}
      type="loop"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {t('nodes.loopRepeat', { cap, exit: until })}
      </div>
      <Handle
        type="source"
        position={Position.Bottom}
        id="body"
        title={t('handles.body')}
        style={{ background: '#db2777' }}
      />
      <Handle
        type="source"
        position={Position.Right}
        id="loop"
        title={t('handles.exit')}
        style={{ background: '#3b82f6' }}
      />
    </NodeShell>
  )
}
