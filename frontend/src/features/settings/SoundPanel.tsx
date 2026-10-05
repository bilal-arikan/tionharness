import { useState } from 'react'
import { Volume2, Mic } from 'lucide-react'
import { soundEffectsEnabled, setSoundEffectsEnabled, playTurnDone } from '@/shared/lib/sounds'
import { Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'
import { SttSettings } from './SttSettings'
import { useTranslation } from 'react-i18next'

// SoundPanel is the dedicated "Ses" settings sub-page collecting every audio
// preference: UI sound effects and speech INPUT (STT — mic dictation language).
// All prefs here are DEVICE-LOCAL (localStorage in sounds.ts / stt.ts), so they apply instantly and are exempt
// from the global Save bar.
export function SoundPanel() {
  const { t } = useTranslation('settings')
  const [sounds, setSounds] = useState<boolean>(() => soundEffectsEnabled())
  const toggleSounds = (on: boolean) => {
    setSoundEffectsEnabled(on)
    setSounds(on)
    if (on) playTurnDone()
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        {t('sound.deviceOnlyPrefix')}{' '}
        <span className="font-medium text-[var(--color-text)]">{t('sound.deviceOnly')}</span>{' '}
        {t('sound.deviceOnlySuffix')}
      </p>

      <div className="flex flex-col gap-2">
        <SubHead icon={Volume2}>{t('sound.effectsTitle')}</SubHead>
        <Toggle
          label={t('sound.effectsLabel')}
          hint={t('sound.effectsHint')}
          checked={sounds}
          onChange={toggleSounds}
        />
      </div>

      <div className="flex flex-col gap-2">
        <SubHead icon={Mic}>{t('sound.sttTitle')}</SubHead>
        <SttSettings />
      </div>
    </div>
  )
}
