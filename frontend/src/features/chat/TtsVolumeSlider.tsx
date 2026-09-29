import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ttsVolume, setTtsVolume, onTtsVolumeChange } from '@/shared/lib/tts'

// TtsVolumeSlider is a compact, GLOBAL read-aloud volume control. Every instance
// reflects the single shared pref (shared/lib/tts) and updates live when any other
// instance — or the Settings page — changes it, so all bubble sliders move together.
export function TtsVolumeSlider({ className = '' }: { className?: string }) {
  const { t } = useTranslation('chatStatus')
  const [vol, setVol] = useState(() => ttsVolume())
  // Keep this slider in sync when another slider (or Settings) changes the value.
  useEffect(() => onTtsVolumeChange(setVol), [])
  const change = (v: number) => {
    setTtsVolume(v) // persist + broadcast (also updates this via the event)
    setVol(v)
  }
  return (
    <input
      type="range"
      min={0}
      max={1}
      step={0.05}
      value={vol}
      onChange={(e) => change(Number(e.target.value))}
      title={t('tts.title', { percent: Math.round(vol * 100) })}
      aria-label={t('tts.label')}
      className={`h-1 w-16 cursor-pointer accent-[var(--color-accent)] ${className}`}
    />
  )
}
