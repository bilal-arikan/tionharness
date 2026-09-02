// Lane store (_Docs/77 R10): the module-level external store that holds the
// active workspace's LaneState and keeps it fed from the workspace stream.
// Follows the refreshSignals.ts pattern (module store + useSyncExternalStore)
// so any panel can read the same picture without provider plumbing.
//
// Lifecycle: a panel calls connectLanes() on mount (ref-counted; the stream
// opens for the first subscriber and closes after the last one leaves), seeds
// the tables over REST with seedLanes(), and renders from useLanes(). When the
// stream resets its cursor the state turns `stale` and the panel re-seeds.
// Switching workspace: resetLanes() drops everything (the stream itself is
// workspace-scoped by the server cookie/header, so it is reopened as well).
import { useSyncExternalStore } from 'react'
import { subscribeWorkspaceStream } from '@/api/workspaceStream'
import { emptyLanes, type LaneState } from './laneModel'
import { applyLaneEvent, markConnection, markStale } from './laneReducer'

let state: LaneState = emptyLanes()
const listeners = new Set<() => void>()
let disconnect: (() => void) | null = null
let refs = 0

function commit(next: LaneState): void {
  if (next === state) return
  state = next
  listeners.forEach((l) => l())
}

export function getLanes(): LaneState {
  return state
}

export function subscribeLanes(cb: () => void): () => void {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

// useLanes reads the current lane state reactively.
export function useLanes(): LaneState {
  return useSyncExternalStore(subscribeLanes, getLanes, getLanes)
}

// seedLanes applies a REST-derived transform (seedSessions / seedLiveness …).
export function seedLanes(fn: (s: LaneState) => LaneState): void {
  commit(fn(state))
}

// dispatchLaneEvent folds one stream event in (exported for tests and for
// callers that already own a stream subscription).
export function dispatchLaneEvent(ev: Parameters<typeof applyLaneEvent>[1]): void {
  commit(applyLaneEvent(state, ev))
}

// resetLanes drops the picture (workspace switch) but keeps the connection
// flag; the caller reopens the stream through connectLanes(). Revision goes
// back to 0 so "not seeded yet" reads the same as a fresh store.
export function resetLanes(): void {
  commit({ ...emptyLanes(), connected: state.connected })
}

// connectLanes opens the workspace stream for the lanes (ref-counted). Returns
// the release function to call on unmount.
export function connectLanes(): () => void {
  refs++
  if (!disconnect) {
    disconnect = subscribeWorkspaceStream({
      onOpen: () => commit(markConnection(state, true)),
      onClose: () => commit(markConnection(state, false)),
      onReset: () => commit(markStale(state)),
      onEvent: (ev) => commit(applyLaneEvent(state, ev)),
    })
  }
  let released = false
  return () => {
    if (released) return
    released = true
    refs--
    if (refs <= 0 && disconnect) {
      disconnect()
      disconnect = null
      refs = 0
      commit(markConnection(state, false))
    }
  }
}

// __resetLaneStoreForTest clears the module state between tests.
export function __resetLaneStoreForTest(): void {
  if (disconnect) disconnect()
  disconnect = null
  refs = 0
  state = emptyLanes()
  listeners.clear()
}
