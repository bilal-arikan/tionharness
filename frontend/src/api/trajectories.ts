// Trajectory ("Rota") reads — GET /api/trajectories and /api/trajectories/{id}.
import { req } from './client'
import type { RecipeStats, Trajectory, TrajectoryIndexEntry } from '@/types/trajectory'

export interface TrajectoryListParams {
  root?: string
  template?: string
  status?: string
  terminal?: boolean
  limit?: number
}

export const trajectoryApi = {
  listTrajectories: (params?: TrajectoryListParams): Promise<TrajectoryIndexEntry[]> => {
    const p = new URLSearchParams()
    if (params?.root) p.set('root', params.root)
    if (params?.template) p.set('template', params.template)
    if (params?.status) p.set('status', params.status)
    if (params?.terminal !== undefined) p.set('terminal', params.terminal ? 'true' : 'false')
    if (params?.limit) p.set('limit', String(params.limit))
    const qs = p.toString()
    return req<TrajectoryIndexEntry[]>(`/api/trajectories${qs ? `?${qs}` : ''}`)
  },
  getTrajectory: (id: string): Promise<Trajectory> =>
    req<Trajectory>(`/api/trajectories/${encodeURIComponent(id)}`),
  // Rota F3: recompute the deterministic end-of-run summary on demand.
  summarizeTrajectory: (id: string): Promise<Trajectory> =>
    req<Trajectory>(`/api/trajectories/${encodeURIComponent(id)}/summarize`, { method: 'POST' }),
  // Per-recipe-version rollup of the index (optionally one slug).
  recipeStats: (slug?: string): Promise<RecipeStats[]> =>
    req<RecipeStats[]>(
      `/api/trajectories/recipes${slug ? `?slug=${encodeURIComponent(slug)}` : ''}`,
    ),
}
