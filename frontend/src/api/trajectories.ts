// Trajectory ("Rota") reads — GET /api/trajectories and /api/trajectories/{id}.
import { req } from './client'
import type { Trajectory, TrajectoryIndexEntry } from '@/types/trajectory'

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
}
