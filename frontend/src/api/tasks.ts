// Tasks (kanban board) and cron schedules. The board is a passive status
// surface: tasks are described, columned and optionally tagged with an agent or
// flow. It never runs anything — flows, schedules and agent sessions do the work.
import type { Task, Schedule, BoardState } from '../types'
import { req } from './client'

export const taskApi = {
  listTasks: () => req<Task[]>('/api/tasks'),
  createTask: (data: {
    title?: string
    description?: string
    ownerAgentId?: string
    flowId?: string
    boardState?: BoardState
    dependencies?: string
  }) =>
    req<Task>('/api/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // (Re)generate a task title from its description.
  generateTaskTitle: (id: string) =>
    req<Task>(`/api/tasks/${id}/title`, { method: 'POST' }),
  updateTask: (id: string, patch: Partial<Pick<Task, 'title' | 'description' | 'ownerAgentId' | 'flowId' | 'boardState' | 'dependencies'>>) =>
    req<Task>(`/api/tasks/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  deleteTask: (id: string) =>
    req<{ result: string }>(`/api/tasks/${id}`, { method: 'DELETE' }),

  // Schedules (cron).
  listSchedules: () => req<Schedule[]>('/api/schedules'),
  createSchedule: (data: {
    agentId: string
    cronExpr: string
    prompt?: string
    enabled?: boolean
  }) =>
    req<Schedule>('/api/schedules', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateSchedule: (
    id: string,
    data: { agentId: string; cronExpr: string; prompt?: string },
  ) =>
    req<Schedule>(`/api/schedules/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  toggleSchedule: (id: string, enabled: boolean) =>
    req<{ id: string; enabled: boolean }>(`/api/schedules/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  // Fire a schedule immediately ("Run" button), regardless of enabled state.
  runSchedule: (id: string) =>
    req<Schedule>(`/api/schedules/${id}/run`, { method: 'POST' }),
  deleteSchedule: (id: string) =>
    req<{ result: string }>(`/api/schedules/${id}`, { method: 'DELETE' }),
}
