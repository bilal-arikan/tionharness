// @vitest-environment jsdom

import { act, useState, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ViewHandle, ViewRef } from '@/types'
import { ExplorerGraph } from './ExplorerGraph'
import type { ExplorerRFNode } from './explorerModel'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

vi.mock('@xyflow/react', () => ({
  ReactFlowProvider: ({ children }: { children: ReactNode }) => children,
  ReactFlow: ({
    nodes,
    children,
    onNodeClick,
  }: {
    nodes: ExplorerRFNode[]
    children: ReactNode
    onNodeClick: (event: React.MouseEvent, node: ExplorerRFNode) => void
  }) => {
    const [renderVersion, setRenderVersion] = useState(0)
    return (
      <div>
        {nodes.map((node) => (
          <button
            key={`${node.id}:${renderVersion}`}
            className="react-flow__node"
            data-id={node.id}
            onClick={(event) => {
              onNodeClick(event, node)
              setRenderVersion((version) => version + 1)
            }}
          >
            {node.data.label}
          </button>
        ))}
        {children}
      </div>
    )
  },
  Background: () => null,
  Controls: () => null,
  MiniMap: () => null,
  useNodesInitialized: () => true,
  useReactFlow: () => ({
    getNode: mocks.getNode,
    getViewport: mocks.getViewport,
    setCenter: mocks.setCenter,
  }),
}))

const mocks = vi.hoisted(() => ({
  getNode: vi.fn(),
  getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
  setCenter: vi.fn(),
}))

const refs = {
  parent: { kind: 'agent', id: 'P' } as ViewRef,
  focus: { kind: 'session', id: 'F' } as ViewRef,
  child: { kind: 'session', id: 'C' } as ViewRef,
}

const node = (id: string, ref: ViewRef, depth: number, y: number): ExplorerRFNode => ({
  id,
  type: 'explorer',
  position: { x: depth * 100, y },
  data: {
    ref,
    label: id,
    depth,
    expanded: false,
    loading: false,
    selected: false,
    childCount: null,
    drillable: true,
    dimmed: false,
    focus: depth === 1,
  },
})

const roots: Root[] = []

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.clearAllMocks()
})

describe('ExplorerGraph viewport', () => {
  it('centers an opened node after measurement and preserves zoom', () => {
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0)
      return 1
    })
    mocks.getNode.mockReturnValue({
      position: { x: 340, y: 92 },
      measured: { width: 200, height: 60 },
    })
    mocks.getViewport.mockReturnValue({ x: 10, y: 20, zoom: 1.75 })
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)

    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[node('session:F', refs.focus, 1, 92)]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )

    expect(mocks.setCenter).toHaveBeenCalledWith(440, 122, { zoom: 1.75 })
  })
})

describe('ExplorerGraph pointer interaction', () => {
  it('delays a single click before selecting the node', () => {
    vi.useFakeTimers()
    const select = vi.fn()
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[node('agent:P', refs.parent, 0, 0)]}
          edges={[]}
          onNodeClick={select}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )

    act(() => container.querySelector<HTMLElement>('.react-flow__node')!.click())
    expect(select).not.toHaveBeenCalled()
    act(() => vi.runAllTimers())
    expect(select).toHaveBeenCalledOnce()
    expect(select).toHaveBeenCalledWith(refs.parent)
  })

  it('cancels pending selection and focuses across a click-triggered node rerender', () => {
    vi.useFakeTimers()
    const select = vi.fn()
    const focus = vi.fn()
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[node('agent:P', refs.parent, 0, 0)]}
          edges={[]}
          onNodeClick={select}
          onNodeDoubleClick={focus}
          onOverflowClick={() => {}}
        />,
      ),
    )
    const firstTarget = container.querySelector<HTMLElement>('.react-flow__node')!
    act(() => firstTarget.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 1 })))
    const secondTarget = container.querySelector<HTMLElement>('.react-flow__node')!
    expect(secondTarget).not.toBe(firstTarget)
    act(() => {
      secondTarget.dispatchEvent(new MouseEvent('click', { bubbles: true, detail: 2 }))
      secondTarget.dispatchEvent(new MouseEvent('dblclick', { bubbles: true, detail: 2 }))
    })
    act(() => vi.runAllTimers())

    expect(select).not.toHaveBeenCalled()
    expect(focus).toHaveBeenCalledOnce()
    expect(focus).toHaveBeenCalledWith(refs.parent)
  })

  it('opens an overflow node immediately on pointer click', () => {
    vi.useFakeTimers()
    const handles: ViewHandle[] = [{ label: 'Child', ref: refs.child }]
    const openOverflow = vi.fn()
    const overflow = node('__overflow:children', refs.focus, 2, 0)
    overflow.data.overflow = { side: 'children', handles }
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[overflow]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={openOverflow}
        />,
      ),
    )

    act(() => container.querySelector<HTMLElement>('.react-flow__node')!.click())
    expect(openOverflow).toHaveBeenCalledWith('children', handles)
  })
})

describe('ExplorerGraph keyboard interaction', () => {
  it('selects with Enter/Space, focuses with Shift+Enter and navigates layers with arrows', () => {
    const select = vi.fn()
    const focus = vi.fn()
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[
            node('agent:P', refs.parent, 0, 0),
            node('session:F', refs.focus, 1, 10),
            node('session:C', refs.child, 2, 20),
          ]}
          edges={[]}
          onNodeClick={select}
          onNodeDoubleClick={focus}
          onOverflowClick={() => {}}
        />,
      ),
    )
    const parent = container.querySelector<HTMLElement>('[data-id="agent:P"]')!
    const focused = container.querySelector<HTMLElement>('[data-id="session:F"]')!

    act(() => parent.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })))
    expect(select).toHaveBeenCalledWith(refs.parent)
    act(() => parent.dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true })))
    expect(select).toHaveBeenCalledTimes(2)
    act(() =>
      parent.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', shiftKey: true, bubbles: true }),
      ),
    )
    expect(focus).toHaveBeenCalledWith(refs.parent)

    act(() =>
      parent.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true })),
    )
    expect(document.activeElement).toBe(focused)
  })

  it('opens an overflow node with keyboard activation', () => {
    const handles: ViewHandle[] = [{ label: 'Child', ref: refs.child }]
    const openOverflow = vi.fn()
    const overflow = node('__overflow:children', refs.focus, 2, 0)
    overflow.data.overflow = { side: 'children', handles }
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[overflow]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={openOverflow}
        />,
      ),
    )

    act(() =>
      container
        .querySelector<HTMLElement>('.react-flow__node')!
        .dispatchEvent(new KeyboardEvent('keydown', { key: ' ', bubbles: true })),
    )
    expect(openOverflow).toHaveBeenCalledWith('children', handles)
  })
})
