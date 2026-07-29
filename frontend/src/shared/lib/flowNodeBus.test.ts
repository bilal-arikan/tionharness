import { describe, it, expect, vi } from 'vitest'

import { publishFlowNode, subscribeFlowNode, subscribeFlowTree } from './flowNodeBus'
import type { FlowNodeEvent } from '@/types'

const frame = (nodeId: string): FlowNodeEvent =>
  ({ phase: 'done', nodeId, type: 'agent', title: '', index: 1 }) as FlowNodeEvent

describe('flowNodeBus', () => {
  it('delivers a frame to the subscriber of the emitting run only', () => {
    const mine = vi.fn()
    const other = vi.fn()
    const offMine = subscribeFlowNode('RUN-a', mine)
    const offOther = subscribeFlowNode('RUN-b', other)

    publishFlowNode('RUN-a', frame('n1'), 'RUN-a')

    expect(mine).toHaveBeenCalledOnce()
    expect(other).not.toHaveBeenCalled()
    offMine()
    offOther()
  })

  it('routes a child run frame to a tree subscriber watching the root', () => {
    // The parent viewer subscribes to the ROOT, never to the child's id — the
    // child run does not exist yet when the viewer mounts.
    const tree = vi.fn()
    const off = subscribeFlowTree('RUN-root', tree)

    publishFlowNode('RUN-child', frame('n2'), 'RUN-root')

    expect(tree).toHaveBeenCalledOnce()
    // The frame must say which run it came from, or the parent would paint the
    // child's node ids onto its own graph.
    expect(tree.mock.calls[0][0]).toEqual({ runId: 'RUN-child', ev: frame('n2') })
    off()
  })

  it('carries the lineage tags through to the tree scope only', () => {
    // The parent's canvas rolls a child's progress up onto the node that launched
    // it, so both the launching run and the launching node have to survive the
    // hop. The per-run scope is already keyed by the run and gets the bare event.
    const tree = vi.fn()
    const perRun = vi.fn()
    const offTree = subscribeFlowTree('RUN-root', tree)
    const offRun = subscribeFlowNode('RUN-child', perRun)

    publishFlowNode('RUN-child', frame('n2'), 'RUN-root', {
      parentRunId: 'RUN-root',
      parentNodeId: 'sub1',
    })

    expect(tree.mock.calls[0][0]).toEqual({
      runId: 'RUN-child',
      parentRunId: 'RUN-root',
      parentNodeId: 'sub1',
      ev: frame('n2'),
    })
    expect(perRun.mock.calls[0][0]).toEqual(frame('n2'))
    offTree()
    offRun()
  })

  it('does not leak frames across sibling trees', () => {
    const tree = vi.fn()
    const off = subscribeFlowTree('RUN-root', tree)

    publishFlowNode('RUN-elsewhere', frame('n3'), 'RUN-otherroot')

    expect(tree).not.toHaveBeenCalled()
    off()
  })

  it('treats a missing root tag as the run being its own root', () => {
    // Frames from a backend that predates tree tagging must still reach both
    // scopes rather than vanishing.
    const perRun = vi.fn()
    const tree = vi.fn()
    const offRun = subscribeFlowNode('RUN-solo', perRun)
    const offTree = subscribeFlowTree('RUN-solo', tree)

    publishFlowNode('RUN-solo', frame('n4'))

    expect(perRun).toHaveBeenCalledOnce()
    expect(tree).toHaveBeenCalledOnce()
    offRun()
    offTree()
  })

  it('delivers a root run frame to both scopes exactly once each', () => {
    const perRun = vi.fn()
    const tree = vi.fn()
    const offRun = subscribeFlowNode('RUN-root', perRun)
    const offTree = subscribeFlowTree('RUN-root', tree)

    publishFlowNode('RUN-root', frame('n5'), 'RUN-root')

    expect(perRun).toHaveBeenCalledOnce()
    expect(tree).toHaveBeenCalledOnce()
    offRun()
    offTree()
  })

  it('stops delivering after unsubscribe', () => {
    const tree = vi.fn()
    const off = subscribeFlowTree('RUN-root', tree)
    off()

    publishFlowNode('RUN-child', frame('n6'), 'RUN-root')

    expect(tree).not.toHaveBeenCalled()
  })
})
