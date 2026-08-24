// App-wide UI sound cues, synthesized via Web Audio (no asset files): the mic
// start/stop blips and the chat turn-done chime. Every cue is gated by a single
// device-local "sound effects" preference (localStorage), toggled from Settings
// and applied instantly. Best-effort: any AudioContext failure (autoplay policy,
// unsupported) is swallowed so a missing cue never blocks the triggering action.

const PREF_KEY = 'tionharness.soundEffects'

// Sounds default ON; the stored value only records an explicit opt-out ('0').
export function soundEffectsEnabled(): boolean {
  try {
    return localStorage.getItem(PREF_KEY) !== '0'
  } catch {
    return true
  }
}

export function setSoundEffectsEnabled(on: boolean) {
  try {
    localStorage.setItem(PREF_KEY, on ? '1' : '0')
  } catch {
    // storage full / unavailable — ignore (prefs are best-effort).
  }
}

let ctx: AudioContext | null = null

function audioCtx(): AudioContext | null {
  try {
    const Ctor =
      window.AudioContext ??
      (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext
    if (!Ctor) return null
    if (!ctx) ctx = new Ctor()
    return ctx
  } catch {
    return null
  }
}

// tone plays a single sine note after delayMs, with a quick attack + exponential
// decay so it reads as a soft blip rather than a flat beep.
function tone(freq: number, durationMs: number, delayMs = 0, peak = 0.15) {
  const ac = audioCtx()
  if (!ac) return
  // A prior user gesture unlocks a suspended context; resume best-effort.
  if (ac.state === 'suspended') ac.resume().catch(() => {})
  const start = () => {
    const osc = ac.createOscillator()
    const gain = ac.createGain()
    osc.type = 'sine'
    osc.frequency.value = freq
    const now = ac.currentTime
    const dur = durationMs / 1000
    gain.gain.setValueAtTime(0.0001, now)
    gain.gain.exponentialRampToValueAtTime(peak, now + 0.012)
    gain.gain.exponentialRampToValueAtTime(0.0001, now + dur)
    osc.connect(gain).connect(ac.destination)
    osc.start(now)
    osc.stop(now + dur)
  }
  if (delayMs > 0) window.setTimeout(start, delayMs)
  else start()
}

// Rising two-tone cue when dictation starts.
export function playMicStart() {
  if (!soundEffectsEnabled()) return
  tone(660, 90)
  tone(990, 110, 90)
}

// Single lower tone when dictation stops.
export function playMicStop() {
  if (!soundEffectsEnabled()) return
  tone(440, 130)
}

// Gentle rising "ta-da" chime when an assistant reply completes.
export function playTurnDone() {
  if (!soundEffectsEnabled()) return
  tone(784, 140, 0, 0.13) // G5
  tone(1047, 200, 110, 0.13) // C6
}

// Attention cue when a turn BLOCKS waiting on the user (ask_user question,
// permission approval, plan approval). Deliberately unlike playTurnDone: that one
// resolves upward and stops, this one rises and then falls back — an unfinished,
// questioning shape — so "the agent needs you" is never mistaken for "the agent
// is done". Slightly louder, since the turn stays stalled until it is answered.
export function playAskPrompt() {
  if (!soundEffectsEnabled()) return
  tone(880, 120, 0, 0.17) // A5
  tone(1175, 130, 120, 0.17) // D6
  tone(880, 190, 260, 0.15) // back to A5 — "still waiting on you"
}

// Attention cue specifically for a tool-execution PERMISSION request — the agent
// wants to run something and needs a yes/no before it can act. Deliberately
// distinct from both playTurnDone (resolves upward, done) and playAskPrompt
// (rises then falls, an open question): this one alternates two notes sharply
// (a "buzz") so an approval gate reads differently at a glance from a plain
// question or a completed reply.
export function playPermissionPrompt() {
  if (!soundEffectsEnabled()) return
  tone(700, 90, 0, 0.18)
  tone(1000, 90, 100, 0.18)
  tone(700, 90, 200, 0.18)
  tone(1000, 140, 300, 0.18)
}
