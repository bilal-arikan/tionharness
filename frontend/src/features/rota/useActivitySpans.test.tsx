// @vitest-environment jsdom
//
// Covers the cache invalidation useActivitySpans owns: an answer is keyed by
// transcript version AND by the sitting threshold it was computed with, so a
// zoom change that moves the gap onto another ladder rung has to re-ask
// (rotaActivityGap.ts) instead of redrawing a stale split.

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LaneSession } from '@/shared/lib/laneModel'

const apiMock = vi.hoisted(() => ({ sessionActivity: vi.fn() }))
vi.mock('@/api', () => ({ api: apiMock }))

import { useActivitySpans } from './useActivitySpans'

// Only the two fields the hook reads; the rest of a lane is irrelevant here.
function lane(id: string, updatedAt: number): LaneSession {
  return { id, updatedAt } as LaneSession
}

function Harness({ sessions, gapSec }: { sessions: LaneSession[]; gapSec: number }) {
  useActivitySpans(sessions, gapSec)
  return null
}

describe('useActivitySpans', () => {
  let container: HTMLDivElement
  let root: Root | null

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
    apiMock.sessionActivity.mockResolvedValue({ gapSec: 600, sessions: {} })
  })

  afterEach(async () => {
    if (root) await act(async () => root?.unmount())
    container.remove()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  // Render, then run out the hook's debounce so the read actually fires.
  async function render(sessions: LaneSession[], gapSec: number) {
    if (!root) root = createRoot(container)
    await act(async () => {
      root!.render(<Harness sessions={sessions} gapSec={gapSec} />)
    })
    await act(async () => {
      await vi.runAllTimersAsync()
    })
  }

  it('asks with the given threshold and re-asks when it changes', async () => {
    const sessions = [lane('SES1', 10)]
    await render(sessions, 900)
    expect(apiMock.sessionActivity).toHaveBeenCalledTimes(1)
    expect(apiMock.sessionActivity).toHaveBeenLastCalledWith(['SES1'], 900)

    // Same transcript, different rung: the cached answer no longer applies.
    await render(sessions, 300)
    expect(apiMock.sessionActivity).toHaveBeenCalledTimes(2)
    expect(apiMock.sessionActivity).toHaveBeenLastCalledWith(['SES1'], 300)
  })

  it('does not re-ask when neither the transcript nor the threshold moved', async () => {
    await render([lane('SES1', 10)], 600)
    expect(apiMock.sessionActivity).toHaveBeenCalledTimes(1)

    // A new array with the same contents must not count as a change.
    await render([lane('SES1', 10)], 600)
    expect(apiMock.sessionActivity).toHaveBeenCalledTimes(1)
  })
})
