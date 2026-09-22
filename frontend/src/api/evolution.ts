// Evolution fitness and configuration history (_Docs/83, E1).
import { req } from './client'
import type { EvolutionResult, GoalEvolution, GoalFitness, SnapshotList } from '@/types/evolution'

export const evolutionApi = {
  // since: unix seconds; omit for the default 30-day window. agent: compare
  // configuration versions over one agent's sessions only.
  goalFitness: (id: string, since?: number, agent?: string): Promise<GoalFitness> => {
    const q = new URLSearchParams()
    if (since !== undefined) q.set('since', String(since))
    if (agent) q.set('agent', agent)
    const qs = q.toString()
    return req<GoalFitness>(`/api/goals/${encodeURIComponent(id)}/fitness${qs ? `?${qs}` : ''}`)
  },
  listSnapshots: (): Promise<SnapshotList> => req<SnapshotList>('/api/evolution/snapshots'),
  // Manual evolver pass (ignores cooldown; low-confidence under minRuns).
  evolveGoal: (id: string): Promise<EvolutionResult> =>
    req<EvolutionResult>(`/api/goals/${encodeURIComponent(id)}/evolve`, { method: 'POST' }),
  goalEvolution: (id: string): Promise<GoalEvolution> =>
    req<GoalEvolution>(`/api/goals/${encodeURIComponent(id)}/evolution`),
}
