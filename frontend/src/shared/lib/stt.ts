// Server (whisper.cpp) STT engine selection + status, mirroring shared/lib/tts.ts.
// The browser Web Speech API stays the default/fallback; when the host has
// whisper-cli + ffmpeg + a model, the server engine can transcribe uploaded audio
// (works in WebView2 / thin clients that lack Web Speech recognition).
import { api } from '@/api'
import type { SttStatus } from '@/api/stt'

const ENGINE_KEY = 'tionharness.stt.engine'
const MODEL_KEY = 'tionharness.stt.model'

export type SttEngine = 'auto' | 'browser' | 'server'

export function sttEngine(): SttEngine {
  try {
    const v = localStorage.getItem(ENGINE_KEY)
    return v === 'browser' || v === 'server' ? v : 'auto'
  } catch {
    return 'auto'
  }
}

export function setSttEngine(v: SttEngine) {
  try {
    localStorage.setItem(ENGINE_KEY, v)
  } catch {
    // best-effort
  }
}

export function sttModel(): string {
  try {
    return localStorage.getItem(MODEL_KEY) || ''
  } catch {
    return ''
  }
}

export function setSttModel(v: string) {
  try {
    localStorage.setItem(MODEL_KEY, v)
  } catch {
    // best-effort
  }
}

// browserSttSupported reports whether the browser has Web Speech recognition.
function browserSttSupported(): boolean {
  if (typeof window === 'undefined') return false
  const w = window as unknown as { SpeechRecognition?: unknown; webkitSpeechRecognition?: unknown }
  return !!(w.SpeechRecognition ?? w.webkitSpeechRecognition)
}

// Cached server-engine status (whisper availability + model list), loaded via
// initServerStt and reused by the resolver + settings UI.
let serverState: SttStatus = { available: false, models: [] }

export function serverSttStatus(): SttStatus {
  return serverState
}

export async function initServerStt(): Promise<SttStatus> {
  try {
    serverState = await api.sttStatus()
  } catch {
    serverState = { available: false, models: [] }
  }
  return serverState
}

// resolveSttEngine decides which engine actually runs, honouring the pref and
// real availability. 'none' when neither is usable.
export function resolveSttEngine(): 'server' | 'browser' | 'none' {
  const pref = sttEngine()
  const serverOk = serverState.available
  const browserOk = browserSttSupported()
  if (pref === 'server') return serverOk ? 'server' : browserOk ? 'browser' : 'none'
  if (pref === 'browser') return browserOk ? 'browser' : serverOk ? 'server' : 'none'
  return serverOk ? 'server' : browserOk ? 'browser' : 'none'
}
