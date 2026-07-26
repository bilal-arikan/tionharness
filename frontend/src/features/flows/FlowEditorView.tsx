import type { Dispatch, RefObject, SetStateAction } from 'react'
import type { Edge, EdgeChange, NodeChange } from '@xyflow/react'
import { Loader2, XCircle, ChevronDown, ChevronRight, PanelLeftClose, PanelLeftOpen, X } from 'lucide-react'
import type { FlowNodeEvent } from '@/api/flows'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { FlowCanvas, FLOW_NODE_DND_MIME, type EdgeStyle } from './FlowCanvas'
import { NodeInspector } from './NodeInspector'
import { FlowVarsButton } from './FlowVarsButton'
import type { FlowRFNode } from './flowGraph'
import { NODE_TYPES, EDGE_STYLES } from './flowsPanelShared'
import type { Agent, FlowNode, FlowNodeType, FlowRun, FlowState } from '@/types'
import { Button, TagEditor, ModalOverlay } from '@/shared/components'

interface Props {
  agents: Agent[]
  nodes: FlowRFNode[]
  edges: Edge[]
  edgeStyle: EdgeStyle
  animated: boolean
  setAnimated: Dispatch<SetStateAction<boolean>>
  accumulate: boolean
  setAccumulate: Dispatch<SetStateAction<boolean>>
  changeEdgeStyle: (s: EdgeStyle) => void
  onNodesChange: (c: NodeChange<FlowRFNode>[]) => void
  onEdgesChange: (c: EdgeChange[]) => void
  setEdges: Dispatch<SetStateAction<Edge[]>>
  setSelectedNodeId: (id: string | null) => void
  openNodeEditor: (id: string) => void
  addNode: (type: FlowNodeType) => void
  addNodeAt: (type: FlowNodeType, pos: { x: number; y: number }) => void
  autoArrange: () => void
  paletteOpen: boolean
  togglePalette: () => void
  paletteVisible: boolean
  togglePaletteVisible: () => void
  tags: string[]
  onTagsChange: (next: string[]) => void
  nodeEditorOpen: boolean
  setNodeEditorOpen: Dispatch<SetStateAction<boolean>>
  selectedNode: FlowNode | null
  start: string
  patchSelected: (patch: Partial<FlowNode>) => void
  makeStart: () => void
  duplicateSelected: () => void
  deleteSelected: () => void
  runInputRef: RefObject<HTMLTextAreaElement | null>
  input: string
  setInput: Dispatch<SetStateAction<string>>
  doRun: () => void
  running: boolean
  run: FlowRun | null
  liveNodes: FlowNodeEvent[]
  trace: FlowState | null
}

// FlowEditorView is the flow editor's main body: the node palette + view
// options column, the drag-and-drop canvas, the node editor popup, and the
// bottom-anchored run panel with live progress / final trace. All state lives
// in FlowsPanel; this renders it.
export function FlowEditorView({
  agents,
  nodes,
  edges,
  edgeStyle,
  animated,
  setAnimated,
  accumulate,
  setAccumulate,
  changeEdgeStyle,
  onNodesChange,
  onEdgesChange,
  setEdges,
  setSelectedNodeId,
  openNodeEditor,
  addNode,
  addNodeAt,
  autoArrange,
  paletteOpen,
  togglePalette,
  paletteVisible,
  togglePaletteVisible,
  tags,
  onTagsChange,
  nodeEditorOpen,
  setNodeEditorOpen,
  selectedNode,
  start,
  patchSelected,
  makeStart,
  duplicateSelected,
  deleteSelected,
  runInputRef,
  input,
  setInput,
  doRun,
  running,
  run,
  liveNodes,
  trace,
}: Props) {
  return (
    <div className="flex min-w-0 flex-1 flex-col">
      {/* Node palette (add nodes + flow-level presentation) + canvas. The flow
          name/id + file actions + save now live in the top PaneHeader above. */}
      <div className="flex min-h-0 flex-1">
        {paletteVisible && (
        <div className="w-40 flex-shrink-0 space-y-2 overflow-y-auto border-r border-[var(--color-border)] p-2 max-md:w-32">
          <button
            type="button"
            onClick={togglePalette}
            className="flex w-full items-center gap-1 px-1 text-xs font-semibold text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            title={paletteOpen ? 'Node ekle bölümünü daralt' : 'Node ekle bölümünü genişlet'}
            aria-expanded={paletteOpen}
          >
            {paletteOpen ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
            Node ekle
          </button>
          {paletteOpen && NODE_TYPES.map((t) => (
            <button
              key={t.value}
              draggable
              onDragStart={(e) => {
                e.dataTransfer.setData(FLOW_NODE_DND_MIME, t.value)
                e.dataTransfer.effectAllowed = 'move'
              }}
              onClick={() => addNode(t.value)}
              className="flex w-full cursor-grab items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-2 text-left text-xs hover:border-[var(--color-accent)] active:cursor-grabbing"
            >
              <t.Icon size={15} className="shrink-0 text-[var(--color-text-dim)]" />
              <span>{t.label}</span>
            </button>
          ))}

          {/* Flow-level presentation moved here from the meta toolbar. */}
          <div className="space-y-2 border-t border-[var(--color-border)] pt-2">
            <div className="px-1 text-xs font-semibold text-[var(--color-text-dim)]">
              Görünüm
            </div>
            <div className="px-0.5">
              <span className="mb-1 block px-0.5 text-[11px] text-[var(--color-text-dim)]">Etiket</span>
              <TagEditor
                tags={tags}
                onChange={onTagsChange}
                placeholder="Etiket…"
                className="py-1"
              />
            </div>
            <label className="block px-0.5">
              <span className="mb-1 block px-0.5 text-[11px] text-[var(--color-text-dim)]">Kablo</span>
              <select
                value={edgeStyle}
                onChange={(e) => changeEdgeStyle(e.target.value as EdgeStyle)}
                className="w-full rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-xs outline-none"
              >
                {EDGE_STYLES.map((s) => (
                  <option key={s.value} value={s.value}>
                    {s.label}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex cursor-pointer items-center gap-1.5 px-0.5 text-xs text-[var(--color-text-dim)]">
              <input
                type="checkbox"
                checked={animated}
                onChange={(e) => setAnimated(e.target.checked)}
              />
              Animasyon
            </label>
            <label
              className="flex cursor-pointer items-center gap-1.5 px-0.5 text-xs text-[var(--color-text-dim)]"
              title="Ardışık ajan node'ları büyüyen tek bir konuşmayı paylaşır → prompt-cache düğümler arası yeniden kullanılır"
            >
              <input
                type="checkbox"
                checked={accumulate}
                onChange={(e) => setAccumulate(e.target.checked)}
              />
              Bağlamı biriktir (cache)
            </label>
          </div>
        </div>
        )}
        <div className="relative min-w-0 flex-1">
          {/* Floating top-left toggle to hide/show the whole left palette,
              overlaid on the canvas (React Flow's own toolbar is top-right,
              Controls bottom-left, so top-left is free). */}
          <button
            type="button"
            onClick={togglePaletteVisible}
            aria-pressed={paletteVisible}
            title={paletteVisible ? 'Sol paneli gizle' : 'Sol paneli göster'}
            className="absolute left-2 top-2 z-10 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1.5 text-[var(--color-text-dim)] shadow-lg transition hover:text-[var(--color-accent)]"
          >
            {paletteVisible ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
          </button>
          <FlowCanvas
            agents={agents}
            nodes={nodes}
            edges={edges}
            edgeStyle={edgeStyle}
            animated={animated}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            setEdges={setEdges}
            onSelect={setSelectedNodeId}
            onNodeClick={openNodeEditor}
            onDropNode={addNodeAt}
            onAutoLayout={autoArrange}
          />
        </div>
      </div>

      {/* Node editor popup — opens on node click (not drag). Holds the node
          fields plus its make-start / duplicate / delete actions. */}
      {nodeEditorOpen && selectedNode && (
        <ModalOverlay onClose={() => setNodeEditorOpen(false)}>
          <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl">
            <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-2.5">
              <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                Node
              </span>
              <button
                onClick={() => setNodeEditorOpen(false)}
                title="Kapat"
                aria-label="Kapat"
                className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
              >
                <X size={16} />
              </button>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-4">
              <NodeInspector
                node={selectedNode}
                agents={agents}
                isStart={start === selectedNode.id}
                allNodes={nodes.map((n) => n.data.node)}
                onPatch={patchSelected}
                onMakeStart={makeStart}
                onDuplicate={duplicateSelected}
                onDelete={deleteSelected}
              />
            </div>
          </div>
        </ModalOverlay>
      )}

      {/* Run panel. MobileNavBar clearance is handled globally by <main>'s
          max-md bottom padding, so no extra padding is needed here (an earlier
          pb-24 created dead space + scroll). Tight top padding so the input row
          hugs the canvas above it. */}
      <div className="max-h-[40%] overflow-y-auto border-t border-[var(--color-border)] px-4 pb-4 pt-2">
        {/* Variable helper (ℹ️) sits to the LEFT of the run input; its content
            shows in a floating balloon (portal), so the row stays a simple
            centered [ℹ️][input][Çalıştır] line. The run panel is bottom-anchored,
            so the input (and the area around it) grows upward as it gets taller. */}
        <div className="flex items-center gap-2">
          <FlowVarsButton
            context="seed"
            nodeRefs={[]}
            onInsert={(t) => setInput((v) => v + t)}
          />
          <textarea
            ref={runInputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Girdi (akışa {{input}} olarak geçer)"
            rows={1}
            className="max-h-40 min-w-0 flex-1 resize-none overflow-y-auto rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
          />
          <Button onClick={doRun} disabled={running} size="lg">
            {running ? 'Çalışıyor…' : '▶ Çalıştır'}
          </Button>
        </div>

        {/* Live node progress while running (before the final run lands). */}
        {!run && liveNodes.length > 0 && (
          <div className="mt-4 border-t border-[var(--color-border)] pt-3">
            <div className="mb-2 text-xs text-[var(--color-text-dim)]">Canlı ilerleme</div>
            <ol className="space-y-2">
              {liveNodes.map((n, i) => (
                <li key={`${n.nodeId}-${i}`} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                  <div className="mb-1 flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                    {n.error !== undefined ? (
                      <XCircle size={12} className="text-[var(--color-danger)]" />
                    ) : n.output === undefined ? (
                      <Loader2 size={12} className="animate-spin text-[var(--color-accent)]" />
                    ) : null}
                    {i + 1}. [{n.type}] {n.title}
                  </div>
                  {n.error !== undefined ? (
                    <span className="text-xs whitespace-pre-wrap text-[var(--color-danger)]">⚠️ {n.error}</span>
                  ) : n.output === undefined ? (
                    <span className="text-xs italic text-[var(--color-text-dim)]">çalışıyor…</span>
                  ) : n.type === 'branch' ? (
                    <div className="whitespace-pre-wrap">{n.output}</div>
                  ) : (
                    <Markdown>{n.output}</Markdown>
                  )}
                </li>
              ))}
            </ol>
          </div>
        )}

        {run && (
          <div className="mt-4 border-t border-[var(--color-border)] pt-3">
            <div className="mb-2 text-xs">
              Durum:{' '}
              <span className={run.status === 'success' ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}>
                {run.status}
              </span>
              {run.error && <span className="ml-2 text-[var(--color-danger)]">· {run.error}</span>}
            </div>
            <ol className="space-y-2">
              {(trace?.trace ?? []).map((t, i) => (
                <li key={i} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                  <div className="mb-1 text-xs text-[var(--color-text-dim)]">
                    {i + 1}. [{t.type}] {t.title}
                  </div>
                  {t.type === 'branch' ? (
                    <div className="whitespace-pre-wrap">{t.output}</div>
                  ) : (
                    <Markdown>{t.output}</Markdown>
                  )}
                </li>
              ))}
            </ol>
          </div>
        )}
      </div>
    </div>
  )
}
