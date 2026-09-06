// Evolution fitness and configuration history (_Docs/83, E1).
import { req } from './client'
import type { EvolutionResult, GoalEvolution, GoalFitness, SnapshotList } from '@/types/evolution'

export const evolutionApi = {
  // since: unix seconds; omit for the default 30-day window.
  goalFitness: (id: string, since?: number): Promise<GoalFitness> =>
    req<GoalFitness>(
      `/api/goals/${encodeURIComponent(id)}/fitness${since !== undefined ? `?since=${since}` : ''}`,
    ),
  listSnapshots: (): Promise<SnapshotList> => req<SnapshotList>('/api/evolution/snapshots'),
  // Manual evolver pass (ignores cooldown; low-confidence under minRuns).
  evolveGoal: (id: string): Promise<EvolutionResult> =>
    req<EvolutionResult>(`/api/goals/${encodeURIComponent(id)}/evolve`, { method: 'POST' }),
  goalEvolution: (id: string): Promise<GoalEvolution> =>
    req<GoalEvolution>(`/api/goals/${encodeURIComponent(id)}/evolution`),
}
