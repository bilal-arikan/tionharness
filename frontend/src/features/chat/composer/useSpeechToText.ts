import { useCallback, useEffect, useRef, useState } from 'react'

// Minimal Web Speech API typings — the DOM lib does not ship SpeechRecognition,
// and the constructor is often vendor-prefixed (webkit) and Chromium-only.
interface RecognitionAlternative {
  transcript: string
}
interface RecognitionResult {
  isFinal: boolean
  0: RecognitionAlternative
}
interface RecognitionEvent {
  resultIndex: number
  results: ArrayLike<RecognitionResult>
}
interface RecognitionErrorEvent {
  error: string
}
interface SpeechRecognitionLike {
  lang: string
  continuous: boolean
  interimResults: boolean
  start(): void
  stop(): void
  abort(): void
  onresult: ((e: RecognitionEvent) => void) | null
  onerror: ((e: RecognitionErrorEvent) => void) | null
  onend: (() => void) | null
}
type SpeechRecognitionCtor = new () => SpeechRecognitionLike

// Resolve the (possibly vendor-prefixed) constructor, or null when the browser
// has no Web Speech recognition (most WebView2 desktop builds, Firefox, …).
function getCtor(): SpeechRecognitionCtor | null {
  const w = window as unknown as {
    SpeechRecognition?: SpeechRecognitionCtor
    webkitSpeechRecognition?: SpeechRecognitionCtor
  }
  return w.SpeechRecognition ?? w.webkitSpeechRecognition ?? null
}

export interface SpeechToText {
  // false where the API is absent — callers hide the mic entirely.
  supported: boolean
  listening: boolean
  // Live, not-yet-final transcript for an inline preview while speaking.
  interim: string
  // Last recognition error code (e.g. 'not-allowed', 'no-speech'), or null.
  error: string | null
  start: (lang: string) => void
  stop: () => void
}

// useSpeechToText wraps the browser Web Speech API for dictation into the
// composer. Final transcript chunks are pushed to onFinal (the caller appends
// them to the draft); interim text is exposed live for a preview. Recognition is
// client-side and Chromium-backed; `supported` is false where the API is missing.
export function useSpeechToText(onFinal: (text: string) => void): SpeechToText {
  const [supported] = useState(() => getCtor() !== null)
  const [listening, setListening] = useState(false)
  const [interim, setInterim] = useState('')
  const [error, setError] = useState<string | null>(null)
  const recRef = useRef<SpeechRecognitionLike | null>(null)
  // Hold the latest onFinal so handlers never go stale without re-subscribing.
  const onFinalRef = useRef(onFinal)
  useEffect(() => {
    onFinalRef.current = onFinal
  }, [onFinal])

  const stop = useCallback(() => {
    recRef.current?.stop()
  }, [])

  const start = useCallback((lang: string) => {
    const Ctor = getCtor()
    if (!Ctor) return
    // Restarting: tear down any in-flight session first (no double onend).
    recRef.current?.abort()

    const rec = new Ctor()
    rec.lang = lang
    rec.continuous = true
    rec.interimResults = true
    rec.onresult = (e) => {
      let live = ''
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const r = e.results[i]
        const txt = r[0].transcript
        if (r.isFinal) {
          const clean = txt.trim()
          if (clean) onFinalRef.current(clean)
        } else {
          live += txt
        }
      }
      setInterim(live)
    }
    rec.onerror = (e) => {
      setError(e.error)
      setListening(false)
    }
    rec.onend = () => {
      setListening(false)
      setInterim('')
    }

    recRef.current = rec
    setError(null)
    setInterim('')
    rec.start()
    setListening(true)
  }, [])

  // Abort any live recognition when the composer unmounts.
  useEffect(() => () => recRef.current?.abort(), [])

  return { supported, listening, interim, error, start, stop }
}
