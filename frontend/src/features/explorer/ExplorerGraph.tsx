import { useEffect, useLayoutEffect, useMemo, useRef, type KeyboardEvent } from 'react'
import {
  ReactFlow,
  ReactFlowProvider,
  Background,
  Controls,
  MiniMap,
  useReactFlow,
  useStore,
  type Edge,
  type NodeTypes,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import type { ViewHandle, ViewRef } from '@/types'
import { ExplorerNode } from './ExplorerNode'
import {
  nextExplorerNodeId,
  type ExplorerNavigationKey,
  type ExplorerRFNode,
} from './explorerModel'

interface Props {
  nodes: ExplorerRFNode[]
  edges: Edge[]
  onNodeClick: (ref: ViewRef) => void
  onNodeDoubleClick: (ref: ViewRef) => void
  onOverflowClick: (side: 'parents' | 'children', handles: ViewHandle[]) => void
}

const SINGLE_CLICK_DELAY_MS = 250

function FocusedNodeCenter({
  nodes,
  viewportInteraction,
}: {
  nodes: ExplorerRFNode[]
  viewportInteraction: { current: number }
}) {
  const { getInternalNode, getViewport, setCenter } = useReactFlow()
  const focusNode = nodes.find((node) => node.data.focus)
  const focusNodeId = focusNode?.id
  const focusLoading = focusNode?.data.loading
  const focusedNodeVersion = useStore((state) => {
    const node = focusNodeId ? state.nodeLookup.get(focusNodeId) : undefined
    if (!node) return ''
    const { positionAbsolute, userNode } = node.internals
    return `${positionAbsolute.x}:${positionAbsolute.y}:${userNode.position.x}:${userNode.position.y}:${node.measured.width ?? ''}:${node.measured.height ?? ''}`
  })
  const focusSequence = useRef(0)
  const activeFocusId = useRef<string | undefined>(undefined)
  const centeredActiveFocus = useRef(false)
  const initialViewportInteraction = useRef<number | null>(null)

  useLayoutEffect(() => {
    if (activeFocusId.current !== focusNodeId) {
      activeFocusId.current = focusNodeId
      centeredActiveFocus.current = false
      initialViewportInteraction.current = null
    }
    if (!focusNodeId || focusLoading || centeredActiveFocus.current || !focusedNodeVersion) return
    initialViewportInteraction.current ??= viewportInteraction.current
    const sequence = ++focusSequence.current
    let frame = 0
    let framesUntilCenter = 4
    const settle = () => {
      frame = requestAnimationFrame(() => {
        if (focusSequence.current !== sequence || activeFocusId.current !== focusNodeId) return
        framesUntilCenter -= 1
        if (framesUntilCenter > 0) {
          settle()
          return
        }
        if (centeredActiveFocus.current) return
        const focusedNode = getInternalNode(focusNodeId)
        const width = focusedNode?.measured.width
        const height = focusedNode?.measured.height
        if (!focusedNode || !width || !height) return
        if (initialViewportInteraction.current !== viewportInteraction.current) return
        centeredActiveFocus.current = true
        const { positionAbsolute } = focusedNode.internals
        const zoom = getViewport().zoom
        void setCenter(positionAbsolute.x + width / 2, positionAbsolute.y + height / 2, {
          zoom,
        })
      })
    }
    settle()
    return () => {
      focusSequence.current += 1
      cancelAnimationFrame(frame)
    }
  }, [
    focusedNodeVersion,
    focusLoading,
    focusNodeId,
    getInternalNode,
    getViewport,
    setCenter,
    viewportInteraction,
  ])

  return null
}

// Positions come from the pure focus model; canvas owns only pan/zoom and input.
export function ExplorerGraph({
  nodes,
  edges,
  onNodeClick,
  onNodeDoubleClick,
  onOverflowClick,
}: Props) {
  const nodeTypes = useMemo<NodeTypes>(() => ({ explorer: ExplorerNode }), [])
  const pendingNodeClick = useRef<ReturnType<typeof setTimeout> | null>(null)
  const viewportInteraction = useRef(0)

  const cancelPendingNodeClick = () => {
    if (pendingNodeClick.current === null) return
    clearTimeout(pendingNodeClick.current)
    pendingNodeClick.current = null
  }

  useEffect(() => cancelPendingNodeClick, [])

  const activateNode = (node: ExplorerRFNode, focusNode: boolean) => {
    if (node.data.overflow) {
      onOverflowClick(node.data.overflow.side, node.data.overflow.handles)
    } else if (focusNode) {
      onNodeDoubleClick(node.data.ref)
    } else {
      onNodeClick(node.data.ref)
    }
  }

  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    const element = (event.target as HTMLElement).closest<HTMLElement>('.react-flow__node')
    const id = element?.dataset.id
    if (!id) return
    const node = nodes.find((candidate) => candidate.id === id)
    if (!node) return

    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      activateNode(node, event.key === 'Enter' && event.shiftKey)
      return
    }
    if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return
    const nextId = nextExplorerNodeId(nodes, id, event.key as ExplorerNavigationKey)
    if (!nextId) return
    event.preventDefault()
    event.stopPropagation()
    ;[...document.querySelectorAll<HTMLElement>('.react-flow__node')]
      .find((candidate) => candidate.dataset.id === nextId)
      ?.focus()
  }

  return (
    <ReactFlowProvider>
      <FocusedNodeCenter nodes={nodes} viewportInteraction={viewportInteraction} />
      <div
        className="h-full w-full"
        role="region"
        aria-label="Odak ilişkileri grafiği"
        aria-describedby="explorer-keyboard-help"
        onKeyDown={handleKeyDown}
      >
        <ReactFlow
          nodes={nodes}
          edges={edges}
          nodeTypes={nodeTypes}
          onMoveStart={() => {
            viewportInteraction.current += 1
          }}
          onNodeClick={(event, n) => {
            const data = (n as ExplorerRFNode).data
            if (data.overflow) onOverflowClick(data.overflow.side, data.overflow.handles)
            else if (event.detail >= 2) {
              cancelPendingNodeClick()
              onNodeDoubleClick(data.ref)
            } else {
              cancelPendingNodeClick()
              pendingNodeClick.current = setTimeout(() => {
                pendingNodeClick.current = null
                onNodeClick(data.ref)
              }, SINGLE_CLICK_DELAY_MS)
            }
          }}
          nodesDraggable={false}
          nodesConnectable={false}
          elementsSelectable
          nodesFocusable
          edgesFocusable
          fitView
          fitViewOptions={{ padding: 0.3, maxZoom: 1 }}
          minZoom={0.2}
          proOptions={{ hideAttribution: true }}
        >
          <Background gap={18} color="var(--color-border)" />
          <Controls showInteractive={false} />
          <MiniMap pannable zoomable className="!bg-[var(--color-surface-2)] max-md:hidden" />
        </ReactFlow>
      </div>
    </ReactFlowProvider>
  )
}
