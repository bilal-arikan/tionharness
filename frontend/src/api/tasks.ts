// Tasks (kanban board) and cron schedules. The board is a passive status
// surface: tasks are described, columned and optionally tagged with an agent or
// flow. It never runs anything — flows, schedules and agent sessions do the work.
import type {
  Task,
  TaskPatch,
  Schedule,
  ScheduleSessionMode,
  BoardState,
  Automation,
  BoardAction,
} from '@/types'
import { req } from './client'

export const taskApi = {
  // The active board (archived cards excluded). Pass includeArchived to also get
  // archived cards (the "Arşivlenenler" view).
  listTasks: (includeArchived = false) =>
    req<Task[]>(`/api/tasks${includeArchived ? '?archived=1' : ''}`),
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
  // Archive (soft-hide) or restore a card. Reversible, unlike deleteTask.
  archiveTask: (id: string, archived: boolean) =>
    req<{ id: string; archived: boolean }>(`/api/tasks/${id}/archive`, {
      method: 'POST',
      body: JSON.stringify({ archived }),
    }),

  // Schedules (cron).
  listSchedules: () => req<Schedule[]>('/api/schedules'),
  createSchedule: (data: {
    name?: string
    agentId?: string
    flowId?: string
    cronExpr: string
    prompt?: string
    sessionMode?: ScheduleSessionMode
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
      name?: string
      agentId?: string
      flowId?: string
      cronExpr: string
      prompt?: string
      sessionMode?: ScheduleSessionMode
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
  // Suggests a name without writing it — the caller puts it in the form and the
  // normal save persists it, so Cancel still discards the suggestion.
  generateScheduleTitle: (id: string) =>
    req<{ title: string }>(`/api/schedules/${id}/generate-title`, { method: 'POST' }),

  // Tag-triggered automations (event-driven loops; surfaced in the Schedules UI).
  listAutomations: () => req<Automation[]>('/api/automations'),
  // Live workspace metrics the automation-screen lane headers show: today's token
  // spend (token lane) + cumulative message/tool counts (counter lane).
  getAutomationLiveStats: () =>
    req<{ tokensToday: number; messages: number; tools: number }>('/api/automations/live-stats'),
  createAutomation: (data: {
    name?: string
    triggerKind?: 'tag' | 'board' | 'token' | 'counter'
    triggerTag?: string
    boardOp?: 'any' | 'move' | 'create' | 'update' | 'delete'
    boardFromState?: string
    boardToState?: string
    boardPriority?: number
    boardExclusive?: boolean
    boardAction?: BoardAction
    boardMoveToState?: string
    tokenScope?: 'session' | 'workspace'
    tokenThreshold?: number
    counterMetric?: 'message' | 'tool'
    counterInterval?: number
    sessionMode?: 'spawn' | 'continue'
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
      triggerKind?: 'tag' | 'board' | 'token' | 'counter'
      triggerTag?: string
      boardOp?: 'any' | 'move' | 'create' | 'update' | 'delete'
      boardFromState?: string
      boardToState?: string
      boardPriority?: number
      boardExclusive?: boolean
      boardAction?: BoardAction
      boardMoveToState?: string
      tokenScope?: 'session' | 'workspace'
      tokenThreshold?: number
      counterMetric?: 'message' | 'tool'
      counterInterval?: number
      sessionMode?: 'spawn' | 'continue'
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
  // Suggests a name without writing it (see generateScheduleTitle).
  generateAutomationTitle: (id: string) =>
    req<{ title: string }>(`/api/automations/${id}/generate-title`, { method: 'POST' }),
}
