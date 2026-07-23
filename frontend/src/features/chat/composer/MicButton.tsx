import { useEffect, useRef, useState } from 'react'
import { Mic, Square } from 'lucide-react'
import { BTN_ICON } from './buttonStyles'
import { ComposerPicker } from './ComposerPicker'
import { useSpeechToText } from './useSpeechToText'
import { playMicStart, playMicStop } from '@/shared/lib/sounds'
import { STT_LANGUAGES, DEFAULT_STT_LANG, STT_LANG_STORAGE_KEY } from './sttLanguages'

interface Props {
  // Disabled while there is no active session (nothing to dictate into).
  disabled: boolean
  // Called with each finalized transcript chunk; the composer appends it to the
  // draft. Interim (not-yet-final) text is shown as a local preview only.
  onTranscript: (text: string) => void
}

// Read the persisted dictation language once, falling back to the default.
function initialLang(): string {
  const saved = localStorage.getItem(STT_LANG_STORAGE_KEY)
  return STT_LANGUAGES.some((l) => l.value === saved) ? saved! : DEFAULT_STT_LANG
}

// MicButton adds voice dictation to the composer: a language picker plus a mic
// toggle. Speaking appends recognized text to the input; a live interim preview
// floats above while listening. The whole cluster renders nothing when the
// browser lacks Web Speech recognition (e.g. most WebView2 desktop builds).
export function MicButton({ disabled, onTranscript }: Props) {
  const [lang, setLang] = useState(initialLang)
  const { supported, listening, interim, error, start, stop } = useSpeechToText(onTranscript)

  // Play a start/stop blip on every listening transition — this fires for an
  // explicit toggle AND when recognition ends on its own (e.g. 'no-speech'), so
  // the cue always matches the real mic state. The initial mount is skipped.
  const prevListening = useRef(listening)
  useEffect(() => {
    if (prevListening.current !== listening) {
      if (listening) playMicStart()
      else playMicStop()
      prevListening.current = listening
    }
  }, [listening])

  if (!supported) return null

  const toggle = () => {
    if (listening) stop()
    else start(lang)
  }

  const pickLang = (v: string) => {
    setLang(v)
    localStorage.setItem(STT_LANG_STORAGE_KEY, v)
    // Re-arm on the new language so a mid-session switch takes effect immediately.
    if (listening) start(v)
  }

  return (
    <div className="relative flex items-center gap-1.5">
      {/* Live interim transcript, floating just above the mic while speaking. */}
      {listening && interim && (
        <div className="absolute bottom-full left-0 mb-2 max-w-[16rem] truncate rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-xs text-[var(--color-text-dim)] shadow-lg">
          {interim}
        </div>
      )}

      <ComposerPicker
        value={lang}
        onChange={pickLang}
        options={STT_LANGUAGES}
        header="Ses dili"
        title={(c) => `Ses tanıma dili: ${c.label}`}
        iconOnly
      />

      <button
        type="button"
        onClick={toggle}
        disabled={disabled}
        title={
          error
            ? `Ses tanıma hatası: ${error}`
            : listening
              ? 'Dinlemeyi durdur'
              : 'Sesle yaz — konuş, metin girdiye eklenir'
        }
        aria-label={listening ? 'Dinlemeyi durdur' : 'Sesle yaz'}
        aria-pressed={listening}
        data-testid="composer-mic"
        className={`${BTN_ICON} ${
          listening ? 'animate-pulse border-[var(--color-danger)] text-[var(--color-danger)]' : ''
        }`}
      >
        {listening ? <Square size={18} /> : <Mic size={18} />}
      </button>
    </div>
  )
}
