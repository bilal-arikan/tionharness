import { useSyncExternalStore } from 'react'

// Client-side mirror of settings.coordinatorStallNoteVisible: whether the
// runtime's injected <coordination-guard> corrective note is RENDERED in the
// transcript. Display-only — the note is always recorded and always reaches the
// coordinator's next turn; this only decides whether the reader sees it.
//
// It lives in a module store instead of props because the value arrives with the
// app-settings payload (boot + the `settings` SSE event, both handled in the app
// shell) while the only reader sits deep inside the chat tree.

// GUARD_NOTE_ORIGIN is the Message.origin the runtime stamps on the note
// (internal/agent/coordination_stall.go: recordInjectedUserNote).
export const GUARD_NOTE_ORIGIN = 'coordination-guard'

let visible = false
const listeners = new Set<() => void>()

// setGuardNoteVisible is called from the app shell whenever app settings load or
// change, so an update_settings from an agent or another window takes effect live.
export function setGuardNoteVisible(next: boolean) {
  if (next === visible) return
  visible = next
  for (const l of listeners) l()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function snapshot() {
  return visible
}

export function useGuardNoteVisible(): boolean {
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}

// dropGuardNotes removes the guard notes when the preference is off. Kept as a
// plain function (not folded into the hook) so it is unit-testable without React.
// Returns the input array untouched when nothing is filtered, so React sees a
// stable reference in the common case.
export function dropGuardNotes<T extends { origin?: string }>(messages: T[], show: boolean): T[] {
  if (show) return messages
  return messages.some((m) => m.origin === GUARD_NOTE_ORIGIN)
    ? messages.filter((m) => m.origin !== GUARD_NOTE_ORIGIN)
    : messages
}
