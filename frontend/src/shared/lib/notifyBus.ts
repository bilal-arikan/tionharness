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
import { cueForType, type NotifyCue } from './notifyTypes'
import { playTurnDone, playAskPrompt, playPermissionPrompt } from './sounds'

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
  // Overrides the type's default cue (notifyTypes.cueForType) for this one call.
  // The 'prompt' notify type covers ask/permission/plan interactions alike so the
  // OS-toast mute stays a single "Onay / soru" toggle; the caller picks the
  // per-event sound (e.g. 'permission' for a tool-approval prompt) without
  // splitting that toggle into more notify types.
  cue?: NotifyCue
}

// playCue fires a sound cue (if any). Gated only by the sound-effects pref
// (inside sounds.ts); independent of the toast gates so a focused window still
// hears "done"/"ask"/"permission" even though no OS toast shows. Pass
// cueOverride to play a specific cue regardless of the type's configured
// default (see ToastRequest.cue).
export function playCue(type: string, cueOverride?: NotifyCue) {
  switch (cueOverride ?? cueForType(type)) {
    case 'done':
      playTurnDone()
      break
    case 'ask':
      playAskPrompt()
      break
    case 'permission':
      playPermissionPrompt()
      break
  }
}

// emitToast plays the cue then raises the OS toast subject to the per-type mute and
// the master gate. Best-effort; safe to fire-and-forget.
export function emitToast(req: ToastRequest) {
  playCue(req.type, req.cue)
  if (!isTypeEnabled(req.type)) return
  notify(req.enabled, req.title, req.body ?? '', req.onClick, req.tag)
}
