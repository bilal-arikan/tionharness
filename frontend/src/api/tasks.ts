// Tasks (kanban board) and cron schedules. The board is a passive status
// surface: tasks are described, columned and optionally tagged with an agent or
// flow. It never runs anything — flows, schedules and agent sessions do the work.
import type { Task, TaskPatch, Schedule, BoardState, Automation } from '@/types'
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
    priority?: Task['priority']
    tags?: string[]
    artifactIds?: string[]
    progress?: number
    startDate?: string
    dueDate?: string
  }) =>
    req<Task>('/api/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // (Re)generate a task title from its description.
  generateTaskTitle: (id: string) => req<Task>(`/api/tasks/${id}/title`, { method: 'POST' }),
  updateTask: (id: string, patch: TaskPatch) =>
    req<Task>(`/api/tasks/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  deleteTask: (id: string) => req<{ result: string }>(`/api/tasks/${id}`, { method: 'DELETE' }),

  // Schedules (cron).
  listSchedules: () => req<Schedule[]>('/api/schedules'),
  createSchedule: (data: {
    agentId?: string
    flowId?: string
    cronExpr: string
    prompt?: string
    enabled?: boolean
    expiresAt?: number
  }) =>
    req<Schedule>('/api/schedules', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateSchedule: (
    id: string,
    data: {
      agentId?: string
      flowId?: string
      cronExpr: string
      prompt?: string
      expiresAt?: number
    },
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
  runSchedule: (id: string) => req<Schedule>(`/api/schedules/${id}/run`, { method: 'POST' }),
  setScheduleTags: (id: string, tags: string[]) =>
    req<{ id: string; tags: string[] }>(`/api/schedules/${id}/tags`, {
      method: 'PUT',
      body: JSON.stringify({ tags }),
    }),
  deleteSchedule: (id: string) =>
    req<{ result: string }>(`/api/schedules/${id}`, { method: 'DELETE' }),

  // Tag-triggered automations (event-driven loops; surfaced in the Schedules UI).
  listAutomations: () => req<Automation[]>('/api/automations'),
  createAutomation: (data: {
    name?: string
    triggerKind?: 'tag' | 'board' | 'token'
    triggerTag?: string
    boardOp?: 'any' | 'move' | 'create' | 'update' | 'delete'
    boardFromState?: string
    boardToState?: string
    boardPriority?: number
    boardExclusive?: boolean
    tokenScope?: 'session' | 'workspace'
    tokenThreshold?: number
    targetAgentId?: string
    flowId?: string
    promptTemplate: string
    spawnTags?: string[]
    enabled?: boolean
    maxIterations?: number
    cooldownSec?: number
    expiresAt?: number
  }) =>
    req<Automation>('/api/automations', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAutomation: (
    id: string,
    data: {
      name?: string
      triggerKind?: 'tag' | 'board' | 'token'
      triggerTag?: string
      boardOp?: 'any' | 'move' | 'create' | 'update' | 'delete'
      boardFromState?: string
      boardToState?: string
      boardPriority?: number
      boardExclusive?: boolean
      tokenScope?: 'session' | 'workspace'
      tokenThreshold?: number
      targetAgentId?: string
      flowId?: string
      promptTemplate?: string
      spawnTags?: string[]
      enabled?: boolean
      maxIterations?: number
      cooldownSec?: number
      expiresAt?: number
    },
  ) =>
    req<{ id: string; action: string }>(`/api/automations/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  toggleAutomation: (id: string, enabled: boolean) =>
    req<{ id: string; enabled: boolean }>(`/api/automations/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  resetAutomation: (id: string) =>
    req<{ id: string; action: string }>(`/api/automations/${id}/reset`, { method: 'POST' }),
  deleteAutomation: (id: string) =>
    req<{ deleted: string }>(`/api/automations/${id}`, { method: 'DELETE' }),
}
