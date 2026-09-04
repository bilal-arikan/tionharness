// @vitest-environment jsdom

import { StrictMode, act, type ReactElement } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Edge, Node, Options } from 'vis-network'
import {
  networkLayoutKey,
  readNetworkLayout,
  readNetworkPositions,
  writeNetworkPositions,
} from './networkLayoutStorage'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const vis = vi.hoisted(() => {
  class MockDataSet {
    private items = new Map<string, Record<string, unknown>>()

    constructor(items: Record<string, unknown>[] = []) {
      this.add(items)
    }

    add(input: Record<string, unknown> | Record<string, unknown>[]) {
      for (const item of Array.isArray(input) ? input : [input]) {
        this.items.set(item.id as string, { ...item })
      }
    }

    update(input: Record<string, unknown> | Record<string, unknown>[]) {
      for (const item of Array.isArray(input) ? input : [input]) {
        const id = item.id as string
        this.items.set(id, { ...this.items.get(id), ...item })
      }
    }

    remove(input: string | string[]) {
      for (const id of Array.isArray(input) ? input : [input]) this.items.delete(id)
    }

    getIds() {
      return [...this.items.keys()]
    }

    get(id: string) {
      return this.items.get(id) ?? null
    }
  }

  type Handler = (payload?: unknown) => void

  class MockNetwork {
    static instances: MockNetwork[] = []
    readonly handlers = new Map<string, Set<Handler>>()
    readonly onceHandlers = new Map<string, Set<Handler>>()
    readonly setOptionsCalls: Options[] = []
    readonly positions = new Map<string, { x: number; y: number }>()
    readonly physics = {
      physicsBody: {
        velocities: {} as Record<string, { x: number; y: number }>,
      },
    }
    startSimulationCalls = 0
    stopSimulationCalls = 0
    fitCalls = 0
    destroyed = false
    readonly data: { nodes: MockDataSet; edges: MockDataSet }
    readonly initialOptions: Options
    readonly initialNodes: Record<string, unknown>[]
    readonly moveNodeCalls: Array<{ id: string; x: number; y: number }> = []
    readonly moveToCalls: Array<{
      scale?: number
      position?: { x: number; y: number }
      animation?: boolean
    }> = []
    scale = 1
    viewPosition = { x: 0, y: 0 }

    constructor(
      _container: HTMLElement,
      data: { nodes: MockDataSet; edges: MockDataSet },
      initialOptions: Options,
    ) {
      this.data = data
      this.initialOptions = initialOptions
      this.initialNodes = data.nodes.getIds().map((id) => ({ ...data.nodes.get(id) }))
      for (const item of this.initialNodes) {
        const id = item.id as string
        this.positions.set(id, {
          x: (item.x as number | undefined) ?? 0,
          y: (item.y as number | undefined) ?? 0,
        })
        this.physics.physicsBody.velocities[id] = { x: 0, y: 0 }
      }
      MockNetwork.instances.push(this)
    }

    on(event: string, handler: Handler) {
      const handlers = this.handlers.get(event) ?? new Set<Handler>()
      handlers.add(handler)
      this.handlers.set(event, handlers)
    }

    once(event: string, handler: Handler) {
      const handlers = this.onceHandlers.get(event) ?? new Set<Handler>()
      handlers.add(handler)
      this.onceHandlers.set(event, handlers)
    }

    off(event: string, handler: Handler) {
      this.handlers.get(event)?.delete(handler)
      this.onceHandlers.get(event)?.delete(handler)
    }

    emit(event: string, payload?: unknown) {
      for (const handler of [...(this.handlers.get(event) ?? [])]) handler(payload)
      const once = [...(this.onceHandlers.get(event) ?? [])]
      this.onceHandlers.delete(event)
      for (const handler of once) handler(payload)
    }

    listenerCount(event: string) {
      return (this.handlers.get(event)?.size ?? 0) + (this.onceHandlers.get(event)?.size ?? 0)
    }

    setPosition(id: string, position: { x: number; y: number }) {
      this.positions.set(id, position)
    }

    moveNode(id: string, x: number, y: number) {
      this.moveNodeCalls.push({ id, x, y })
      this.positions.set(id, { x, y })
    }

    getPositions(ids: string[]) {
      return Object.fromEntries(
        ids.map((id) => {
          const node = this.data.nodes.get(id) as { x?: number; y?: number } | null
          return [id, this.positions.get(id) ?? { x: node?.x ?? 0, y: node?.y ?? 0 }]
        }),
      )
    }

    setOptions(options: Options) {
      this.setOptionsCalls.push(options)
    }

    startSimulation() {
      this.startSimulationCalls += 1
    }

    advancePhysics() {
      for (const id of this.data.nodes.getIds()) {
        const item = this.data.nodes.get(id) as { fixed?: Node['fixed'] } | null
        const fixed = item?.fixed
        const fixedX = fixed === true || (typeof fixed === 'object' && fixed.x === true)
        const fixedY = fixed === true || (typeof fixed === 'object' && fixed.y === true)
        const position = this.getPositions([id])[id]
        const velocity = this.physics.physicsBody.velocities[id] ?? { x: 0, y: 0 }
        this.positions.set(id, {
          x: fixedX ? position.x : position.x + velocity.x,
          y: fixedY ? position.y : position.y + velocity.y,
        })
      }
    }

    stopSimulation() {
      this.stopSimulationCalls += 1
    }

    fit() {
      this.fitCalls += 1
    }

    getScale() {
      return this.scale
    }

    getViewPosition() {
      return this.viewPosition
    }

    moveTo(options: { scale?: number; position?: { x: number; y: number }; animation?: boolean }) {
      this.moveToCalls.push(options)
      if (options.scale !== undefined) this.scale = options.scale
      if (options.position !== undefined) this.viewPosition = options.position
    }
    readonly focusCalls: Array<{ id: string; options?: unknown }> = []
    readonly selectNodesCalls: string[][] = []

    focus(id: string, options?: unknown) {
      this.focusCalls.push({ id, options })
    }

    selectNodes(ids: string[]) {
      this.selectNodesCalls.push(ids)
    }

    getConnectedNodes() {
      return []
    }

    getConnectedEdges() {
      return []
    }

    destroy() {
      this.destroyed = true
      this.handlers.clear()
      this.onceHandlers.clear()
    }
  }

  return { MockDataSet, MockNetwork }
})

vi.mock('vis-data', () => ({ DataSet: vis.MockDataSet }))
vi.mock('vis-network', () => ({ Network: vis.MockNetwork }))

import { VisNetworkGraph } from './VisNetworkGraph'

const roots = new Set<Root>()
const node = (id: string, extra: Partial<Node> = {}): Node => ({ id, label: id, ...extra })

function graph(
  workspaceId: string,
  nodes: Node[],
  canonicalNodeIds = nodes.map((item) => item.id as string),
  extra: { edges?: Edge[]; density?: number; lite?: boolean; canonicalReady?: boolean } = {},
): ReactElement {
  return (
    <VisNetworkGraph
      workspaceId={workspaceId}
      nodes={nodes}
      edges={extra.edges ?? []}
      canonicalNodeIds={canonicalNodeIds}
      canonicalReady={extra.canonicalReady ?? true}
      density={extra.density}
      lite={extra.lite}
    />
  )
}

async function mount(element: ReactElement) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  roots.add(root)
  await act(async () => root.render(element))
  return { root, host }
}

async function render(root: Root, element: ReactElement) {
  await act(async () => root.render(element))
}

async function unmount(root: Root) {
  await act(async () => {
    root.unmount()
    await Promise.resolve()
  })
  roots.delete(root)
}

function latestNetwork() {
  const network = vis.MockNetwork.instances.at(-1)
  if (!network) throw new Error('Expected a vis Network instance')
  return network
}

beforeEach(() => {
  localStorage.clear()
  vis.MockNetwork.instances.length = 0
  document.documentElement.style.setProperty('--color-border', '#222')
  document.documentElement.style.setProperty('--color-text', '#eee')
  document.documentElement.style.setProperty('--color-text-dim', '#aaa')
})

afterEach(async () => {
  for (const root of [...roots]) await unmount(root)
  vi.useRealTimers()
})

describe('VisNetworkGraph layout lifecycle', () => {
  it('waits for the authoritative async graph before restore, settle, or persistence', async () => {
    writeNetworkPositions('workspace', { a: { x: 40, y: 50 } })
    const stored = localStorage.getItem(networkLayoutKey('workspace'))
    const { root } = await mount(
      <StrictMode>{graph('workspace', [], [], { canonicalReady: false })}</StrictMode>,
    )
    let network = latestNetwork()

    window.dispatchEvent(new Event('pagehide'))
    expect(localStorage.getItem(networkLayoutKey('workspace'))).toBe(stored)
    expect(network.startSimulationCalls).toBe(0)

    await render(root, graph('workspace', [node('a')], ['a'], { canonicalReady: true }))
    network = latestNetwork()
    expect(network.moveNodeCalls).toContainEqual({ id: 'a', x: 40, y: 50 })
    expect(network.getPositions(['a']).a).toEqual({ x: 40, y: 50 })
    expect(network.startSimulationCalls).toBe(1)
    expect(localStorage.getItem(networkLayoutKey('workspace'))).toBe(stored)
    await unmount(root)
  })

  it('prunes stale positions only when an authoritative empty graph exits', async () => {
    writeNetworkPositions('workspace', { stale: { x: 1, y: 2 } })
    const { root } = await mount(graph('workspace', [], [], { canonicalReady: false }))

    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('workspace')).toEqual({ stale: { x: 1, y: 2 } })

    await render(root, graph('workspace', [], [], { canonicalReady: true }))
    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('workspace')).toEqual({})
    await unmount(root)
  })

  it('disables improved layout and exactly restores a partial layout in every StrictMode instance', async () => {
    writeNetworkPositions('workspace', { a: { x: 40, y: 50 } })
    const { root } = await mount(
      <StrictMode>
        {graph('workspace', [
          node('a', { x: 1, y: 2 }),
          node('b', { x: 3, y: 4 }),
          node('c', { x: 5, y: 6 }),
        ])}
      </StrictMode>,
    )

    expect(vis.MockNetwork.instances).toHaveLength(2)
    for (const network of vis.MockNetwork.instances) {
      expect(network.initialOptions.physics).toMatchObject({ enabled: false })
      expect(network.initialOptions.layout).toMatchObject({ improvedLayout: false })
      expect(network.initialNodes).toContainEqual(
        expect.objectContaining({ id: 'a', x: 40, y: 50 }),
      )
      expect(network.moveNodeCalls).toEqual([{ id: 'a', x: 40, y: 50 }])
      expect(network.getPositions(['a']).a).toEqual({ x: 40, y: 50 })
    }
    expect(latestNetwork().data.nodes.get('a')).toMatchObject({ x: 40, y: 50 })
    await unmount(root)
  })

  it('uses improved layout only for an unrestored desktop graph', async () => {
    const desktop = await mount(graph('desktop', [node('a')]))
    expect(latestNetwork().initialOptions.layout).toMatchObject({ improvedLayout: true })
    await unmount(desktop.root)

    writeNetworkPositions('lite-restored', { a: { x: 1, y: 2 } })
    const liteRestored = await mount(graph('lite-restored', [node('a')], ['a'], { lite: true }))
    expect(latestNetwork().initialOptions.layout).toMatchObject({ improvedLayout: false })
    await unmount(liteRestored.root)

    const liteFresh = await mount(graph('lite-fresh', [node('a')], ['a'], { lite: true }))
    expect(latestNetwork().initialOptions.layout).toMatchObject({ improvedLayout: false })
    await unmount(liteFresh.root)
  })

  it('does not persist StrictMode replay or dragEnd, then persists and restores real unmount', async () => {
    const { root } = await mount(
      <StrictMode>{graph('workspace', [node('a', { x: 1, y: 2 })])}</StrictMode>,
    )
    const network = latestNetwork()

    expect(localStorage.getItem(networkLayoutKey('workspace'))).toBeNull()
    network.setPosition('a', { x: 12, y: 34 })
    network.emit('dragEnd')
    expect(localStorage.getItem(networkLayoutKey('workspace'))).toBeNull()

    await unmount(root)
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 12, y: 34 } })

    const remounted = await mount(graph('workspace', [node('a')]))
    expect(latestNetwork().data.nodes.get('a')).toMatchObject({ x: 12, y: 34 })
    await unmount(remounted.root)
  })

  it('persists current visible positions synchronously on pagehide', async () => {
    const { root } = await mount(graph('workspace', [node('a')]))
    latestNetwork().setPosition('a', { x: 7, y: 8 })

    window.dispatchEvent(new Event('pagehide'))

    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 7, y: 8 } })
    await unmount(root)
  })

  it('persists and restores zoom with its world-space center', async () => {
    const first = await mount(graph('workspace', [node('a')]))
    const firstNetwork = latestNetwork()
    firstNetwork.scale = 0.42
    firstNetwork.viewPosition = { x: 125, y: -75 }
    firstNetwork.emit('zoom')

    await unmount(first.root)
    expect(readNetworkLayout('workspace').viewport).toEqual({
      scale: 0.42,
      position: { x: 125, y: -75 },
    })

    const second = await mount(graph('workspace', [node('a')]))
    const secondNetwork = latestNetwork()
    expect(secondNetwork.initialOptions.physics).toMatchObject({
      enabled: true,
      stabilization: { fit: false },
    })
    expect(secondNetwork.moveToCalls).toContainEqual({
      scale: 0.42,
      position: { x: 125, y: -75 },
      animation: false,
    })
    secondNetwork.scale = 0.9
    secondNetwork.viewPosition = { x: 0, y: 0 }
    secondNetwork.emit('afterDrawing')
    expect(secondNetwork.getScale()).toBe(0.42)
    expect(secondNetwork.getViewPosition()).toEqual({ x: 125, y: -75 })
    secondNetwork.scale = 0.8
    secondNetwork.viewPosition = { x: -1, y: 1 }
    secondNetwork.emit('resize')
    await act(async () => {})
    expect(secondNetwork.getScale()).toBe(0.42)
    expect(secondNetwork.getViewPosition()).toEqual({ x: 125, y: -75 })
    secondNetwork.scale = 0.7
    secondNetwork.viewPosition = { x: 2, y: -2 }
    secondNetwork.emit('stabilizationIterationsDone')
    expect(secondNetwork.getScale()).toBe(0.42)
    expect(secondNetwork.getViewPosition()).toEqual({ x: 125, y: -75 })
    const stabilizationUpdates = secondNetwork.setOptionsCalls.flatMap((options) => {
      if (!options.physics || typeof options.physics !== 'object') return []
      const stabilization = options.physics.stabilization
      return stabilization && typeof stabilization === 'object' ? [stabilization] : []
    })
    expect(stabilizationUpdates.length).toBeGreaterThan(0)
    for (const stabilization of stabilizationUpdates) {
      expect(stabilization).toMatchObject({ fit: false })
    }
    await unmount(second.root)
  })

  it('resumes an active simulation from its saved positions and velocities', async () => {
    vi.useFakeTimers()
    const first = await mount(graph('workspace', [node('a')]))
    const firstNetwork = latestNetwork()
    firstNetwork.setPosition('a', { x: 10, y: 20 })
    firstNetwork.physics.physicsBody.velocities.a = { x: 3, y: 4 }

    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkLayout('workspace')).toEqual({
      physicsActive: true,
      positions: { a: { x: 10, y: 20, vx: 3, vy: 4 } },
      viewport: { scale: 1, position: { x: 0, y: 0 } },
    })
    await unmount(first.root)

    const resumed = await mount(graph('workspace', [node('a')]))
    const resumedNetwork = latestNetwork()
    expect(resumedNetwork.startSimulationCalls).toBe(1)
    expect(resumedNetwork.physics.physicsBody.velocities.a).toEqual({ x: 3, y: 4 })
    resumedNetwork.advancePhysics()
    expect(resumedNetwork.getPositions(['a']).a).toEqual({ x: 13, y: 24 })

    await act(async () => vi.advanceTimersByTimeAsync(5000))
    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkLayout('workspace')).toEqual({
      physicsActive: true,
      positions: { a: { x: 13, y: 24, vx: 3, vy: 4 } },
      viewport: { scale: 1, position: { x: 0, y: 0 } },
    })
    await unmount(resumed.root)
  })

  it('persists pagehide and unmount snapshots while a later refresh remains ready', async () => {
    const { root } = await mount(graph('workspace', [node('a')], ['a']))
    const network = latestNetwork()

    await render(root, graph('workspace', [node('a')], ['a'], { canonicalReady: true }))
    network.setPosition('a', { x: 7, y: 8 })
    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 7, y: 8 } })

    network.setPosition('a', { x: 9, y: 10 })
    await unmount(root)
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 9, y: 10 } })
  })

  it('prunes a node removed after this tab wrote its first snapshot', async () => {
    const { root } = await mount(graph('workspace', [node('a')], ['a']))
    latestNetwork().setPosition('a', { x: 7, y: 8 })
    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 7, y: 8 } })

    await render(root, graph('workspace', [], [], { canonicalReady: true }))
    await unmount(root)
    expect(readNetworkPositions('workspace')).toEqual({})
  })

  it('keeps temporarily filtered positions and prunes only canonical deletions', async () => {
    writeNetworkPositions('workspace', {
      a: { x: 1, y: 2 },
      b: { x: 3, y: 4 },
      deleted: { x: 5, y: 6 },
    })
    const { root } = await mount(graph('workspace', [node('a'), node('b')], ['a', 'b']))
    const network = latestNetwork()
    network.setPosition('b', { x: 30, y: 40 })

    await render(root, graph('workspace', [node('a')], ['a', 'b']))
    expect(readNetworkPositions('workspace')).toHaveProperty('b', { x: 3, y: 4 })
    await render(root, graph('workspace', [node('a'), node('b')], ['a', 'b']))
    expect(network.data.nodes.get('b')).toMatchObject({ x: 30, y: 40 })

    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('workspace')).toEqual({
      a: { x: 1, y: 2 },
      b: { x: 30, y: 40 },
    })
    await unmount(root)
  })

  it('keeps the last known hidden-node position on pagehide and real unmount', async () => {
    writeNetworkPositions('pagehide', { a: { x: 1, y: 2 }, b: { x: 3, y: 4 } })
    const pagehide = await mount(graph('pagehide', [node('a'), node('b')], ['a', 'b']))
    latestNetwork().setPosition('b', { x: 30, y: 40 })
    await render(pagehide.root, graph('pagehide', [node('a')], ['a', 'b']))

    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('pagehide')).toEqual({
      a: { x: 1, y: 2 },
      b: { x: 30, y: 40 },
    })
    await unmount(pagehide.root)

    writeNetworkPositions('unmount', { a: { x: 5, y: 6 }, b: { x: 7, y: 8 } })
    const exiting = await mount(graph('unmount', [node('a'), node('b')], ['a', 'b']))
    latestNetwork().setPosition('b', { x: 70, y: 80 })
    await render(exiting.root, graph('unmount', [node('a')], ['a', 'b']))
    await unmount(exiting.root)

    expect(readNetworkPositions('unmount')).toEqual({
      a: { x: 5, y: 6 },
      b: { x: 70, y: 80 },
    })
  })

  it('stabilizes only new nodes while restored nodes stay fixed in place', async () => {
    writeNetworkPositions('workspace', { a: { x: 100, y: 200 } })
    const originalFixed = { x: false, y: true }
    const { root } = await mount(graph('workspace', [node('a', { fixed: originalFixed })]))
    const network = latestNetwork()
    network.setPosition('a', { x: 100, y: 200 })
    const startsBefore = network.startSimulationCalls

    await render(
      root,
      graph('workspace', [node('a', { fixed: originalFixed }), node('b', { x: 5, y: 6 })]),
    )

    expect(network.startSimulationCalls).toBe(startsBefore + 1)
    expect(network.data.nodes.get('a')).toMatchObject({ fixed: { x: true, y: true } })
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 100, y: 200 } })

    const stopsBeforeCompletion = network.stopSimulationCalls
    const optionsBeforeCompletion = network.setOptionsCalls.length
    network.emit('stabilizationIterationsDone')
    expect(network.stopSimulationCalls).toBe(stopsBeforeCompletion)
    expect(network.setOptionsCalls).toHaveLength(optionsBeforeCompletion)
    expect(network.data.nodes.get('a')).toMatchObject({ fixed: originalFixed })
    expect(network.getPositions(['a']).a).toEqual({ x: 100, y: 200 })
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 100, y: 200 } })
    await unmount(root)
  })

  it('keeps restored nodes temporarily pinned across an incremental refresh', async () => {
    writeNetworkPositions('workspace', { a: { x: 100, y: 200 } })
    const originalFixed = { x: false, y: true }
    const { root } = await mount(graph('workspace', [node('a', { fixed: originalFixed })]))
    const network = latestNetwork()

    await render(
      root,
      graph('workspace', [node('a', { fixed: originalFixed }), node('b', { x: 5, y: 6 })]),
    )
    await render(
      root,
      graph('workspace', [
        node('a', { fixed: originalFixed, label: 'refreshed' }),
        node('b', { x: 5, y: 6, label: 'refreshed' }),
      ]),
    )

    expect(network.data.nodes.get('a')).toMatchObject({ fixed: { x: true, y: true } })
    network.advancePhysics()
    expect(network.getPositions(['a']).a).toEqual({ x: 100, y: 200 })

    network.emit('stabilizationIterationsDone')
    expect(network.data.nodes.get('a')).toMatchObject({ fixed: originalFixed })
    expect(network.getPositions(['a']).a).toEqual({ x: 100, y: 200 })
    await unmount(root)
  })

  it('starts restored physics and keeps it active across density and theme changes', async () => {
    vi.useFakeTimers()
    writeNetworkPositions('workspace', { a: { x: 1, y: 2 } })
    const { root } = await mount(graph('workspace', [node('a')]))
    const network = latestNetwork()
    const restored = network.getPositions(['a']).a

    await render(root, graph('workspace', [node('a')], ['a'], { density: 1.5, lite: true }))
    document.documentElement.style.setProperty('--color-text', '#fff')
    await act(async () => Promise.resolve())

    expect(network.setOptionsCalls.length).toBeGreaterThan(0)
    for (const options of network.setOptionsCalls) {
      if (options.physics) expect(options.physics).toMatchObject({ enabled: true })
      if (options.layout) expect(options.layout).toMatchObject({ improvedLayout: false })
    }
    expect(network.startSimulationCalls).toBe(1)
    expect(network.initialOptions.physics).toMatchObject({ minVelocity: 0 })
    expect(network.physics.physicsBody.velocities.a).not.toEqual({ x: 0, y: 0 })
    network.advancePhysics()
    expect(network.getPositions(['a']).a).not.toEqual(restored)
    const stopsBefore = network.stopSimulationCalls
    await act(async () => vi.advanceTimersByTimeAsync(5000))
    network.emit('stabilizationIterationsDone')
    expect(network.stopSimulationCalls).toBe(stopsBefore)
    await unmount(root)
  })

  it('stops continuous physics during destroy without leaving timers', async () => {
    vi.useFakeTimers()
    const { root } = await mount(graph('workspace', [node('a')]))
    const network = latestNetwork()
    expect(network.listenerCount('stabilizationIterationsDone')).toBe(1)

    const stopsBefore = network.stopSimulationCalls
    await unmount(root)

    expect(network.listenerCount('stabilizationIterationsDone')).toBe(0)
    expect(network.stopSimulationCalls).toBe(stopsBefore + 1)
    expect(network.destroyed).toBe(true)
    const fitCalls = network.fitCalls
    await vi.runAllTimersAsync()
    expect(network.fitCalls).toBe(fitCalls)
  })

  it('saves the old workspace scope without leaking it into the new scope', async () => {
    const { root } = await mount(graph('workspace-a', [node('a')]))
    latestNetwork().setPosition('a', { x: 9, y: 10 })

    await render(root, graph('workspace-b', [node('b')]))
    await act(async () => Promise.resolve())

    expect(readNetworkPositions('workspace-a')).toEqual({ a: { x: 9, y: 10 } })
    expect(readNetworkPositions('workspace-b')).toEqual({})
    latestNetwork().setPosition('b', { x: 11, y: 12 })
    window.dispatchEvent(new Event('pagehide'))
    expect(readNetworkPositions('workspace-b')).toEqual({ b: { x: 11, y: 12 } })
    await unmount(root)
  })
})

describe('VisNetworkGraph explorer props', () => {
  it('scopes persistence to layoutId instead of the workspace id', async () => {
    writeNetworkPositions('explorer:workspace', { a: { x: 7, y: 8 } })
    writeNetworkPositions('workspace', { a: { x: 1, y: 2 } })
    const { root } = await mount(
      <VisNetworkGraph
        workspaceId="workspace"
        layoutId="explorer:workspace"
        nodes={[node('a')]}
        edges={[]}
        canonicalNodeIds={['a']}
        canonicalReady
      />,
    )
    const network = latestNetwork()
    expect(network.moveNodeCalls).toContainEqual({ id: 'a', x: 7, y: 8 })
    network.setPosition('a', { x: 70, y: 80 })
    await unmount(root)
    expect(readNetworkPositions('explorer:workspace')).toEqual({ a: { x: 70, y: 80 } })
    expect(readNetworkPositions('workspace')).toEqual({ a: { x: 1, y: 2 } })
  })

  it('honours each focus request once, waits for the node and never zooms out', async () => {
    const element = (focusNodeId: string | null, focusTick: number, nodes = [node('a')]) => (
      <VisNetworkGraph
        workspaceId="workspace"
        nodes={nodes}
        edges={[]}
        canonicalNodeIds={nodes.map((n) => n.id as string)}
        canonicalReady
        focusNodeId={focusNodeId}
        focusTick={focusTick}
      />
    )
    const { root } = await mount(element('b', 1))
    const network = latestNetwork()
    network.scale = 1.6
    expect(network.focusCalls).toHaveLength(0)

    // The node arrives later: the pending request fires exactly once.
    await render(root, element('b', 1, [node('a'), node('b')]))
    expect(network.focusCalls).toEqual([
      {
        id: 'b',
        options: { scale: 1.6, animation: { duration: 400, easingFunction: 'easeInOutQuad' } },
      },
    ])
    expect(network.selectNodesCalls).toEqual([['b']])
    // A data refresh does not re-focus.
    await render(root, element('b', 1, [node('a'), node('b'), node('c')]))
    expect(network.focusCalls).toHaveLength(1)
    // A repeat click (same node, new tick) does; below scale 1 it zooms in to 1.
    network.scale = 0.4
    await render(root, element('b', 2, [node('a'), node('b'), node('c')]))
    expect(network.focusCalls).toHaveLength(2)
    expect((network.focusCalls[1].options as { scale: number }).scale).toBe(1)
    await unmount(root)
  })

  it('forwards a double click on a node and ignores one on empty canvas', async () => {
    const onNodeDoubleClick = vi.fn()
    const { root } = await mount(
      <VisNetworkGraph
        workspaceId="workspace"
        nodes={[node('a')]}
        edges={[]}
        canonicalNodeIds={['a']}
        canonicalReady
        onNodeDoubleClick={onNodeDoubleClick}
      />,
    )
    const network = latestNetwork()
    network.emit('doubleClick', { nodes: ['a'] })
    network.emit('doubleClick', { nodes: [] })
    expect(onNodeDoubleClick).toHaveBeenCalledTimes(1)
    expect(onNodeDoubleClick).toHaveBeenCalledWith('a')
    await unmount(root)
  })
})
