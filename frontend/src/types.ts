// Shared types mirroring the Go backend JSON models.

export interface Workspace {
  id: string
  name: string
  createdAt: number
  icon?: string
  color?: string
}

export interface Agent {
  id: string
  name: string
  soul: string
  identity: string
  provider: string
  model: string
  planningMode: string
  thinkingLevel?: string
  // Visual identity for the roster avatar. Both optional — when empty the UI
  // derives a deterministic circular look from the agent id.
  avatar?: string
  color?: string
  mcpEnabled: boolean
  allowedTools: string
  createdAt: number
  updatedAt: number
}

// Editable agent profile fields (PUT /api/agents/{id}). Partial — omitted keys
// are left unchanged on the backend.
export interface AgentPatch {
  name?: string
  soul?: string
  identity?: string
  provider?: string
  model?: string
  planningMode?: string
  thinkingLevel?: string
  avatar?: string
  color?: string
}

export interface Session {
  id: string
  agentId: string
  kind: string
  title: string
  messageCount: number
  state: string
  createdAt: number
  updatedAt: number
}

// A single entry in an assistant turn's activity trace (mirrors agent.TurnStep).
// 'delta' is a transient streaming text chunk (live UI only, never persisted).
// 'ask' is a transient interactive prompt (the agent is blocked on ask_user).
export type StepKind = 'text' | 'thinking' | 'tool' | 'delta' | 'ask'

export interface TurnStep {
  kind: StepKind
  text?: string
  tool?: string
  input?: unknown
  output?: string
  isError?: boolean
  // Suggested clickable answers for an 'ask' prompt.
  options?: string[]
}

// A slash command surfaced in the chat composer ("/" menu).
export interface SlashCommand {
  name: string // without the leading slash, e.g. "new"
  description: string
  icon?: string
  run: () => void
}

export interface Message {
  id: string
  sessionId: string
  role: 'user' | 'assistant' | 'system' | 'tool'
  // Which agent produced an assistant turn (empty for user/system). Multi-agent
  // sessions tag each turn so the UI can show the responding agent's avatar.
  agentId?: string
  text: string
  // JSON-encoded TurnStep[] as persisted by the backend (empty "[]" for plain
  // replies). Parsed lazily by the renderer.
  steps?: string
  reasoningContent?: string
  createdAt: number
}

export interface Usage {
  inputTokens: number
  outputTokens: number
}

export interface ChatResponse {
  reply: string
  usage: Usage
  model: string
  userMessage: Message
  replyMessage: Message
  // Live activity trace for this turn (tool calls + intermediate text).
  steps?: TurnStep[]
  // Present only when the first turn auto-generated the session title.
  sessionTitle?: string
}

// Kanban board column states (mirror db.Board* constants).
export type BoardState = 'todo' | 'in_progress' | 'review' | 'done' | 'failed'

export interface Task {
  id: string
  title: string
  description: string
  prompt: string
  ownerAgentId: string
  boardState: BoardState
  dependencies: string
  lastRunId: string
  lastRunStatus: string
  lastRunAt: number
  createdAt: number
  updatedAt: number
}

export interface Run {
  id: string
  taskId: string
  agentId: string
  status: 'pending' | 'running' | 'success' | 'failure'
  trigger: string
  output: string
  error: string
  createdAt: number
  updatedAt: number
}

export interface Schedule {
  id: string
  agentId: string
  taskId: string
  cronExpr: string
  prompt: string
  nextRunAt: number
  lastRunAt: number
  lastDeliveryStatus: string
  lastDeliveryError: string
  enabled: boolean
  createdAt: number
}

export type MemoryKind = 'document' | 'journal' | 'reflection'

export interface Memory {
  id: string
  agentId: string
  kind: MemoryKind
  content: string
  createdAt: number
}

export interface RecallHit {
  id: string
  kind: MemoryKind
  content: string
  score: number
}

export interface SessionContext {
  contextTokens: number
  hasSummary: boolean
  summaryMsgCount: number
  messageCount: number
}

export interface AgentUsage {
  day: string
  calls: number
  inputTokens: number
  outputTokens: number
  dailyCallLimit: number
  dailyTokenLimit: number
}

// MCP servers + tools (Phase 8).
export type MCPTransport = 'stdio' | 'sse' | 'http'

export interface MCPServer {
  id: string
  name: string
  transport: MCPTransport
  command: string
  args: string // JSON array
  url: string
  envConfig: string // JSON object
  enabled: boolean
  scope: string
  createdAt: number
}

// A tool advertised to the model (built-in or MCP-sourced).
export interface ToolDef {
  name: string
  description: string
  inputSchema?: unknown
}

export interface MCPTestResult {
  ok: boolean
  error?: string
  toolCount?: number
  tools?: { name: string; description: string }[]
}

export interface AgentTools {
  mcpEnabled: boolean
  allowedTools: string[]
  catalog: ToolDef[]
}

// Orchestration flows (Phase 7).
export type FlowNodeType = 'agent' | 'branch' | 'parallel'

export interface FlowBranch {
  contains: string // case-insensitive substring; "" = default
  next: string
}

export interface FlowNode {
  id: string
  type: FlowNodeType
  title?: string
  agentId?: string
  prompt?: string
  next?: string
  branches?: FlowBranch[]
  parallel?: string[]
  joinNext?: string
}

export interface FlowGraph {
  start: string
  nodes: FlowNode[]
}

export interface Flow {
  id: string
  name: string
  description: string
  graph: string // JSON FlowGraph
  createdAt: number
  updatedAt: number
}

export interface FlowTraceEntry {
  nodeId: string
  type: string
  title: string
  output: string
  at: number
}

export interface FlowState {
  current: string
  last: string
  outputs: Record<string, string>
  steps: number
  trace: FlowTraceEntry[]
}

export interface FlowRun {
  id: string
  flowId: string
  status: 'running' | 'success' | 'failure'
  input: string
  state: string // JSON FlowState
  output: string
  error: string
  createdAt: number
  updatedAt: number
}

// Application settings (global). Mirrors settings.DTO — the Anthropic key is
// never returned; anthropicKeySet reports whether one is stored.
export type Theme = 'dark' | 'light' | 'system'

export interface AppSettings {
  theme: Theme
  accent: string
  language: 'tr' | 'en'

  defaultProvider: string
  defaultModel: string
  claudeCliPath: string
  anthropicKeySet: boolean
  minimaxKeySet: boolean
  minimaxBaseUrl: string

  oneMillionContext: boolean
  extendedPromptCache: boolean

  desktopNotifications: boolean
  keepAwake: boolean

  userName: string
  userTimezone: string
  userCity: string
  userCountry: string
  userNotes: string

  maxContextTokens: number
  keepRecentMsgs: number
  recallTopN: number
  recallMinScore: number

  defaultDailyCallLimit: number
  defaultDailyTokenLimit: number

  defaultHeartbeatSec: number
  pauseAutonomy: boolean

  autoTitleEnabled: boolean
  titleModel: string

  mcpGatewayUrl: string

  logLevel: string
}

// Partial update. anthropicKey/minimaxKey are write-only: "" clears, non-empty sets.
export type SettingsPatch = Partial<
  Omit<AppSettings, 'anthropicKeySet' | 'minimaxKeySet'> & {
    anthropicKey: string
    minimaxKey: string
  }
>

export interface ProviderTestResult {
  ok: boolean
  model?: string
  sample?: string
  error?: string
}

// Provider/model catalog for the UI's pickers.
export interface CatalogModel {
  id: string
  label: string
  description?: string
}

export interface CatalogEntry {
  id: string
  label: string
  needsKey: boolean
  allowCustomModel: boolean
  available: boolean
  models: CatalogModel[]
}

// Per-workspace settings (overrides + rename). Resolved from X-Workspace-Id.
export interface WorkspaceSettings {
  id: string
  name: string
  description: string
  icon: string
  color: string
  defaultProvider: string
  defaultModel: string
  pauseAutonomy: boolean
  createdAt: number
  agentCount: number
  sessionCount: number
  taskCount: number
}

export type WorkspaceSettingsPatch = Partial<
  Pick<WorkspaceSettings, 'name' | 'description' | 'icon' | 'color' | 'defaultProvider' | 'defaultModel' | 'pauseAutonomy'>
>

// A captured log record (application + all workspaces).
export interface LogEntry {
  seq: number
  time: number // unix milliseconds
  level: string // DEBUG | INFO | WARN | ERROR
  message: string
  attrs?: Record<string, string>
}

// AppEvent is an autonomous runtime notification streamed over /api/events.
// `target` carries navigation hints used to deep-link on notification click
// (keys: view, sessionId, taskId, agentId).
export interface AppEvent {
  type: string // task | schedule | heartbeat | agent
  level: 'info' | 'success' | 'error'
  workspaceId: string
  workspaceName?: string
  title: string
  body: string
  target?: Record<string, string>
  time: number // unix seconds
}
