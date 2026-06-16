// Core HTTP client shared by every api domain module: the JSON fetch wrapper
// and the active-workspace header that scopes each request to its isolated
// backend database.

const WS_KEY = 'swarmgo.workspaceId'
let activeWorkspaceId: string | null = localStorage.getItem(WS_KEY)

export function setActiveWorkspace(id: string) {
  activeWorkspaceId = id
  localStorage.setItem(WS_KEY, id)
}

export function getActiveWorkspace(): string | null {
  return activeWorkspaceId
}

// wsHeaders returns the base JSON headers plus X-Workspace-Id when a workspace
// is active. Used by req() and the streaming/SSE callers that bypass it.
export function wsHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (activeWorkspaceId) headers['X-Workspace-Id'] = activeWorkspaceId
  return headers
}

export async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, { headers: wsHeaders(), ...init })
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
