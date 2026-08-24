// Per-type desktop-notification preferences: the mute/unmute persistence layer.
// The master toggle (settings.desktopNotifications) gates all OS toasts; these
// refine it per event type and are device-local (localStorage, applied instantly).
// Muting a type suppresses its OS toast (its sound cue, if any, still follows the
// separate sound-effects pref — see notifyBus.playCue).
//
// The catalogue of notifiable types lives in ONE place — notifyTypes.ts — so this
// module only owns persistence; the UI and the toast funnel read the same list.
export { NOTIFY_TYPES } from './notifyTypes'
export type { NotifyType } from './notifyTypes'

const KEY = 'tionharness.notifyMutedTypes'

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
