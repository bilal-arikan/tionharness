// Core HTTP client shared by every api domain module: the JSON fetch wrapper
// and the active-workspace header that scopes each request to its isolated
// backend database.

const WS_KEY = 'tionswarm.workspaceId'
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
export function describeHttpError(status: number): string {
  switch (status) {
    case 502:
    case 503:
    case 504:
      return `Sunucuya ulaşılamıyor (HTTP ${status}). TionSwarm backend yanıt vermiyor — geliştirme sunucusunun (go run ./cmd/tionswarm, 127.0.0.1:8090) çalıştığından emin ol.`
    case 500:
      return 'Sunucu hatası (HTTP 500). İşlem sırasında bir şeyler ters gitti; ayrıntı için Loglar ekranına bak.'
    case 408:
      return 'İstek zaman aşımına uğradı (HTTP 408). Sunucu zamanında yanıt vermedi, tekrar dene.'
    case 429:
      return 'Çok fazla istek (HTTP 429). Lütfen biraz bekleyip tekrar dene.'
    case 404:
      return 'Bulunamadı (HTTP 404). İstenen kayıt silinmiş veya adres geçersiz olabilir.'
    case 401:
    case 403:
      return `Yetki reddedildi (HTTP ${status}). Bu işlem için izin yok.`
    case 400:
      return 'Geçersiz istek (HTTP 400). Gönderilen veri sunucu tarafından kabul edilmedi.'
    default:
      return `Beklenmeyen sunucu yanıtı (HTTP ${status}).`
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

export async function req<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, { headers: wsHeaders(), ...init })
  } catch {
    // fetch rejects (no response at all) when the dev server / network is down.
    throw new Error('Sunucuya bağlanılamadı. Ağ bağlantını ve backend\'in çalışıp çalışmadığını kontrol et.')
  }
  if (!res.ok) {
    throw new Error(await errorFromResponse(res))
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
