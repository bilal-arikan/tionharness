// Thin API client for the SwarmGo backend.
import type {
  Agent,
  AgentPatch,
  Session,
  Message,
  ChatResponse,
  TurnStep,
  Workspace,
  Task,
  Run,
  Schedule,
  BoardState,
  Memory,
  MemoryKind,
  RecallHit,
  SessionContext,
  SessionInfo,
  AgentUsage,
  MCPServer,
  MCPTransport,
  MCPTestResult,
  AgentTools,
  WorkspaceTools,
  Flow,
  FlowGraph,
  FlowRun,
  AppSettings,
  SettingsPatch,
  ProviderTestResult,
  WorkspaceSettings,
  WorkspaceSettingsPatch,
  LogEntry,
  CatalogEntry,
  PromptsResponse,
  AppEvent,
  Artifact,
  ArtifactKind,
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

// streamChat POSTs to the SSE endpoint and dispatches parsed events. Uses fetch
// streaming (EventSource can't POST). Resolves when the stream ends.
async function streamChat(
  sessionId: string,
  message: string,
  agentIds: string[],
  handlers: {
    onMeta?: (m: { userMessage: Message; runId: string }) => void
    onAgentStart?: (a: { agentId: string; index: number }) => void
    onStep: (step: TurnStep) => void
    onReply: (r: { replyMessage: Message }) => void
    onDone: (d: { sessionTitle?: string }) => void
    onError: (err: string) => void
  },
  signal?: AbortSignal,
  thinkingLevel?: string,
): Promise<void> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (activeWorkspaceId) headers['X-Workspace-Id'] = activeWorkspaceId

  const res = await fetch('/api/chat/stream', {
    method: 'POST',
    headers,
    body: JSON.stringify({ sessionId, message, agentIds, thinkingLevel }),
    signal,
  })
  if (!res.ok || !res.body) {
    let msg = `HTTP ${res.status}`
    try {
      const body = await res.json()
      if (body?.error) msg = body.error
    } catch { /* ignore */ }
    handlers.onError(msg)
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
        handlers.onMeta?.(data as { userMessage: Message; runId: string })
        break
      case 'agent':
        handlers.onAgentStart?.(data as { agentId: string; index: number })
        break
      case 'step':
        handlers.onStep(data as TurnStep)
        break
      case 'reply':
        handlers.onReply(data as { replyMessage: Message })
        break
      case 'done':
        handlers.onDone(data as { sessionTitle?: string })
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
    // SSE frames are separated by a blank line (\n\n).
    while ((idx = buf.indexOf('\n\n')) >= 0) {
      const frame = buf.slice(0, idx)
      buf = buf.slice(idx + 2)
      if (frame.trim()) dispatch(frame)
    }
  }
}

// subscribeEvents opens the global autonomous-event SSE feed via EventSource
// (which reconnects automatically on drop). Returns an unsubscribe function.
function subscribeEvents(onEvent: (e: AppEvent) => void): () => void {
  const es = new EventSource('/api/events')
  es.addEventListener('notify', (ev) => {
    try {
      onEvent(JSON.parse((ev as MessageEvent).data) as AppEvent)
    } catch {
      // ignore malformed frames
    }
  })
  return () => es.close()
}

export const api = {
  // Workspaces (not workspace-scoped).
  listWorkspaces: () => req<Workspace[]>('/api/workspaces'),
  createWorkspace: (data: { name: string; path?: string; icon?: string; color?: string }) =>
    req<Workspace>('/api/workspaces', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  deleteWorkspace: (id: string) =>
    req<{ deleted: string }>(`/api/workspaces/${id}`, { method: 'DELETE' }),
  // Open the OS native folder picker on the backend host (local desktop app).
  pickFolder: () =>
    req<{ path: string; canceled: boolean }>('/api/pick-folder', { method: 'POST' }),

  // Agents.
  listAgents: () => req<Agent[]>('/api/agents'),
  createAgent: (data: {
    name: string
    soul?: string
    identity?: string
    provider?: string
    model?: string
    avatar?: string
    color?: string
  }) =>
    req<Agent>('/api/agents', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  updateAgent: (id: string, patch: AgentPatch) =>
    req<Agent>(`/api/agents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  // Delete an agent and the sessions it owns.
  deleteAgent: (id: string) =>
    req<{ deleted: string }>(`/api/agents/${id}`, { method: 'DELETE' }),

  // Sessions.
  // List sessions for an agent, or all sessions in the workspace when omitted.
  listSessions: (agentId?: string) =>
    req<Session[]>(
      agentId ? `/api/sessions?agentId=${encodeURIComponent(agentId)}` : '/api/sessions',
    ),
  createSession: (agentId = '', title = '') =>
    req<Session>('/api/sessions', {
      method: 'POST',
      body: JSON.stringify({ agentId, title }),
    }),
  listMessages: (sessionId: string) =>
    req<Message[]>(`/api/sessions/${sessionId}/messages`),
  // (Re)generate a session title — from its conversation, or an explicit source.
  generateSessionTitle: (sessionId: string, source?: string) =>
    req<{ id: string; title: string }>(`/api/sessions/${sessionId}/title`, {
      method: 'POST',
      body: JSON.stringify(source ? { source } : {}),
    }),
  // Set a session title verbatim (manual rename).
  setSessionTitle: (sessionId: string, title: string) =>
    req<{ id: string; title: string }>(`/api/sessions/${sessionId}/title`, {
      method: 'POST',
      body: JSON.stringify({ title }),
    }),
  // On-demand summary/listing posted as an assistant message in the session.
  // kind: 'memory' | 'board' | 'flows' | 'tools'. Returns the new message.
  summarizeSession: (sessionId: string, kind: string) =>
    req<{ userMessage: Message; replyMessage: Message }>(`/api/sessions/${sessionId}/summary`, {
      method: 'POST',
      body: JSON.stringify({ kind }),
    }),
  // Clear a session's unread flag.
  markSessionRead: (sessionId: string) =>
    req<{ id: string }>(`/api/sessions/${sessionId}/read`, { method: 'POST' }),
  // Delete a session and its on-disk folder.
  deleteSession: (sessionId: string) =>
    req<{ deleted: string }>(`/api/sessions/${sessionId}`, { method: 'DELETE' }),
  // Absolute folder holding the session's JSONL file.
  sessionPath: (sessionId: string) =>
    req<{ path: string }>(`/api/sessions/${sessionId}/path`),
  // Rich session detail: disk footprint, context composition, participating agents.
  sessionInfo: (sessionId: string) =>
    req<SessionInfo>(`/api/sessions/${sessionId}/info`),
  // Open the session's folder in the OS file manager (local desktop).
  revealSession: (sessionId: string) =>
    req<{ path: string }>(`/api/sessions/${sessionId}/reveal`, { method: 'POST' }),

  // Chat.
  chat: (sessionId: string, message: string) =>
    req<ChatResponse>('/api/chat', {
      method: 'POST',
      body: JSON.stringify({ sessionId, message }),
    }),

  // Streaming chat: receive each activity step as it happens over SSE.
  chatStream: (
    sessionId: string,
    message: string,
    agentIds: string[],
    handlers: {
      onMeta?: (m: { userMessage: Message; runId: string }) => void
      onAgentStart?: (a: { agentId: string; index: number }) => void
      onStep: (step: TurnStep) => void
      onReply: (r: { replyMessage: Message }) => void
      onDone: (d: { sessionTitle?: string }) => void
      onError: (err: string) => void
    },
    signal?: AbortSignal,
    thinkingLevel?: string,
  ): Promise<void> => streamChat(sessionId, message, agentIds, handlers, signal, thinkingLevel),

  // Control an in-flight streaming turn: stop (cancel), steer (live guidance) or
  // answer (reply to a blocked ask_user prompt).
  chatControl: (runId: string, action: 'stop' | 'steer' | 'answer', text?: string) =>
    req<{ result: string }>('/api/chat/control', {
      method: 'POST',
      body: JSON.stringify({ runId, action, text }),
    }),

  // Tasks (kanban board).
  listTasks: () => req<Task[]>('/api/tasks'),
  createTask: (data: {
    title?: string
    prompt?: string
    description?: string
    ownerAgentId?: string
    boardState?: BoardState
  }) =>
    req<Task>('/api/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  // (Re)generate a task title from its prompt.
  generateTaskTitle: (id: string) =>
    req<Task>(`/api/tasks/${id}/title`, { method: 'POST' }),
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

  // MCP servers (workspace-scoped) + per-agent tool access.
  listMCPServers: () => req<MCPServer[]>('/api/mcp-servers'),
  createMCPServer: (data: {
    name: string
    transport?: MCPTransport
    command?: string
    args?: string[]
    url?: string
    env?: Record<string, string>
  }) =>
    req<MCPServer>('/api/mcp-servers', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  toggleMCPServer: (id: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/api/mcp-servers/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  testMCPServer: (id: string) =>
    req<MCPTestResult>(`/api/mcp-servers/${id}/test`, { method: 'POST' }),
  deleteMCPServer: (id: string) =>
    req<{ result: string }>(`/api/mcp-servers/${id}`, { method: 'DELETE' }),

  agentTools: (agentId: string) => req<AgentTools>(`/api/agents/${agentId}/tools`),
  setAgentTools: (agentId: string, mcpEnabled: boolean, allowedTools: string[]) =>
    req<{ mcpEnabled: boolean; allowedTools: string[] }>(
      `/api/agents/${agentId}/tools`,
      { method: 'POST', body: JSON.stringify({ mcpEnabled, allowedTools }) },
    ),

  // Workspace-wide tool activation (active for the whole workspace; agents pick
  // from the active set).
  workspaceTools: () => req<WorkspaceTools>('/api/workspace-tools'),
  setWorkspaceTools: (disabledTools: string[]) =>
    req<{ disabledTools: string[] }>('/api/workspace-tools', {
      method: 'PUT',
      body: JSON.stringify({ disabledTools }),
    }),

  // Orchestration flows (Phase 7).
  listFlows: () => req<Flow[]>('/api/flows'),
  createFlow: (name: string, description = '', graph?: FlowGraph) =>
    req<Flow>('/api/flows', {
      method: 'POST',
      body: JSON.stringify({ name, description, graph }),
    }),
  updateFlow: (id: string, name: string, description: string, graph: FlowGraph) =>
    req<Flow>(`/api/flows/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ name, description, graph }),
    }),
  deleteFlow: (id: string) =>
    req<{ result: string }>(`/api/flows/${id}`, { method: 'DELETE' }),
  runFlow: (id: string, input: string) =>
    req<FlowRun>(`/api/flows/${id}/run`, {
      method: 'POST',
      body: JSON.stringify({ input }),
    }),
  listFlowRuns: (flowId: string) =>
    req<FlowRun[]>(`/api/flow-runs?flowId=${encodeURIComponent(flowId)}`),
  getFlowRun: (id: string) => req<FlowRun>(`/api/flow-runs/${id}`),

  // Application settings (global).
  getSettings: () => req<AppSettings>('/api/settings'),
  updateSettings: (patch: SettingsPatch) =>
    req<AppSettings>('/api/settings', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),
  testProvider: (provider: string) =>
    req<ProviderTestResult>('/api/settings/test-provider', {
      method: 'POST',
      body: JSON.stringify({ provider }),
    }),

  // Artifacts (versioned agent-produced content, workspace-scoped).
  listArtifacts: (sessionId?: string) =>
    req<Artifact[]>(
      sessionId ? `/api/artifacts?sessionId=${encodeURIComponent(sessionId)}` : '/api/artifacts',
    ),
  getArtifact: (id: string) => req<Artifact>(`/api/artifacts/${id}`),
  createArtifact: (data: {
    title: string
    kind?: ArtifactKind
    language?: string
    content?: string
    sessionId?: string
    agentId?: string
  }) =>
    req<Artifact>('/api/artifacts', { method: 'POST', body: JSON.stringify(data) }),
  updateArtifact: (
    id: string,
    patch: { content?: string; note?: string; title?: string; kind?: ArtifactKind; language?: string },
  ) => req<Artifact>(`/api/artifacts/${id}`, { method: 'PUT', body: JSON.stringify(patch) }),
  deleteArtifact: (id: string) =>
    req<{ ok: boolean }>(`/api/artifacts/${id}`, { method: 'DELETE' }),

  // Provider/model catalog (for agent + settings pickers).
  getCatalog: () => req<CatalogEntry[]>('/api/catalog'),

  // Built-in runtime prompts (read-only) + the source folder that holds them.
  getPrompts: () => req<PromptsResponse>('/api/prompts'),
  revealPrompts: () => req<{ path: string }>('/api/prompts/reveal', { method: 'POST' }),

  // Per-workspace settings (active workspace via X-Workspace-Id header).
  getWorkspaceSettings: () => req<WorkspaceSettings>('/api/workspace-settings'),
  updateWorkspaceSettings: (patch: WorkspaceSettingsPatch) =>
    req<WorkspaceSettings>('/api/workspace-settings', {
      method: 'PUT',
      body: JSON.stringify(patch),
    }),

  // Autonomous event feed (heartbeat/task/schedule) — global SSE stream.
  subscribeEvents,

  // Application + workspace logs (global ring buffer).
  getLogs: (opts?: { limit?: number; level?: string; q?: string }) => {
    const p = new URLSearchParams()
    if (opts?.limit) p.set('limit', String(opts.limit))
    if (opts?.level) p.set('level', opts.level)
    if (opts?.q) p.set('q', opts.q)
    const qs = p.toString()
    return req<LogEntry[]>(`/api/logs${qs ? `?${qs}` : ''}`)
  },
}
