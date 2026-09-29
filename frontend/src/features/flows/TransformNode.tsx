import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useIsEndNode } from './nodeStyles'
import { useTranslation } from 'react-i18next'

// TransformNode: emits a rendered template as its output (no LLM). One inbound
// (top) + one outbound (bottom) handle. The body previews the template.
export function TransformNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, isStart, status } = data
  const isEnd = useIsEndNode(id)
  return (
    <NodeShell
      id={id}
      type="transform"
      title={node.title}
      isStart={isStart}
      isEnd={isEnd}
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="line-clamp-3 whitespace-pre-wrap font-mono text-[11px] text-[var(--color-text-dim)]">
        {node.template || t('nodes.transformEmpty')}
      </div>
      <Handle type="source" position={Position.Bottom} title={t('handles.next')} />
    </NodeShell>
  )
}
