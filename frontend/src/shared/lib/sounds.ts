// App-wide UI sound cues, synthesized via Web Audio (no asset files): the mic
// start/stop blips and the chat turn-done chime. Every cue is gated by a single
// device-local "sound effects" preference (localStorage), toggled from Settings
// and applied instantly. Best-effort: any AudioContext failure (autoplay policy,
// unsupported) is swallowed so a missing cue never blocks the triggering action.

const PREF_KEY = 'tionswarm.soundEffects'

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
