// Per-session persistence for the composer's turn overrides (reasoning level).
//
// These used to live under a single global localStorage key, which made one
// pick leak into every other session AND every other workspace: opening a new
// chat showed the level chosen somewhere else instead of the agent's own
// default. The choice is now scoped to the session that made it, so a fresh
// session starts at '' (= "use the agent's setting").
//
// Everything is kept in ONE localStorage entry (a sessionId -> value map) that
// is pruned to the most recent MAX_ENTRIES sessions, so long-lived browsers do
// not accumulate one key per session forever.

const MAX_ENTRIES = 200

type Store = { order: string[]; values: Record<string, string> }

function read(key: string): Store {
  const raw = localStorage.getItem(key)
  if (!raw) return { order: [], values: {} }
  try {
    const parsed = JSON.parse(raw) as Partial<Store>
    if (!parsed || typeof parsed !== 'object') return { order: [], values: {} }
    return {
      order: Array.isArray(parsed.order) ? parsed.order.filter((v) => typeof v === 'string') : [],
      values: parsed.values && typeof parsed.values === 'object' ? parsed.values : {},
    }
  } catch {
    // A corrupted entry is not worth failing the chat over — start clean.
    return { order: [], values: {} }
  }
}

export function readSessionOverride(key: string, sessionId: string | null): string {
  if (!sessionId) return ''
  return read(key).values[sessionId] ?? ''
}

export function writeSessionOverride(key: string, sessionId: string | null, value: string): void {
  if (!sessionId) return
  const store = read(key)
  store.order = store.order.filter((id) => id !== sessionId)
  store.order.push(sessionId)
  store.values[sessionId] = value
  while (store.order.length > MAX_ENTRIES) {
    const dropped = store.order.shift()
    if (dropped) delete store.values[dropped]
  }
  try {
    localStorage.setItem(key, JSON.stringify(store))
  } catch {
    // Ignore quota / private-mode write failures — the override still applies
    // to the running session, it just will not survive a reload.
  }
}
