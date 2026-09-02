// Thin API client for the TionHarness backend.
//
// This is a barrel: the endpoints live in domain modules under ./api/* (mirroring
// the backend's internal/api/ split) and are composed into the single `api`
// object here so existing `import { api } from './api'` sites keep working.
import { setActiveWorkspace, getActiveWorkspace, clearActiveWorkspace } from './api/client'
import { workspaceApi } from './api/workspaces'
import { agentApi } from './api/agents'
import { sessionApi } from './api/sessions'
import { chatApi } from './api/chat'
import { uploadsApi } from './api/uploads'
import { taskApi } from './api/tasks'
import { graphApi } from './api/graph'
import { mcpApi } from './api/mcp'
import { hookApi } from './api/hooks'
import { lessonApi } from './api/lessons'
import { insightApi } from './api/insights'
import { flowApi } from './api/flows'
import { executionApi } from './api/executions'
import { artifactApi } from './api/artifacts'
import { secretApi } from './api/secrets'
import { skillApi } from './api/skills'
import { ingestApi } from './api/ingest'
import { marketApi } from './api/market'
import { systemApi } from './api/system'
import { providerApi } from './api/providers'
import { ttsServerApi } from './api/tts'
import { sttServerApi } from './api/stt'
import { viewApi } from './api/views'
import { trajectoryApi } from './api/trajectories'
import { dashboardApi } from './api/dashboard'

export { setActiveWorkspace, getActiveWorkspace, clearActiveWorkspace }
export type { ChatStreamHandlers } from './api/chat'

export const api = {
  ...workspaceApi,
  ...agentApi,
  ...sessionApi,
  ...chatApi,
  ...uploadsApi,
  ...taskApi,
  ...graphApi,
  ...mcpApi,
  ...hookApi,
  ...lessonApi,
  ...insightApi,
  ...flowApi,
  ...executionApi,
  ...artifactApi,
  ...secretApi,
  ...skillApi,
  ...ingestApi,
  ...marketApi,
  ...systemApi,
  ...providerApi,
  ...ttsServerApi,
  ...sttServerApi,
  ...viewApi,
  ...trajectoryApi,
  ...dashboardApi,
}
