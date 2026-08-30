// @vitest-environment jsdom

import { act, type ReactNode } from 'react'
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
  ReactFlow: ({ nodes, children }: { nodes: ExplorerRFNode[]; children: ReactNode }) => (
    <div>
      {nodes.map((node) => (
        <button key={node.id} className="react-flow__node" data-id={node.id}>
          {node.data.label}
        </button>
      ))}
      {children}
    </div>
  ),
  Background: () => null,
  Controls: () => null,
  MiniMap: () => null,
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
