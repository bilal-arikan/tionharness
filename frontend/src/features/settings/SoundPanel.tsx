import { useState } from 'react'
import { Volume2, Mic, Speaker } from 'lucide-react'
import { soundEffectsEnabled, setSoundEffectsEnabled, playTurnDone } from '@/shared/lib/sounds'
import { ttsAutoRead, setTtsAutoRead, ttsAvailable, speak } from '@/shared/lib/tts'
import { Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'
import { SttSettings } from './SttSettings'
import { TtsSettings } from './TtsSettings'

// SoundPanel is the dedicated "Ses" settings sub-page collecting every audio
// preference: UI sound effects, speech INPUT (STT — mic dictation language) and
// speech OUTPUT (TTS — read-aloud engine/voice). All prefs here are DEVICE-LOCAL
// (localStorage in sounds.ts / tts.ts), so they apply instantly and are exempt
// from the global Save bar.
export function SoundPanel() {
  const [sounds, setSounds] = useState<boolean>(() => soundEffectsEnabled())
  const toggleSounds = (on: boolean) => {
    setSoundEffectsEnabled(on)
    setSounds(on)
    if (on) playTurnDone()
  }
  const [autoRead, setAutoRead] = useState<boolean>(() => ttsAutoRead())
  const toggleAutoRead = (on: boolean) => {
    setTtsAutoRead(on)
    setAutoRead(on)
    if (on) speak('Sesli okuma açık.')
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        Bu sayfadaki tüm ses ayarları <span className="font-medium text-[var(--color-text)]">bu cihaza özeldir</span> ve anında uygulanır (Kaydet gerekmez).
      </p>

      <div className="flex flex-col gap-2">
        <SubHead icon={Volume2}>Ses efektleri</SubHead>
        <Toggle
          label="Ses efektleri"
          hint="Ajan yanıtı bitince çalınan bitiş sesi ile mikrofon başlat/durdur seslerini açar/kapatır."
          checked={sounds}
          onChange={toggleSounds}
        />
      </div>

      <div className="flex flex-col gap-2">
        <SubHead icon={Mic}>Sesli giriş (STT)</SubHead>
        <SttSettings />
      </div>

      {ttsAvailable() && (
        <div className="flex flex-col gap-2">
          <SubHead icon={Speaker}>Sesli okuma (TTS)</SubHead>
          <Toggle
            label="Yanıtları sesli oku"
            hint="Ajan yanıtı tamamlanınca metni sesli okur (kod blokları, tablolar ve bağlantılar atlanır)."
            checked={autoRead}
            onChange={toggleAutoRead}
          />
          {/* Engine (browser / server Piper) + voice / rate / pitch — apply to both
              auto-read and the per-message 🔊 button. */}
          <TtsSettings />
        </div>
      )}
    </div>
  )
}
