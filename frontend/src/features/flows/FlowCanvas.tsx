import { memo, useCallback, useMemo } from 'react'
import {
  Background,
  BaseEdge,
  Controls,
  EdgeLabelRenderer,
  Handle,
  MarkerType,
  MiniMap,
  Panel,
  Position,
  ReactFlow,
  ReactFlowProvider,
  getSmoothStepPath,
  useReactFlow,
  type Connection,
  type EdgeProps,
  type EdgeTypes,
  type NodeProps,
  type NodeTypes,
  type OnSelectionChangeParams,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import './flowCanvas.css'
import { LayoutGrid, Maximize2, Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { FlowNodeType } from '@/types'
import { ADDABLE_TYPES, NODE_CHROME, statusRing } from './nodeChrome'
import { NODE_W, type FlowNodeData, type FlowRFEdge, type FlowRFNode } from './flowGraph'

// ---- node card ----

function excerpt(s: string | undefined, n = 90): string {
  if (!s) return ''
  const one = s.replace(/\s+/g, ' ').trim()
  return one.length > n ? one.slice(0, n - 1) + '…' : one
}

const FlowNodeCard = memo(function FlowNodeCard({ data, selected }: NodeProps<FlowRFNode>) {
  const { t } = useTranslation('flows')
  const { node, status, arms, agentName, automationName } = data as FlowNodeData
  const chrome = NODE_CHROME[node.type]
  const Icon = chrome.Icon
  const ring = statusRing(status)
  const boxShadow = selected
    ? [ring === 'none' ? '' : ring, `0 0 0 3px ${chrome.accent}`, 'var(--shadow-md)']
        .filter(Boolean)
        .join(', ')
    : ring === 'none'
      ? 'var(--shadow-sm)'
      : ring
  const body = (() => {
    switch (node.type) {
      case 'llm':
        return (
          <>
            <p className="line-clamp-2 text-[11px] text-[var(--color-text-dim)]">
              {excerpt(node.prompt) || '{{input}}'}
            </p>
            <div className="mt-1 flex flex-wrap gap-1 text-[10px]">
              <span className="rounded bg-[var(--color-surface-2)] px-1 py-0.5">
                {t(`context.${node.context ?? 'thread'}`)}
              </span>
              {node.tools === 'none' && (
                <span className="rounded bg-[var(--color-surface-2)] px-1 py-0.5">
                  {t('tools.none')}
                </span>
              )}
              {agentName && (
                <span className="rounded bg-[var(--color-surface-2)] px-1 py-0.5">{agentName}</span>
              )}
              {node.model && (
                <span className="rounded bg-[var(--color-surface-2)] px-1 py-0.5 font-mono">
                  {node.model}
                </span>
              )}
            </div>
          </>
        )
      case 'route':
        return (
          <div className="space-y-0.5 text-[11px]">
            <div className="text-[var(--color-text-dim)]">
              {t(`mode.${node.mode ?? 'contains'}`)} ·{' '}
              {t('route.maxVisits', { count: node.maxVisits || 3 })}
            </div>
            {node.mode === 'criteria' && (
              <div className="text-[10px] text-[var(--color-text-dim)]">
                {t('route.criteriaCount', { count: (node.criteria ?? []).length })}
              </div>
            )}
            {(arms ?? []).map((a, i) => (
              <div key={i} className="flex items-center gap-1 font-mono text-[10px]">
                <span className="rounded bg-[var(--color-surface-2)] px-1">{a.when || '*'}</span>
                <span className="opacity-60">→ {a.to}</span>
              </div>
            ))}
          </div>
        )
      case 'trigger':
        return (
          <div className="space-y-0.5 text-[11px]">
            <div className="truncate text-[var(--color-text-dim)]">
              {automationName ?? (node.automationId ? node.automationId : t('node.triggerUnset'))}
            </div>
            <p className="line-clamp-2 font-mono text-[10px] text-[var(--color-text-dim)]">
              {excerpt(node.template) || '{{last}}'}
            </p>
          </div>
        )
      case 'transform':
      case 'output':
        return (
          <p className="line-clamp-2 font-mono text-[10px] text-[var(--color-text-dim)]">
            {excerpt(node.template) || '{{last}}'}
          </p>
        )
      default:
        return <p className="text-[11px] text-[var(--color-text-dim)]">{t('node.inputHint')}</p>
    }
  })()
  return (
    <div
      className={`rounded-lg border bg-[var(--color-surface)] text-[var(--color-text)] transition-shadow ${status === 'running' ? 'animate-pulse' : ''}`}
      style={{
        width: NODE_W,
        borderColor: selected ? chrome.accent : 'var(--color-border)',
        boxShadow,
      }}
      data-testid={`flow-node-${node.id}`}
    >
      {node.type !== 'input' && (
        <Handle type="target" position={Position.Top} className="!bg-[var(--color-text-dim)]" />
      )}
      <div
        className="flex items-center gap-1.5 rounded-t-lg px-2.5 py-1.5 text-xs font-medium"
        style={{ background: `color-mix(in srgb, ${chrome.accent} 18%, var(--color-surface))` }}
      >
        <Icon size={13} style={{ color: chrome.accent }} />
        <span className="min-w-0 flex-1 truncate">{node.title || node.id}</span>
        <span className="shrink-0 font-mono text-[9px] uppercase tracking-wide opacity-60">
          {t(`types.${node.type}`)}
        </span>
      </div>
      <div className="px-2.5 py-1.5">{body}</div>
      {node.type !== 'output' && (
        <Handle type="source" position={Position.Bottom} style={{ background: chrome.accent }} />
      )}
    </div>
  )
})

// ---- edge with arm label ----

function FlowEdgeLine({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  data,
  selected,
}: EdgeProps<FlowRFEdge>) {
  const [path, labelX, labelY] = getSmoothStepPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
    borderRadius: 12,
  })
  const isRoute = !!data?.isRoute
  const loopBack = targetY < sourceY
  const stroke = selected
    ? 'var(--color-accent)'
    : loopBack
      ? 'var(--color-warning)'
      : 'var(--color-text-dim)'
  return (
    <>
      <BaseEdge
        id={id}
        path={path}
        markerEnd={`url(#flow-arrow-${selected ? 'sel' : loopBack ? 'loop' : 'base'})`}
        style={{
          stroke,
          strokeWidth: selected ? 2.2 : 1.6,
          strokeDasharray: loopBack ? '6 4' : undefined,
        }}
      />
      {isRoute && (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan pointer-events-none absolute rounded border px-1.5 py-0.5 font-mono text-[10px]"
            style={{
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              background: 'var(--color-surface)',
              borderColor: selected ? 'var(--color-accent)' : 'var(--color-border)',
              color: data?.when ? 'var(--color-text)' : 'var(--color-text-dim)',
            }}
          >
            {data?.when || '*'}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

const nodeTypes: NodeTypes = { flow: FlowNodeCard }
const edgeTypes: EdgeTypes = { flow: FlowEdgeLine }

// Arrowheads in the theme's colours (React Flow's built-in marker is fixed grey).
function ArrowDefs() {
  const marker = (id: string, color: string) => (
    <marker
      id={id}
      viewBox="0 0 10 10"
      refX="9"
      refY="5"
      markerWidth="8"
      markerHeight="8"
      orient="auto-start-reverse"
    >
      <path d="M 0 0 L 10 5 L 0 10 z" fill={color} />
    </marker>
  )
  return (
    <svg width="0" height="0" className="absolute">
      <defs>
        {marker('flow-arrow-base', 'var(--color-text-dim)')}
        {marker('flow-arrow-loop', 'var(--color-warning)')}
        {marker('flow-arrow-sel', 'var(--color-accent)')}
      </defs>
    </svg>
  )
}

interface ToolsProps {
  readOnly: boolean
  onAdd: (type: FlowNodeType) => void
  onAutoLayout: () => void
  showMinimap: boolean
}

function CanvasTools({ readOnly, onAdd, onAutoLayout }: ToolsProps) {
  const { t } = useTranslation('flows')
  const { fitView } = useReactFlow()
  return (
    <Panel position="top-left">
      <div className="flex flex-wrap items-center gap-1 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 text-xs shadow-[var(--shadow-md)]">
        {!readOnly &&
          ADDABLE_TYPES.map((type) => {
            const Icon = NODE_CHROME[type].Icon
            return (
              <button
                key={type}
                type="button"
                onClick={() => onAdd(type)}
                data-testid={`flow-add-${type}`}
                title={t('canvas.addNode', { type: t(`types.${type}`) })}
                className="flex items-center gap-1 rounded px-2 py-1 hover:bg-[var(--color-surface-2)]"
              >
                <Plus size={12} />
                <Icon size={13} style={{ color: NODE_CHROME[type].accent }} />
                <span className="max-sm:hidden">{t(`types.${type}`)}</span>
              </button>
            )
          })}
        <span className="mx-0.5 h-4 w-px bg-[var(--color-border)]" />
        <button
          type="button"
          onClick={() => {
            onAutoLayout()
            setTimeout(() => fitView({ padding: 0.2, duration: 300 }), 50)
          }}
          title={t('canvas.autoLayout')}
          className="flex items-center gap-1 rounded px-2 py-1 hover:bg-[var(--color-surface-2)]"
        >
          <LayoutGrid size={13} />
          <span className="max-sm:hidden">{t('canvas.autoLayout')}</span>
        </button>
        <button
          type="button"
          onClick={() => fitView({ padding: 0.2, duration: 300 })}
          title={t('canvas.fit')}
          className="flex items-center gap-1 rounded px-2 py-1 hover:bg-[var(--color-surface-2)]"
        >
          <Maximize2 size={13} />
        </button>
      </div>
    </Panel>
  )
}

export interface FlowCanvasProps {
  nodes: FlowRFNode[]
  edges: FlowRFEdge[]
  readOnly?: boolean
  showMinimap?: boolean
  onNodesChange: (nodes: FlowRFNode[]) => void
  onConnect: (c: Connection) => void
  onEdgesDelete: (ids: string[]) => void
  onNodesDelete: (ids: string[]) => void
  onSelect: (sel: { nodeId: string | null; edgeId: string | null }) => void
  onAdd: (type: FlowNodeType) => void
  onAutoLayout: () => void
}

// FlowCanvas is the editable node graph. Positions are controlled by the parent
// (it owns the draft graph); the canvas reports moves, connections, deletions
// and the selection. Edges are drawn with arm labels and loop-backs dashed.
export function FlowCanvas({
  nodes,
  edges,
  readOnly = false,
  showMinimap = true,
  onNodesChange,
  onConnect,
  onEdgesDelete,
  onNodesDelete,
  onSelect,
  onAdd,
  onAutoLayout,
}: FlowCanvasProps) {
  const { t } = useTranslation('flows')
  const onSelectionChange = useCallback(
    (p: OnSelectionChangeParams) => {
      onSelect({
        nodeId: p.nodes[0]?.id ?? null,
        edgeId: p.nodes.length ? null : (p.edges[0]?.id ?? null),
      })
    },
    [onSelect],
  )
  const defaultEdgeOptions = useMemo(
    () => ({ type: 'flow', markerEnd: { type: MarkerType.ArrowClosed } }),
    [],
  )
  return (
    <ReactFlowProvider>
      <div className="relative h-full w-full" data-testid="flow-canvas">
        <ArrowDefs />
        <ReactFlow<FlowRFNode, FlowRFEdge>
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          defaultEdgeOptions={defaultEdgeOptions}
          onNodesChange={(changes) => {
            // Positions and selection are the only node changes the parent needs;
            // apply them onto the controlled list.
            const next = nodes.map((n) => ({ ...n }))
            let moved = false
            for (const ch of changes) {
              if (ch.type === 'position' && ch.position) {
                const i = next.findIndex((n) => n.id === ch.id)
                if (i >= 0) {
                  next[i] = { ...next[i], position: ch.position }
                  moved = true
                }
              }
              if (ch.type === 'remove') {
                moved = true
              }
            }
            if (moved) onNodesChange(next)
          }}
          onEdgesChange={(changes) => {
            const removed = changes.filter((c) => c.type === 'remove').map((c) => c.id)
            if (removed.length) onEdgesDelete(removed)
          }}
          onNodesDelete={(deleted) => onNodesDelete(deleted.map((n) => n.id))}
          onConnect={onConnect}
          onSelectionChange={onSelectionChange}
          nodesDraggable={!readOnly}
          nodesConnectable={!readOnly}
          elementsSelectable
          deleteKeyCode={readOnly ? null : ['Backspace', 'Delete']}
          fitView
          fitViewOptions={{ padding: 0.2 }}
          minZoom={0.2}
          maxZoom={2}
          proOptions={{ hideAttribution: true }}
        >
          <Background
            gap={18}
            size={1}
            color="color-mix(in srgb, var(--color-text-dim) 35%, transparent)"
          />
          <Controls showInteractive={false} position="bottom-left" />
          {showMinimap && (
            <MiniMap
              pannable
              zoomable
              position="bottom-right"
              nodeColor={(n) =>
                NODE_CHROME[((n.data as FlowNodeData).node?.type ?? 'llm') as FlowNodeType].accent
              }
              maskColor="color-mix(in srgb, var(--color-bg) 70%, transparent)"
            />
          )}
          <CanvasTools
            readOnly={readOnly}
            onAdd={onAdd}
            onAutoLayout={onAutoLayout}
            showMinimap={showMinimap}
          />
          {readOnly && (
            <Panel position="top-right">
              <span className="rounded bg-[var(--color-surface)] px-2 py-1 text-[10px] text-[var(--color-text-dim)] shadow">
                {t('canvas.readOnly')}
              </span>
            </Panel>
          )}
        </ReactFlow>
      </div>
    </ReactFlowProvider>
  )
}
