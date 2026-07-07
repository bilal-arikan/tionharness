// Small pure helpers shared by the chat-stream modules.

// Grace window before a staged steer is actually POSTed to the running turn
// (removing the item within this window cancels it).
export const STEER_GRACE_MS = 3000

// Immutable Set/Record helpers for the per-session streaming state.
export function withAdded(prev: ReadonlySet<string>, v: string): ReadonlySet<string> {
  if (prev.has(v)) return prev
  const n = new Set(prev)
  n.add(v)
  return n
}
export function withRemoved(prev: ReadonlySet<string>, v: string): ReadonlySet<string> {
  if (!prev.has(v)) return prev
  const n = new Set(prev)
  n.delete(v)
  return n
}
export function withoutKey<T>(prev: Record<string, T>, key: string): Record<string, T> {
  if (!(key in prev)) return prev
  const n = { ...prev }
  delete n[key]
  return n
}

// flowSlug turns a flow name into a space-less "/" command token (Turkish chars
// folded to ASCII), e.g. "Geri Bildirim Yönlendirici" → "geri-bildirim-yonlendirici".
export function flowSlug(name: string): string {
  const map: Record<string, string> = { ı: 'i', İ: 'i', ş: 's', ğ: 'g', ü: 'u', ö: 'o', ç: 'c' }
  return name
    .replace(/[ıİşğüöç]/g, (c) => map[c] ?? c)
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// Stable id for a pending item (no crypto needed — display/dedup only).
export const pendingId = () => `p-${Date.now()}-${Math.round(Math.random() * 1e6)}`
