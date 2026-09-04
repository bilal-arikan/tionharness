// Projection layer (internal/view): the compact summary of a large entity.
import type {
  ViewChildrenResult,
  ViewGraphResult,
  ViewLevel,
  ViewNeighborhoodResult,
  ViewRef,
  ViewResult,
} from '@/types'
import { req } from './client'

export const viewApi = {
  // getView fetches one projection. A missing entity is a 404 and an unknown kind
  // a 400 — the backend deliberately never degrades to an empty view, because a
  // blank summary reads like a healthy empty entity.
  getView(ref: ViewRef, level: ViewLevel = 'card'): Promise<ViewResult> {
    const q = new URLSearchParams({ level })
    if (ref.sub) q.set('sub', ref.sub)
    return req<ViewResult>(
      `/api/views/${encodeURIComponent(ref.kind)}/${encodeURIComponent(ref.id)}?${q}`,
    )
  },

  // viewChildren fetches the structural child handles of a node — the Explorer
  // map's lazy-expand edge. An unsupported kind is a 400 (never an empty 200 that
  // would read like a real leaf); a node with no children returns an empty array.
  viewChildren(ref: ViewRef): Promise<ViewChildrenResult> {
    const q = new URLSearchParams()
    if (ref.sub) q.set('sub', ref.sub)
    return req<ViewChildrenResult>(
      `/api/views/${encodeURIComponent(ref.kind)}/${encodeURIComponent(ref.id)}/children?${q}`,
    )
  },

  // viewGraph fetches the whole structural map in one call — the Explorer
  // network's data source. Uncapped: the physics layout owns the visual budget.
  viewGraph(signal?: AbortSignal): Promise<ViewGraphResult> {
    return req<ViewGraphResult>('/api/views/graph', { signal })
  },

  viewNeighborhood(ref: ViewRef, signal?: AbortSignal): Promise<ViewNeighborhoodResult> {
    const q = new URLSearchParams()
    if (ref.sub) q.set('sub', ref.sub)
    return req<ViewNeighborhoodResult>(
      `/api/views/${encodeURIComponent(ref.kind)}/${encodeURIComponent(ref.id)}/neighborhood?${q}`,
      { signal },
    )
  },
}
