// Thin API client for the SwarmGo backend.
import type {
  Agent,
  Session,
  Message,
  ChatResponse,
  Workspace,
  Task,
  Run,
  Schedule,
  BoardState,
  Memory,
  MemoryKind,
  RecallHit,
  SessionContext,
  AgentUsage,
} from './types'

// Active workspace — sent as X-Workspace-Id on every request so the backend
// routes to the correct isolated database.
const WS_KEY = 'swarmgo.workspaceId'
let activeWorkspaceId: string | null = localStorage.getItem(WS_KEY)

export function setActiveWorkspace(id: string) {
  activeWorkspaceId = id
  localStorage.setItem(WS_KEY, id)
}

export function getActiveWorkspace(): string | null {
  return activeWorkspaceId
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (activeWorkspaceId) headers['X-Workspace-Id'] = activeWorkspaceId

  const res = await fetch(path, { headers, ...init })
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      // ignore
    }
    throw new Error(msg)
  }
  return res.json() as Promise<T>
}

export const api = {
  // Workspaces (not workspace-scoped).
  listWorkspaces: () => req<Workspace[]>('/api/workspaces'),
  createWorkspace: (name: string) =>
    req<Workspace>('/api/workspaces', {
      method: 'POST',
      body: JSON.stringify({ name }),
    }),
  deleteWorkspace: (id: string) =>
    req<{ deleted: string }>(`/api/workspaces/${id}`, { method: 'DELETE' }),

  // Agents.
  listAgents: () => req<Agent[]>('/api/agents'),
  createAgent: (data: {
    name: string
    soul?: string
    identity?: string
    provider?: string
    model?: string
  }) =>
    req<Agent>('/api/agents', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  // Sessions.
  listSessions: (agentId: string) =>
    req<Session[]>(`/api/sessions?agentId=${encodeURIComponent(agentId)}`),
  createSession: (agentId: string, title = '') =>
    req<Session>('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ agentId, title }),
    }),
  listMessages: (sessionId: string) =>
    req<Message[]>(`/api/sessions/${sessionId}/messages`),

  // Chat.
  chat: (sessionId: string, message: string) =>
    req<ChatResponse>('/api/chat', {
      method: 'POST',
      body: JSON.stringify({ sessionId, message }),
    }),

  // Tasks (kanban board).
  listTasks: () => req<Task[]>('/api/tasks'),
  createTask: (data: {
    title: string
    prompt?: string
    description?: string
    ownerAgentId?: string
    boardState?: BoardState
  }) =>
    req<Task>('/api/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateTask: (id: string, patch: Partial<Pick<Task, 'title' | 'description' | 'prompt' | 'ownerAgentId' | 'boardState'>>) =>
    req<Task>(`/api/tasks/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  deleteTask: (id: string) =>
    req<{ result: string }>(`/api/tasks/${id}`, { method: 'DELETE' }),
  runTask: (id: string) =>
    req<Run>(`/api/tasks/${id}/run`, { method: 'POST' }),
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
  toggleSchedule: (id: string, enabled: boolean) =>
    req<{ id: string; enabled: boolean }>(`/api/schedules/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  deleteSchedule: (id: string) =>
    req<{ result: string }>(`/api/schedules/${id}`, { method: 'DELETE' }),

  // Memory (per-agent: documents, journal, reflections).
  listMemories: (agentId: string, kind?: MemoryKind) =>
    req<Memory[]>(
      `/api/agents/${agentId}/memories${kind ? `?kind=${kind}` : ''}`,
    ),
  createMemory: (agentId: string, content: string, kind: MemoryKind = 'document') =>
    req<Memory>(`/api/agents/${agentId}/memories`, {
      method: 'POST',
      body: JSON.stringify({ content, kind }),
    }),
  deleteMemory: (id: string) =>
    req<{ result: string }>(`/api/memories/${id}`, { method: 'DELETE' }),
  reflect: (agentId: string) =>
    req<Memory>(`/api/agents/${agentId}/reflect`, { method: 'POST' }),
  recall: (agentId: string, query: string, limit = 5) =>
    req<RecallHit[]>(`/api/agents/${agentId}/recall`, {
      method: 'POST',
      body: JSON.stringify({ query, limit }),
    }),

  // Context meter + budget guardrails.
  sessionContext: (sessionId: string) =>
    req<SessionContext>(`/api/sessions/${sessionId}/context`),
  agentUsage: (agentId: string) => req<AgentUsage>(`/api/agents/${agentId}/usage`),
  setBudget: (agentId: string, dailyCallLimit: number, dailyTokenLimit: number) =>
    req<{ dailyCallLimit: number; dailyTokenLimit: number }>(
      `/api/agents/${agentId}/budget`,
      { method: 'POST', body: JSON.stringify({ dailyCallLimit, dailyTokenLimit }) },
    ),
}
