import { useState } from 'react'
import { Volume2, Mic } from 'lucide-react'
import { soundEffectsEnabled, setSoundEffectsEnabled, playTurnDone } from '@/shared/lib/sounds'
import { InfoPopover } from '@/shared/components/InfoPopover'
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

  // Both sections below are device-local; the note sits behind each heading's (ⓘ).
  const deviceNote = (
    <>
      {t('sound.deviceOnlyPrefix')}{' '}
      <span className="font-medium text-[var(--color-text)]">{t('sound.deviceOnly')}</span>{' '}
      {t('sound.deviceOnlySuffix')}
    </>
  )

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <SubHead icon={Volume2}>
          {t('sound.effectsTitle')}
          <InfoPopover text={deviceNote} />
        </SubHead>
        <Toggle
          label={t('sound.effectsLabel')}
          hint={t('sound.effectsHint')}
          checked={sounds}
          onChange={toggleSounds}
        />
      </div>

      <div className="flex flex-col gap-2">
        <SubHead icon={Mic}>
          {t('sound.sttTitle')}
          <InfoPopover text={deviceNote} />
        </SubHead>
        <SttSettings />
      </div>
    </div>
  )
}
