import { useEffect, useRef, useState } from 'react'
import { Mic, Square, Loader2 } from 'lucide-react'
import { BTN_ICON } from './buttonStyles'
import { useSpeechToText } from './useSpeechToText'
import { useServerStt } from './useServerStt'
import { playMicStart, playMicStop } from '@/shared/lib/sounds'
import { initServerStt, resolveSttEngine } from '@/shared/lib/stt'
import { sttLang, onSttLangChange, sttLangLabel } from './sttLanguages'

interface Props {
  // Disabled while there is no active session (nothing to dictate into).
  disabled: boolean
  // Called with each finalized transcript chunk; the composer appends it to the
  // draft. Interim (not-yet-final) text is shown as a local preview only.
  onTranscript: (text: string) => void
}

// MicButton adds voice dictation to the composer with two engines: the browser
// Web Speech API (streaming, interim preview) or the server whisper.cpp engine
// (record → upload → transcribe on stop; works in WebView2 / thin clients). The
// engine is auto-selected (server when installed) and configurable in Settings ▸
// Ses. Language is chosen there too and surfaced only in the button tooltip.
export function MicButton({ disabled, onTranscript }: Props) {
  const [lang, setLang] = useState(sttLang)
  // Bumped once the server STT status resolves so the engine choice re-evaluates.
  const [, setReady] = useState(0)
  useEffect(() => {
    void initServerStt().then(() => setReady((n) => n + 1))
  }, [])

  // Both engines are wired; only one is driven, chosen per render by the resolver.
  const ws = useSpeechToText(onTranscript)
  const srv = useServerStt(onTranscript)

  const engine = resolveSttEngine()
  const useServer = engine === 'server'
  const listening = useServer ? srv.listening : ws.listening
  const transcribing = useServer ? srv.transcribing : false
  const interim = useServer ? '' : ws.interim
  const error = useServer ? srv.error : ws.error
  const start = (l: string) => (useServer ? srv.start(l) : ws.start(l))
  const stop = () => (useServer ? srv.stop() : ws.stop())

  // Follow language changes from Settings: update label; re-arm only the browser
  // engine mid-listen (re-arming the recorder would abort an in-progress clip).
  const listeningRef = useRef(listening)
  useEffect(() => {
    listeningRef.current = listening
  }, [listening])
  useEffect(
    () =>
      onSttLangChange((v) => {
        setLang(v)
        if (listeningRef.current && !useServer) ws.start(v)
      }),
    [ws, useServer],
  )

  // Blip on every listening transition (explicit toggle or self-stop). Skip mount.
  const prevListening = useRef(listening)
  useEffect(() => {
    if (prevListening.current !== listening) {
      if (listening) playMicStart()
      else playMicStop()
      prevListening.current = listening
    }
  }, [listening])

  if (engine === 'none') return null

  const toggle = () => {
    if (transcribing) return
    if (listening) stop()
    else start(lang)
  }

  const engineLabel = useServer ? 'sunucu' : 'tarayıcı'
  const title = error
    ? `Ses tanıma hatası: ${error}`
    : transcribing
      ? 'Yazıya çevriliyor…'
      : listening
        ? useServer
          ? `Kaydı bitir (${sttLangLabel(lang)} · sunucu)`
          : `Dinlemeyi durdur (${sttLangLabel(lang)})`
        : `Sesle yaz (${sttLangLabel(lang)} · ${engineLabel}) — dili Ayarlar'dan değiştir`

  return (
    <div className="relative flex items-center">
      {/* Live interim transcript (browser engine only), floating above the mic. */}
      {listening && interim && (
        <div className="absolute bottom-full left-0 mb-2 max-w-[16rem] truncate rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs text-[var(--color-text-dim)] shadow-lg">
          {interim}
        </div>
      )}

      <button
        type="button"
        onClick={toggle}
        disabled={disabled || transcribing}
        title={title}
        aria-label={listening ? 'Kaydı durdur' : 'Sesle yaz'}
        aria-pressed={listening}
        data-testid="composer-mic"
        className={`${BTN_ICON} ${
          listening ? 'animate-pulse border-[var(--color-danger)] text-[var(--color-danger)]' : ''
        } ${transcribing ? 'text-[var(--color-accent)]' : ''}`}
      >
        {transcribing ? (
          <Loader2 size={18} className="animate-spin" />
        ) : listening ? (
          <Square size={18} />
        ) : (
          <Mic size={18} />
        )}
      </button>
    </div>
  )
}
