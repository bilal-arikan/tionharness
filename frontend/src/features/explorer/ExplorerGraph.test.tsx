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

vi.mock('@xyflow/react', async () => {
  const { useSyncExternalStore } = await vi.importActual<typeof import('react')>('react')
  return {
    ReactFlowProvider: ({ children }: { children: ReactNode }) => children,
    ReactFlow: ({
      nodes,
      children,
      onNodeClick,
      onMoveStart,
    }: {
      nodes: ExplorerRFNode[]
      children: ReactNode
      onNodeClick: (event: React.MouseEvent, node: ExplorerRFNode) => void
      onMoveStart: (event: MouseEvent | TouchEvent | null) => void
    }) => {
      const [renderVersion, setRenderVersion] = useState(0)
      mocks.setOnMoveStart(onMoveStart)
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
    useStore: (selector: (state: { nodeLookup: Map<string, unknown> }) => unknown) =>
      useSyncExternalStore(mocks.subscribe, () => selector(mocks.getStoreState())),
    useReactFlow: () => ({
      getInternalNode: (id: string) => mocks.getStoreState().nodeLookup.get(id),
      getViewport: mocks.getViewport,
      setCenter: mocks.setCenter,
    }),
  }
})

const mocks = vi.hoisted(() => {
  let storeState = { nodeLookup: new Map<string, unknown>() }
  let onMoveStart: ((event: MouseEvent | TouchEvent | null) => void) | null = null
  const listeners = new Set<() => void>()
  return {
    getStoreState: () => storeState,
    subscribe: (listener: () => void) => {
      listeners.add(listener)
      return () => listeners.delete(listener)
    },
    setStoreNode: (id: string, node: unknown) => {
      storeState = { nodeLookup: new Map(storeState.nodeLookup).set(id, node) }
      listeners.forEach((listener) => listener())
    },
    resetStore: () => {
      storeState = { nodeLookup: new Map() }
      listeners.clear()
      onMoveStart = null
    },
    setOnMoveStart: (callback: (event: MouseEvent | TouchEvent | null) => void) => {
      onMoveStart = callback
    },
    moveStart: (event: MouseEvent | TouchEvent | null) => onMoveStart!(event),
    getViewport: vi.fn(() => ({ x: 0, y: 0, zoom: 1 })),
    setCenter: vi.fn(),
  }
})

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

const flushCenterFrames = (frames: FrameRequestCallback[], start = 0) => {
  for (let index = 0; index < 4; index += 1) {
    act(() => frames.shift()!(start + index * 16))
  }
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.clearAllMocks()
  mocks.resetStore()
})

describe('ExplorerGraph viewport', () => {
  it('centers an opened node after measurement and preserves zoom', () => {
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0)
      return 1
    })
    const focusNode = { ...node('session:F', refs.focus, 1, 92), position: { x: 340, y: 92 } }
    mocks.setStoreNode(focusNode.id, {
      measured: { width: 200, height: 60 },
      internals: {
        positionAbsolute: { x: 360, y: 110 },
        userNode: focusNode,
      },
    })
    mocks.getViewport.mockReturnValue({ x: 10, y: 20, zoom: 1.75 })
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)

    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[focusNode]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )

    expect(mocks.setCenter).toHaveBeenCalledWith(460, 140, { zoom: 1.75 })
  })

  it('centers only after the changed focus enters the measured node store', () => {
    const frames: FrameRequestCallback[] = []
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    mocks.getViewport.mockReturnValue({ x: -51, y: 94, zoom: 0.645791 })
    const firstFocus = node('workspace:W', refs.parent, 1, 20)
    const nextFocus = {
      ...node('category:sessions', refs.focus, 1, 610),
      position: { x: 360, y: 610 },
    }
    mocks.setStoreNode(firstFocus.id, {
      measured: { width: 200, height: 60 },
      internals: {
        positionAbsolute: { x: 100, y: 20 },
        userNode: firstFocus,
      },
    })
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)

    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[firstFocus]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )
    flushCenterFrames(frames)
    mocks.setCenter.mockClear()

    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[nextFocus]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )
    expect(mocks.setCenter).not.toHaveBeenCalled()

    act(() =>
      mocks.setStoreNode(nextFocus.id, {
        measured: { width: 200, height: 65.2 },
        internals: {
          positionAbsolute: { x: 360, y: 610 },
          userNode: nextFocus,
        },
      }),
    )
    flushCenterFrames(frames, 64)

    expect(mocks.setCenter).toHaveBeenCalledOnce()
    expect(mocks.setCenter).toHaveBeenCalledWith(460, 642.6, { zoom: 0.645791 })
  })

  it('centers once per focus ID across store node and selection updates', () => {
    const frames: FrameRequestCallback[] = []
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    mocks.getViewport.mockReturnValue({ x: 0, y: 0, zoom: 1 })
    const firstFocus = { ...node('session:F', refs.focus, 1, 92), position: { x: 340, y: 92 } }
    mocks.setStoreNode(firstFocus.id, {
      measured: { width: 200, height: 60 },
      internals: {
        positionAbsolute: { x: 340, y: 92 },
        userNode: firstFocus,
      },
    })
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)
    const render = (focusedNode: ExplorerRFNode) =>
      root.render(
        <ExplorerGraph
          nodes={[focusedNode]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      )

    act(() => render(firstFocus))
    flushCenterFrames(frames)
    expect(mocks.setCenter).toHaveBeenCalledOnce()

    act(() =>
      mocks.setStoreNode(firstFocus.id, {
        measured: { width: 220, height: 80 },
        internals: {
          positionAbsolute: { x: 420, y: 160 },
          userNode: firstFocus,
        },
      }),
    )
    act(() => render({ ...firstFocus, data: { ...firstFocus.data, selected: true } }))
    expect(frames).toHaveLength(0)
    expect(mocks.setCenter).toHaveBeenCalledOnce()

    const nextFocus = { ...node('session:C', refs.child, 1, 240), position: { x: 500, y: 240 } }
    mocks.setStoreNode(nextFocus.id, {
      measured: { width: 180, height: 70 },
      internals: {
        positionAbsolute: { x: 500, y: 240 },
        userNode: nextFocus,
      },
    })
    act(() => render(nextFocus))
    flushCenterFrames(frames, 64)

    expect(mocks.setCenter).toHaveBeenCalledTimes(2)
    expect(mocks.setCenter).toHaveBeenLastCalledWith(590, 275, { zoom: 1 })
  })

  it('never centers stale focus requests during rapid A to B and A to B to A changes', () => {
    const frames: FrameRequestCallback[] = []
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    const firstFocus = { ...node('session:A', refs.focus, 1, 20), position: { x: 100, y: 20 } }
    const nextFocus = { ...node('session:B', refs.child, 1, 220), position: { x: 500, y: 220 } }
    for (const focusedNode of [firstFocus, nextFocus]) {
      mocks.setStoreNode(focusedNode.id, {
        measured: { width: 200, height: 60 },
        internals: {
          positionAbsolute: focusedNode.position,
          userNode: focusedNode,
        },
      })
    }
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)
    const render = (focusedNode: ExplorerRFNode) =>
      root.render(
        <ExplorerGraph
          nodes={[focusedNode]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      )

    act(() => render(firstFocus))
    const staleFirstRequest = frames.shift()!
    act(() => render(nextFocus))
    act(() => staleFirstRequest(0))
    flushCenterFrames(frames)

    expect(mocks.setCenter).toHaveBeenCalledOnce()
    expect(mocks.setCenter).toHaveBeenLastCalledWith(600, 250, { zoom: 1 })

    mocks.setCenter.mockClear()
    act(() => render(firstFocus))
    const staleReturnToFirst = frames.shift()!
    act(() => render(nextFocus))
    const staleSecondRequest = frames.shift()!
    act(() => render(firstFocus))
    act(() => {
      staleReturnToFirst(64)
      staleSecondRequest(64)
    })
    flushCenterFrames(frames, 80)

    expect(mocks.setCenter).toHaveBeenCalledOnce()
    expect(mocks.setCenter).toHaveBeenLastCalledWith(200, 50, { zoom: 1 })
  })

  it('does not overwrite a viewport changed while focus layout settles', () => {
    const frames: FrameRequestCallback[] = []
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })
    const focusNode = { ...node('session:F', refs.focus, 1, 92), position: { x: 340, y: 92 } }
    mocks.setStoreNode(focusNode.id, {
      measured: { width: 200, height: 60 },
      internals: {
        positionAbsolute: focusNode.position,
        userNode: focusNode,
      },
    })
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)

    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[focusNode]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )
    act(() => frames.shift()!(0))
    act(() => frames.shift()!(16))
    mocks.getViewport.mockReturnValue({ x: 48, y: -24, zoom: 1.2 })
    // MiniMap viewport changes can arrive without a source pointer event.
    act(() => mocks.moveStart(null))
    act(() =>
      mocks.setStoreNode(focusNode.id, {
        measured: { width: 200, height: 60 },
        internals: {
          positionAbsolute: { x: 360, y: 110 },
          userNode: focusNode,
        },
      }),
    )
    act(() => frames.shift()!(32))
    flushCenterFrames(frames, 48)

    expect(mocks.setCenter).not.toHaveBeenCalled()
  })

  it('cancels pending focus frames on unmount', () => {
    const frames: FrameRequestCallback[] = []
    vi.spyOn(globalThis, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return 37
    })
    const cancelFrame = vi.spyOn(globalThis, 'cancelAnimationFrame')
    const focusNode = node('session:F', refs.focus, 1, 92)
    mocks.setStoreNode(focusNode.id, {
      measured: { width: 200, height: 60 },
      internals: {
        positionAbsolute: focusNode.position,
        userNode: focusNode,
      },
    })
    const container = document.createElement('div')
    const root = createRoot(container)
    roots.push(root)

    act(() =>
      root.render(
        <ExplorerGraph
          nodes={[focusNode]}
          edges={[]}
          onNodeClick={() => {}}
          onNodeDoubleClick={() => {}}
          onOverflowClick={() => {}}
        />,
      ),
    )
    act(() => root.unmount())
    roots.pop()
    act(() => frames.shift()!(0))

    expect(cancelFrame).toHaveBeenCalledWith(37)
    expect(mocks.setCenter).not.toHaveBeenCalled()
    expect(frames).toHaveLength(0)
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
