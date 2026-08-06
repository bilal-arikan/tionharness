// Projection layer (internal/view): the compact summary of a large entity.
import type { ViewChildrenResult, ViewLens, ViewLevel, ViewRef, ViewResult } from '@/types'
import { req } from './client'

export const viewApi = {
  // getView fetches one projection. A missing entity is a 404 and an unknown kind
  // a 400 — the backend deliberately never degrades to an empty view, because a
  // blank summary reads like a healthy empty entity.
  getView(ref: ViewRef, level: ViewLevel = 'card', lens: ViewLens = 'health'): Promise<ViewResult> {
    const q = new URLSearchParams({ level, lens })
    if (ref.sub) q.set('sub', ref.sub)
    return req<ViewResult>(
      `/api/views/${encodeURIComponent(ref.kind)}/${encodeURIComponent(ref.id)}?${q}`,
    )
  },

  // viewChildren fetches the structural child handles of a node — the Explorer
  // map's lazy-expand edge. An unsupported kind is a 400 (never an empty 200 that
  // would read like a real leaf); a node with no children returns an empty array.
  viewChildren(ref: ViewRef, lens: ViewLens = 'health'): Promise<ViewChildrenResult> {
    const q = new URLSearchParams({ lens })
    if (ref.sub) q.set('sub', ref.sub)
    return req<ViewChildrenResult>(
      `/api/views/${encodeURIComponent(ref.kind)}/${encodeURIComponent(ref.id)}/children?${q}`,
    )
  },
}
