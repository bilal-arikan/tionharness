// Per-type desktop-notification preferences. The master toggle
// (settings.desktopNotifications) gates all OS toasts; these refine it per event
// type and are device-local (stored in localStorage, applied instantly). Muting
// a type suppresses its OS toast.

// The autonomous event types the runtime publishes (see internal/events). `chat`
// is intentionally excluded: chat replies surface inside the session, never as a
// background notification.
export interface NotifTypeMeta {
  type: string
  label: string
  hint: string
}

export const NOTIFY_TYPES: NotifTypeMeta[] = [
  { type: 'task', label: 'Görevler', hint: 'Görev tamamlandı / başarısız oldu.' },
  { type: 'flow', label: 'Akışlar', hint: 'Akış (flow) çalışması tamamlandı / başarısız oldu.' },
  { type: 'schedule', label: 'Zamanlamalar', hint: 'Zamanlanmış prompt çalıştı / başarısız oldu.' },
  { type: 'agent', label: 'Ajan', hint: 'Genel ajan bildirimleri.' },
]

const KEY = 'swarmgo.notifyMutedTypes'

// We persist the MUTED set (not the enabled set), so a newly added event type
// defaults to enabled without a migration.
function readMuted(): Set<string> {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return new Set()
    const arr = JSON.parse(raw)
    return Array.isArray(arr) ? new Set(arr.map(String)) : new Set()
  } catch {
    return new Set()
  }
}

function writeMuted(muted: Set<string>) {
  try {
    localStorage.setItem(KEY, JSON.stringify([...muted]))
  } catch {
    // storage full / unavailable — ignore (prefs are best-effort).
  }
}

// isTypeEnabled reports whether OS toasts are allowed for an event type.
export function isTypeEnabled(type: string): boolean {
  return !readMuted().has(type)
}

// setTypeEnabled mutes/unmutes a type. Returns the new enabled state.
export function setTypeEnabled(type: string, enabled: boolean): boolean {
  const muted = readMuted()
  if (enabled) muted.delete(type)
  else muted.add(type)
  writeMuted(muted)
  return enabled
}

// mutedTypes returns the current muted set (for initialising UI state).
export function mutedTypes(): Set<string> {
  return readMuted()
}
