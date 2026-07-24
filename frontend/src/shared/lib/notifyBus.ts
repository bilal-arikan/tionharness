// The single funnel every desktop-notification path calls. Before this existed the
// "should this become a toast (and a sound)?" decision was re-derived in three
// places — the SSE chat branch, the SSE generic branch, and the session-stream ask
// path — each with its own gating, so per-type mute worked in only one of them and
// the chat/ask cues could never be silenced. emitToast centralises it:
//
//   1. play the type's sound cue (gated only by the device-local sound-effects
//      pref inside sounds.ts — a focused window still hears it, and it is NOT
//      suppressed by muting the type; the type mute governs the OS toast only),
//   2. drop out if the type is muted in Settings (per-type, device-local),
//   3. raise the OS toast (gated by the master switch + backgrounded-window rule
//      inside clientPrefs.notify).
//
// Callers pass the resolved master `enabled` (notifyEnabled.current) so a Settings
// change applies without resubscribing.
import { notify } from './clientPrefs'
import { isTypeEnabled } from './notifyPrefs'
import { cueForType } from './notifyTypes'
import { playTurnDone, playAskPrompt } from './sounds'

export interface ToastRequest {
  // The notify type (drives the sound cue + the per-type mute). See notifyTypes.ts.
  type: string
  // Resolved master desktop-notification gate (global master ⊕ workspace override).
  enabled: boolean
  title: string
  body?: string
  // Stable, event-derived tag so multiple open windows collapse into one OS toast.
  tag?: string
  // Runs when the user clicks the toast (after the window is focused).
  onClick?: () => void
}

// playCue fires the type's sound cue (if any). Gated only by the sound-effects
// pref (inside sounds.ts); independent of the toast gates so a focused window
// still hears "done"/"ask" even though no OS toast shows.
export function playCue(type: string) {
  switch (cueForType(type)) {
    case 'done':
      playTurnDone()
      break
    case 'ask':
      playAskPrompt()
      break
  }
}

// emitToast plays the cue then raises the OS toast subject to the per-type mute and
// the master gate. Best-effort; safe to fire-and-forget.
export function emitToast(req: ToastRequest) {
  playCue(req.type)
  if (!isTypeEnabled(req.type)) return
  notify(req.enabled, req.title, req.body ?? '', req.onClick, req.tag)
}
