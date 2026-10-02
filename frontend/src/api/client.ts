// Core HTTP client shared by every api domain module: the JSON fetch wrapper
// and the active-workspace header that scopes each request to its isolated
// backend database.

import { sharedText } from '@/shared/lib/sharedI18n'
import { requestDeadline } from './requestDeadline'

const WS_KEY = 'tionharness.workspaceId'
let activeWorkspaceId: string | null = localStorage.getItem(WS_KEY)

export function setActiveWorkspace(id: string) {
  activeWorkspaceId = id
  localStorage.setItem(WS_KEY, id)
}

export function getActiveWorkspace(): string | null {
  return activeWorkspaceId
}

// clearActiveWorkspace drops the active-workspace pointer entirely (used when the
// last workspace is deleted → the app returns to the onboarding screen). After
// this, requests carry no X-Workspace-Id and the backend resolves no workspace.
export function clearActiveWorkspace() {
  activeWorkspaceId = null
  localStorage.removeItem(WS_KEY)
}

// wsHeaders returns the base JSON headers plus X-Workspace-Id when a workspace
// is active. Used by req() and the streaming/SSE callers that bypass it.
export function wsHeaders(): Record<string, string> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (activeWorkspaceId) headers['X-Workspace-Id'] = activeWorkspaceId
  return headers
}

// describeHttpError maps a non-OK HTTP status to a human, actionable message.
// A 502/503/504 from the Vite dev proxy means the Go backend is unreachable
// (most often: it isn't running) — the bare "HTTP 502" the user used to see
// gave no hint about that, so we spell it out.
function describeHttpError(status: number): string {
  switch (status) {
    case 502:
    case 503:
    case 504:
      return sharedText('api.http.backendUnavailable', { status })
    case 500:
      return sharedText('api.http.serverError')
    case 408:
      return sharedText('api.http.timeout')
    case 429:
      return sharedText('api.http.tooManyRequests')
    case 404:
      return sharedText('api.http.notFound')
    case 401:
    case 403:
      return sharedText('api.http.forbidden', { status })
    case 400:
      return sharedText('api.http.badRequest')
    default:
      return sharedText('api.http.unexpected', { status })
  }
}

// errorFromResponse builds the best message for a non-OK response: a backend
// JSON `{error}` payload wins (most specific); otherwise — e.g. a non-JSON
// proxy/gateway page — fall back to the status-based description.
export async function errorFromResponse(res: Response): Promise<string> {
  try {
    const body = await res.json()
    if (body?.error) return String(body.error)
  } catch {
    // Non-JSON body (proxy 502 HTML, empty 504, …) → use the status description.
  }
  return describeHttpError(res.status)
}

export type APIRequestInit = RequestInit & { timeoutMs?: number }

export async function req<T>(path: string, options?: APIRequestInit): Promise<T> {
  const { timeoutMs, ...init } = options ?? {}
  const deadline = requestDeadline(init.signal, timeoutMs)
  try {
    return await requestWithDeadline<T>(path, init, deadline)
  } catch (error) {
    if (init?.signal?.aborted) throw error
    if (deadline.timedOut())
      throw new Error(sharedText('api.network.requestTimedOut'), { cause: error })
    throw error
  } finally {
    deadline.close()
  }
}

async function requestWithDeadline<T>(
  path: string,
  init: RequestInit,
  deadline: ReturnType<typeof requestDeadline>,
): Promise<T> {
  const { signal } = deadline
  let res: Response
  try {
    // no-store: API responses are live workspace state, never cacheable. Without
    // this the browser may heuristically serve a stale GET (e.g. /api/hooks after
    // an out-of-band change), so a panel shows outdated data until a hard reload.
    // A caller may still override via init.cache.
    res = await fetch(path, { cache: 'no-store', headers: wsHeaders(), ...init, signal })
  } catch (error) {
    if (signal.aborted) throw error
    // fetch rejects (no response at all) when the dev server / network is down.
    throw new Error(sharedText('api.network.connectionFailed'), { cause: error })
  }
  if (!res.ok) {
    if (res.status === 409) {
      const body = await res
        .clone()
        .json()
        .catch(() => null)
      if (body?.confirmationRequired === true) {
        // Human approval is not a network stall. A confirmed retry gets its
        // own deadline rather than inheriting time spent in the dialog.
        deadline.close()
        const names = (body.workspaces as { workspaceName: string }[])
          .map((workspace) => `• ${workspace.workspaceName}`)
          .join('\n')
        if (
          !window.confirm(
            sharedText('api.sharedAgent.confirm', { agentName: body.agentName, workspaces: names }),
          )
        ) {
          throw new Error(sharedText('api.sharedAgent.cancelled'))
        }
        return req<T>(path, {
          ...init,
          headers: {
            ...wsHeaders(),
            ...Object.fromEntries(new Headers(init?.headers)),
            'X-Confirm-Shared-Agent': 'true',
          },
        })
      }
    }
    const err = new Error(await errorFromResponse(res)) as Error & { status?: number }
    err.status = res.status
    throw err
  }
  // Tolerate empty bodies (204 No Content, or any handler that writes no JSON):
  // parsing "" would throw "Unexpected end of JSON input". Endpoints typed as
  // req<void> rely on this.
  if (res.status === 204) {
    return undefined as T
  }
  const text = await res.text()
  if (text.trim() === '') {
    return undefined as T
  }
  return JSON.parse(text) as T
}
