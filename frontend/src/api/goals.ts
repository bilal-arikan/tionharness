// Evolution goals (_Docs/83) — direct create/edit, plus the goal-writer
// intake that drafts a goal from the user's own words (kept verbatim).
import { req } from './client'
import type { Goal, GoalCatalog, GoalIntakeResult, GoalStatus } from '@/types/goal'

export const goalApi = {
  listGoals: (status?: GoalStatus): Promise<Goal[]> =>
    req<Goal[]>(status ? `/api/goals?status=${encodeURIComponent(status)}` : '/api/goals'),
  getGoal: (id: string): Promise<Goal> => req<Goal>(`/api/goals/${encodeURIComponent(id)}`),
  goalCatalog: (): Promise<GoalCatalog> => req<GoalCatalog>('/api/goals/catalog'),
  // The user creates a goal directly in the editor (server validates).
  createGoal: (goal: Goal): Promise<Goal> =>
    req<Goal>('/api/goals', { method: 'POST', body: JSON.stringify(goal) }),
  // The goal-writer agent drafts a new goal (or rewrites goalId) from free text.
  intakeGoal: (text: string, goalId?: string): Promise<GoalIntakeResult> =>
    req<GoalIntakeResult>('/api/goals/intake', {
      method: 'POST',
      body: JSON.stringify({ text, goalId: goalId ?? '' }),
    }),
  updateGoal: (goal: Goal): Promise<Goal> =>
    req<Goal>(`/api/goals/${encodeURIComponent(goal.id)}`, {
      method: 'PUT',
      body: JSON.stringify(goal),
    }),
  setGoalStatus: (id: string, status: GoalStatus): Promise<Goal> =>
    req<Goal>(`/api/goals/${encodeURIComponent(id)}/status`, {
      method: 'POST',
      body: JSON.stringify({ status }),
    }),
  deleteGoal: (id: string): Promise<void> =>
    req<void>(`/api/goals/${encodeURIComponent(id)}`, { method: 'DELETE' }),
}
