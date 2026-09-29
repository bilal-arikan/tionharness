import { Handle, Position, type NodeProps } from '@xyflow/react'
import type { FlowRFNode } from './flowGraph'
import { NodeShell } from './NodeShell'
import { useTranslation } from 'react-i18next'

// EndNode: an optional terminal. One inbound handle; no outbound (the run ends
// here). Optionally shapes (template) and/or validates (outputSchema) the output.
export function EndNode({ id, data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, status } = data
  const hasSchema = !!node.outputSchema?.trim()
  return (
    <NodeShell
      id={id}
      type="end"
      title={node.title || t('nodes.endDefault')}
      isStart={false}
      isEnd
      selected={selected}
      status={status}
    >
      <Handle type="target" position={Position.Top} title={t('handles.input')} />
      <div className="text-[11px] text-[var(--color-text-dim)]">
        {hasSchema ? t('nodes.endSchema') : t('nodes.endHere')}
      </div>
    </NodeShell>
  )
}
