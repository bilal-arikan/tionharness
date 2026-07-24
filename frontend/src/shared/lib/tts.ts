// Text-to-speech for assistant replies via the browser SpeechSynthesis API:
// client-side, multilingual, and — unlike the microphone — needs NO secure
// context or permission, so it works over plain HTTP/LAN too. Only PROSE is
// spoken: fenced code, inline code, tables, images, links, bare URLs and file
// paths are stripped so the voice never dictates code or tool output.
import type { Message } from '@/types'
import { api } from '@/api'
import type { TtsStatus } from '@/api/tts'

const AUTO_KEY = 'tionswarm.tts.autoRead'
const LANG_KEY = 'tionswarm.tts.lang'
const VOICE_KEY = 'tionswarm.tts.voiceURI'
const RATE_KEY = 'tionswarm.tts.rate'
const PITCH_KEY = 'tionswarm.tts.pitch'
// Engine selection ('auto' | 'browser' | 'server') and the chosen server (Piper)
// voice id. Auto prefers the server engine when the host has Piper installed.
const ENGINE_KEY = 'tionswarm.tts.engine'
const SERVER_VOICE_KEY = 'tionswarm.tts.serverVoice'
// Fallback voice language: the dictation language the user already picked in the
// composer (kept as a plain string to avoid a features→shared import edge).
const STT_LANG_KEY = 'tionswarm.stt.lang'

// Rate/pitch bounds (SpeechSynthesisUtterance accepts rate 0.1–10, pitch 0–2; we
// expose a sane, musical subset). Defaults are the neutral 1.0.
export const TTS_RATE_MIN = 0.5
export const TTS_RATE_MAX = 2
export const TTS_PITCH_MIN = 0
export const TTS_PITCH_MAX = 2

export function ttsSupported(): boolean {
  return typeof window !== 'undefined' && 'speechSynthesis' in window
}

// Auto-read is OFF by default (opt-in); records only an explicit enable.
export function ttsAutoRead(): boolean {
  try {
    return localStorage.getItem(AUTO_KEY) === '1'
  } catch {
    return false
  }
}

export function setTtsAutoRead(on: boolean) {
  try {
    localStorage.setItem(AUTO_KEY, on ? '1' : '0')
  } catch {
    // best-effort
  }
}

// Resolved voice language: an explicit TTS choice, else the composer's dictation
// language, else '' (let the engine pick its default voice).
export function ttsLang(): string {
  try {
    return localStorage.getItem(LANG_KEY) || localStorage.getItem(STT_LANG_KEY) || ''
  } catch {
    return ''
  }
}

export function setTtsLang(v: string) {
  try {
    localStorage.setItem(LANG_KEY, v)
  } catch {
    // best-effort
  }
}

// Explicit voice choice by voiceURI ('' = auto: pick by language). Rate/pitch are
// numeric, clamped to their exposed bounds with a 1.0 default.
export function ttsVoiceURI(): string {
  try {
    return localStorage.getItem(VOICE_KEY) || ''
  } catch {
    return ''
  }
}

export function setTtsVoiceURI(v: string) {
  try {
    localStorage.setItem(VOICE_KEY, v)
  } catch {
    // best-effort
  }
}

function clampNum(n: number, min: number, max: number, fallback: number): number {
  return Number.isFinite(n) && n >= min && n <= max ? n : fallback
}

export function ttsRate(): number {
  try {
    return clampNum(Number(localStorage.getItem(RATE_KEY)), TTS_RATE_MIN, TTS_RATE_MAX, 1)
  } catch {
    return 1
  }
}

export function setTtsRate(n: number) {
  try {
    localStorage.setItem(RATE_KEY, String(n))
  } catch {
    // best-effort
  }
}

export function ttsPitch(): number {
  try {
    return clampNum(Number(localStorage.getItem(PITCH_KEY)), TTS_PITCH_MIN, TTS_PITCH_MAX, 1)
  } catch {
    return 1
  }
}

export function setTtsPitch(n: number) {
  try {
    localStorage.setItem(PITCH_KEY, String(n))
  } catch {
    // best-effort
  }
}

// The engine's available voices (empty until the async 'voiceschanged' fires on
// some browsers — subscribe with onVoicesChanged to refresh a UI list).
export function ttsVoices(): SpeechSynthesisVoice[] {
  if (!ttsSupported()) return []
  return window.speechSynthesis.getVoices()
}

export function onVoicesChanged(cb: () => void): () => void {
  if (!ttsSupported()) return () => {}
  const synth = window.speechSynthesis
  synth.addEventListener?.('voiceschanged', cb)
  return () => synth.removeEventListener?.('voiceschanged', cb)
}

// --- Server (Piper) engine --------------------------------------------------

export type TtsEngine = 'auto' | 'browser' | 'server'

export function ttsEngine(): TtsEngine {
  try {
    const v = localStorage.getItem(ENGINE_KEY)
    return v === 'browser' || v === 'server' ? v : 'auto'
  } catch {
    return 'auto'
  }
}

export function setTtsEngine(v: TtsEngine) {
  try {
    localStorage.setItem(ENGINE_KEY, v)
  } catch {
    // best-effort
  }
}

export function serverVoiceId(): string {
  try {
    return localStorage.getItem(SERVER_VOICE_KEY) || ''
  } catch {
    return ''
  }
}

export function setServerVoiceId(v: string) {
  try {
    localStorage.setItem(SERVER_VOICE_KEY, v)
  } catch {
    // best-effort
  }
}

// Cached server-engine status (Piper availability + voice list), loaded once at
// app boot via initServerTts and reused by the resolver + settings UI.
let serverState: TtsStatus = { available: false, voices: [] }

export function serverTtsStatus(): TtsStatus {
  return serverState
}

// initServerTts fetches the server TTS status once and caches it. Call at app
// boot so 'auto' can prefer Piper immediately; safe to call again to refresh.
export async function initServerTts(): Promise<TtsStatus> {
  try {
    serverState = await api.ttsStatus()
  } catch {
    serverState = { available: false, voices: [] }
  }
  return serverState
}

// resolveEngine decides which engine actually runs, honouring the pref and real
// availability: 'server' needs Piper installed; 'browser' needs speechSynthesis.
// Returns 'none' when neither is usable.
export function resolveEngine(): 'server' | 'browser' | 'none' {
  const pref = ttsEngine()
  const serverOk = serverState.available
  const browserOk = ttsSupported()
  if (pref === 'server') return serverOk ? 'server' : browserOk ? 'browser' : 'none'
  if (pref === 'browser') return browserOk ? 'browser' : serverOk ? 'server' : 'none'
  // auto
  return serverOk ? 'server' : browserOk ? 'browser' : 'none'
}

// ttsAvailable reports whether read-aloud can run at all (either engine).
export function ttsAvailable(): boolean {
  return resolveEngine() !== 'none'
}

// Shared audio element for server-synthesized playback + a one-time autoplay
// unlock so auto-read can play on mobile (which blocks audio without a gesture).
let audioEl: HTMLAudioElement | null = null
let currentURL: string | null = null

function getAudio(): HTMLAudioElement {
  if (!audioEl) audioEl = new Audio()
  return audioEl
}

// A 44-byte silent WAV used to "unlock" the audio element inside a user gesture.
const SILENT_WAV =
  'data:audio/wav;base64,UklGRiQAAABXQVZFZm10IBAAAAABAAEAgD4AAAB9AAACABAAZGF0YQAAAAA='

let unlockBound = false

// initTtsUnlock primes the shared audio element on the first user gesture so a
// later server-audio play() (e.g. auto-read) is allowed on mobile browsers.
export function initTtsUnlock() {
  if (unlockBound || typeof window === 'undefined') return
  unlockBound = true
  const unlock = () => {
    const a = getAudio()
    a.src = SILENT_WAV
    a.play().catch(() => {}).finally(() => {
      a.pause()
      a.currentTime = 0
    })
    window.removeEventListener('pointerdown', unlock)
    window.removeEventListener('keydown', unlock)
    window.removeEventListener('touchstart', unlock)
  }
  window.addEventListener('pointerdown', unlock)
  window.addEventListener('keydown', unlock)
  window.addEventListener('touchstart', unlock)
}

// speakServer synthesizes on the server (Piper) and plays the returned WAV.
// Resolves true on success; false lets the caller fall back to browser speech.
async function speakServer(clean: string, onEnd?: () => void): Promise<boolean> {
  try {
    const blob = await api.synthesizeTts(clean, serverVoiceId())
    const url = URL.createObjectURL(blob)
    const a = getAudio()
    a.pause()
    if (currentURL) URL.revokeObjectURL(currentURL)
    currentURL = url
    a.src = url
    a.playbackRate = ttsRate()
    a.onended = () => onEnd?.()
    a.onerror = () => onEnd?.()
    await a.play()
    return true
  } catch {
    return false
  }
}

// stripForSpeech reduces assistant markdown to spoken prose. Order matters:
// fenced code first (multi-line), then inline constructs, then leftover markers.
export function stripForSpeech(md: string): string {
  let t = md
  // Fenced code / preview blocks (``` or ~~~), including datatable/mermaid/html.
  t = t.replace(/```[\s\S]*?```/g, ' ')
  t = t.replace(/~~~[\s\S]*?~~~/g, ' ')
  // A dangling unclosed fence (streamed/truncated) → drop to end of text.
  t = t.replace(/```[\s\S]*$/g, ' ')
  // Inline code is part of the sentence (a name, a flag, a file) → KEEP the words,
  // drop only the backticks. Only fenced BLOCKS above are skipped as real code.
  t = t.replace(/`([^`]*)`/g, '$1')
  // Images → alt text; links → visible text.
  t = t.replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
  t = t.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
  // Markdown table rows (a line wrapped in pipes).
  t = t.replace(/^\s*\|.*\|\s*$/gm, ' ')
  // Line-leading markers: headings, blockquotes, list bullets/numbers.
  t = t.replace(/^\s{0,3}#{1,6}\s+/gm, '')
  t = t.replace(/^\s{0,3}>\s?/gm, '')
  t = t.replace(/^\s{0,3}[-*+]\s+/gm, '')
  t = t.replace(/^\s{0,3}\d+\.\s+/gm, '')
  // Emphasis / strikethrough markers.
  t = t.replace(/(\*\*|__|~~|\*|_)/g, '')
  // Bare URLs and Windows drive paths (unreadable when spoken).
  t = t.replace(/https?:\/\/\S+/g, ' ')
  t = t.replace(/[A-Za-z]:\\[^\s]+/g, ' ')
  // Collapse whitespace.
  return t.replace(/\s+/g, ' ').trim()
}

// pickVoice finds a voice whose language matches the requested tag by prefix
// (e.g. 'tr' matches 'tr-TR'), or null to let the engine choose.
function pickVoice(lang: string): SpeechSynthesisVoice | null {
  if (!lang) return null
  const voices = window.speechSynthesis.getVoices()
  const base = lang.split('-')[0].toLowerCase()
  return (
    voices.find((v) => v.lang.toLowerCase() === lang.toLowerCase()) ??
    voices.find((v) => v.lang.toLowerCase().startsWith(base)) ??
    null
  )
}

// resolveVoice honours an explicit voiceURI choice, else falls back to a
// language-matched voice (from the TTS/dictation language).
function resolveVoice(): SpeechSynthesisVoice | null {
  const uri = ttsVoiceURI()
  if (uri) {
    const hit = window.speechSynthesis.getVoices().find((v) => v.voiceURI === uri)
    if (hit) return hit
  }
  return pickVoice(ttsLang())
}

// speakBrowser reads text with the browser SpeechSynthesis engine (client voices).
function speakBrowser(clean: string, onEnd?: () => void) {
  if (!ttsSupported()) {
    onEnd?.()
    return
  }
  const synth = window.speechSynthesis
  synth.cancel()
  const u = new SpeechSynthesisUtterance(clean)
  u.rate = ttsRate()
  u.pitch = ttsPitch()
  const voice = resolveVoice()
  if (voice) {
    u.voice = voice
    u.lang = voice.lang
  } else {
    const lang = ttsLang()
    if (lang) u.lang = lang
  }
  if (onEnd) {
    u.onend = () => onEnd()
    u.onerror = () => onEnd()
  }
  synth.speak(u)
}

// speak reads the given text aloud (after stripping), cancelling any current
// playback first. Routes to the server (Piper) engine when selected/available —
// so a phone plays the server-generated audio — otherwise the browser engine.
// onEnd fires when playback finishes, is cancelled, or errors.
export function speak(text: string, onEnd?: () => void) {
  const clean = stripForSpeech(text)
  if (!clean) {
    onEnd?.()
    return
  }
  stopSpeaking()
  if (resolveEngine() === 'server') {
    void speakServer(clean, onEnd).then((ok) => {
      // Server unavailable/failed mid-call → fall back to browser speech.
      if (!ok) speakBrowser(clean, onEnd)
    })
    return
  }
  speakBrowser(clean, onEnd)
}

export function stopSpeaking() {
  if (ttsSupported()) window.speechSynthesis.cancel()
  if (audioEl) audioEl.pause()
}

// Dedupe guard so the same completed reply is auto-read only once even though a
// completion may reload the transcript more than once.
let lastSpokenId = ''

// speakLatestReply auto-reads the newest assistant reply in a freshly-loaded
// transcript, once. No-op unless auto-read is on. Interrupted/cancelled replies
// are skipped (partial content shouldn't be dictated).
export function speakLatestReply(messages: Message[]) {
  if (!ttsAutoRead() || !ttsAvailable()) return
  for (let i = messages.length - 1; i >= 0; i--) {
    const m = messages[i]
    if (m.role === 'user') continue
    if (m.id === lastSpokenId) return
    if (m.interrupted || m.cancelled) return
    lastSpokenId = m.id
    speak(m.text)
    return
  }
}
