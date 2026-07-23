import { useState } from 'react'
import { Bell, Volume2 } from 'lucide-react'
import { NOTIFY_TYPES, mutedTypes, setTypeEnabled } from '@/shared/lib/notifyPrefs'
import { soundEffectsEnabled, setSoundEffectsEnabled, playTurnDone } from '@/shared/lib/sounds'
import { Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

export function NotificationsPanel({ draft, set }: PanelProps) {
  // Per-type toast preferences are device-local (localStorage), so they apply
  // instantly — independent of the backend-persisted master toggle / Save.
  const [muted, setMuted] = useState<Set<string>>(() => mutedTypes())
  // Sound effects are also device-local; toggling on plays a sample chime.
  const [sounds, setSounds] = useState<boolean>(() => soundEffectsEnabled())
  const toggleSounds = (on: boolean) => {
    setSoundEffectsEnabled(on)
    setSounds(on)
    if (on) playTurnDone()
  }
  const toggleType = (type: string, on: boolean) => {
    setTypeEnabled(type, on)
    setMuted((prev) => {
      const next = new Set(prev)
      if (on) next.delete(type)
      else next.add(type)
      return next
    })
  }
  return (
    <>
      <Toggle label="Masaüstü bildirimleri" hint="Pencere arkadayken olay gerçekleşince tarayıcı bildirimi gösterir (izin ister)." checked={draft.desktopNotifications} onChange={(v) => set('desktopNotifications', v)} />
      <Toggle label="Ekranı açık tut" hint="Uygulama açıkken ekran uyku moduna geçmez (Wake Lock)." checked={draft.keepAwake} onChange={(v) => set('keepAwake', v)} />

      <SubHead icon={Bell}>Bildirim türleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Bir türü kapatınca o olay için masaüstü bildirimi gösterilmez. Bu ayarlar bu cihaza özeldir ve anında uygulanır.
      </p>
      {NOTIFY_TYPES.map((t) => (
        <Toggle key={t.type} label={t.label} hint={t.hint} checked={!muted.has(t.type)} onChange={(v) => toggleType(t.type, v)} />
      ))}

      <SubHead icon={Volume2}>Sesler</SubHead>
      <Toggle
        label="Ses efektleri"
        hint="Ajan yanıtı bitince çalınan bitiş sesi ile mikrofon başlat/durdur seslerini açar/kapatır. Bu cihaza özeldir."
        checked={sounds}
        onChange={toggleSounds}
      />
    </>
  )
}
