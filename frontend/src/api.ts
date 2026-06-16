// Thin API client for the SwarmGo backend.
//
// This is a barrel: the endpoints live in domain modules under ./api/* (mirroring
// the backend's internal/api/ split) and are composed into the single `api`
// object here so existing `import { api } from './api'` sites keep working.
import { setActiveWorkspace, getActiveWorkspace } from './api/client'
import { workspaceApi } from './api/workspaces'
import { agentApi } from './api/agents'
import { sessionApi } from './api/sessions'
import { chatApi } from './api/chat'
import { uploadsApi } from './api/uploads'
import { taskApi } from './api/tasks'
import { memoryApi } from './api/memory'
import { mcpApi } from './api/mcp'
import { flowApi } from './api/flows'
import { artifactApi } from './api/artifacts'
import { systemApi } from './api/system'

export { setActiveWorkspace, getActiveWorkspace }
export type { ChatStreamHandlers } from './api/chat'

export const api = {
  ...workspaceApi,
  ...agentApi,
  ...sessionApi,
  ...chatApi,
  ...uploadsApi,
  ...taskApi,
  ...memoryApi,
  ...mcpApi,
  ...flowApi,
  ...artifactApi,
  ...systemApi,
}
