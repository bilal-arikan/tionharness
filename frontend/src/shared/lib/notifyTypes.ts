// Single source of truth for notifiable event types. Everything about "what can
// become a desktop notification" derives from this one list:
//   - the per-type mute settings UI (NotificationsPanel) maps over it,
//   - the nav-view badge mapping (eventViews.viewForEventType) reads its `view`,
//   - the toast funnel (notifyBus.emitToast) reads its `cue` for the sound.
//
// It mirrors the backend `events.NotifyKinds` set (internal/events/types.go) and
// adds one frontend-only kind, `prompt`, for ask/permission/plan cues, which are a
// client-side session-stream signal with no backend event. Keep the two lists in
// sync when adding a notifiable type — that is exactly what stops the settings
// screen from drifting out of sync with what the app actually notifies.

import { sharedText } from './sharedI18n'

// Which sound cue a type plays. `done` = the rising "reply ready" chime;
// `ask` = the attention cue for a turn blocked on the user; `permission` = the
// distinct cue for a tool-approval prompt specifically (see notifyBus.playCue's
// cue override — the 'prompt' type's default cue stays 'ask', 'permission' is
// selected per-event by the caller, not looked up from this table); null =
// silent toast.
export type NotifyCue = 'done' | 'ask' | 'permission' | null

export interface NotifyType {
  // Event type key (matches events.Event.type) or the frontend-only kind 'prompt'.
  type: string
  // Human label shown in Settings.
  label: string
  // One-line explanation shown under the toggle.
  hint: string
  // Nav view id whose unread dot this event lights (null = app-global, no badge).
  // Kept identical to the historical viewForEventType mapping.
  view: string | null
  // Sound cue played when this fires (device-local sound-effects pref still gates it).
  cue: NotifyCue
}

function notifyType(type: string, view: string | null, cue: NotifyCue): NotifyType {
  return {
    type,
    get label() {
      return sharedText(`notifications.${type}.label`)
    },
    get hint() {
      return sharedText(`notifications.${type}.hint`)
    },
    view,
    cue,
  }
}

export const NOTIFY_TYPES: NotifyType[] = [
  notifyType('chat', 'chat', 'done'),
  notifyType('prompt', 'chat', 'ask'),
  notifyType('schedule', 'schedules', null),
  notifyType('flow', 'flows', null),
  notifyType('spawned', 'chat', null),
  notifyType('worker', 'chat', null),
  notifyType('coordination', null, null),
  // Frontend-only (workspace stream, useWorkspaceSignals): trajectory start/end
  // and the other Rota-screen facts surfaced as toasts.
  notifyType('rota', 'rota', null),
  notifyType('automation', null, null),
  notifyType('artifact', 'artifacts', null),
  notifyType('board', 'board', null),
  notifyType('agent', null, null),
  notifyType('anomaly', null, null),
  notifyType('agent-model-changed', 'agents', null),
]

const byType = new Map(NOTIFY_TYPES.map((t) => [t.type, t]))

// notifyTypeMeta returns a type's metadata, or undefined for an unregistered type.
export function notifyTypeMeta(type: string): NotifyType | undefined {
  return byType.get(type)
}

// cueForType reports which sound cue a type plays (null = silent).
export function cueForType(type: string): NotifyCue {
  return byType.get(type)?.cue ?? null
}

// viewIdForType maps an event type to the nav-view id its badge lights, or null
// for app-global / unbadged types. `task` is a legacy alias for `board`: task
// changes are emitted as `board` events, never a distinct `task` event.
export function viewIdForType(type: string): string | null {
  if (type === 'task') return 'board'
  return byType.get(type)?.view ?? null
}
