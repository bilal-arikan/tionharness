// PreToolUse / PostToolUse hooks (workspace-scoped, Phase P4).
import type { Hook, HookEvent, BuiltinHook } from '../types'
import { req } from './client'

export interface HookInput {
  event: HookEvent
  matcher: string
  command: string
  timeoutSec: number
  enabled: boolean
}

export const hookApi = {
  listHooks: () => req<Hook[]>('/api/hooks'),
  listBuiltinHooks: () => req<BuiltinHook[]>('/api/hooks/builtins'),
  createHook: (data: HookInput) =>
    req<Hook>('/api/hooks', { method: 'POST', body: JSON.stringify(data) }),
  updateHook: (id: string, data: HookInput) =>
    req<Hook>(`/api/hooks/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  toggleHook: (id: string, enabled: boolean) =>
    req<{ enabled: boolean }>(`/api/hooks/${id}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  deleteHook: (id: string) =>
    req<{ result: string }>(`/api/hooks/${id}`, { method: 'DELETE' }),
}
