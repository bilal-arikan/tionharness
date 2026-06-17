// Tasks (kanban board), their runs and cron schedules.
import type { Task, Run, Schedule, BoardState, TurnStep } from '../types'
import { req, wsHeaders, errorFromResponse } from './client'

// Handlers invoked as a streamed task run dispatches parsed SSE events. Mirrors
// the chat stream: live activity steps, then the finished run.
export interface TaskStreamHandlers {
  onMeta?: (m: { runId: string; taskId: string; sessionId: string }) => void
  onStep: (st: TurnStep) => void
  onReply: (r: { run: Run }) => void
  onError: (err: string) => void
}

// streamRunTask POSTs to the SSE run-stream endpoint and dispatches parsed events
// (fetch streaming, since EventSource can't POST). Resolves when the stream ends.
async function streamRunTask(
  taskId: string,
  handlers: TaskStreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`/api/tasks/${taskId}/run-stream`, {
    method: 'POST',
    headers: wsHeaders(),
    signal,
  })
  if (!res.ok || !res.body) {
    handlers.onError(await errorFromResponse(res))
    return
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buf = ''

  const dispatch = (frame: string) => {
    let event = 'message'
    const dataLines: string[] = []
    for (const line of frame.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim()
      else if (line.startsWith('data:')) dataLines.push(line.slice(5).trim())
    }
    if (dataLines.length === 0) return
    let data: unknown
    try {
      data = JSON.parse(dataLines.join('\n'))
    } catch {
      return
    }
    switch (event) {
      case 'meta':
        handlers.onMeta?.(data as { runId: string; taskId: string; sessionId: string })
        break
      case 'step':
        handlers.onStep(data as TurnStep)
        break
      case 'reply':
        handlers.onReply(data as { run: Run })
        break
      case 'error':
        handlers.onError((data as { error: string }).error)
        break
    }
  }

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buf += decoder.decode(value, { stream: true })
    let idx: number
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      if (frame.trim()) dispatch(frame)
    }
  }
}

export const taskApi = {
  listTasks: () => req<Task[]>('/api/tasks'),
  createTask: (data: {
    title?: string
    prompt?: string
    description?: string
    ownerAgentId?: string
    flowId?: string
    boardState?: BoardState
  }) =>
    req<Task>('/api/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // (Re)generate a task title from its prompt.
  generateTaskTitle: (id: string) =>
    req<Task>(`/api/tasks/${id}/title`, { method: 'POST' }),
  updateTask: (id: string, patch: Partial<Pick<Task, 'title' | 'description' | 'prompt' | 'ownerAgentId' | 'flowId' | 'boardState'>>) =>
    req<Task>(`/api/tasks/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  deleteTask: (id: string) =>
    req<{ result: string }>(`/api/tasks/${id}`, { method: 'DELETE' }),
  runTask: (id: string) =>
    req<Run>(`/api/tasks/${id}/run`, { method: 'POST' }),
  // Run a task over SSE, streaming each activity step live, then the finished run.
  runTaskStream: (id: string, handlers: TaskStreamHandlers, signal?: AbortSignal): Promise<void> =>
    streamRunTask(id, handlers, signal),
  listTaskRuns: (id: string) => req<Run[]>(`/api/tasks/${id}/runs`),

  // Schedules (cron).
  listSchedules: () => req<Schedule[]>('/api/schedules'),
  createSchedule: (data: {
    agentId: string
    cronExpr: string
    taskId?: string
    prompt?: string
    enabled?: boolean
  }) =>
    req<Schedule>('/api/schedules', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateSchedule: (
    id: string,
    data: { agentId: string; cronExpr: string; taskId?: string; prompt?: string },
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
