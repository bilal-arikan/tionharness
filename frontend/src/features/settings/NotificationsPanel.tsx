import { useState } from 'react'
import { Bell } from 'lucide-react'
import { NOTIFY_TYPES, mutedTypes, setTypeEnabled } from '@/shared/lib/notifyPrefs'
import { Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

// The master toggle (draft.desktopNotifications) is the single desktop
// notification switch; the per-type list below is device-local.
export function NotificationsPanel({ draft, set }: PanelProps) {
  // Per-type toast preferences are device-local (localStorage), so they apply
  // instantly — independent of the backend-persisted master toggle / Save.
  const [muted, setMuted] = useState<Set<string>>(() => mutedTypes())
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
      <Toggle
        label="Masaüstü bildirimleri"
        hint="Pencere arkadayken olay gerçekleşince tarayıcı bildirimi gösterir (izin ister). Tüm workspace'ler için geçerlidir."
        checked={draft.desktopNotifications}
        onChange={(v) => set('desktopNotifications', v)}
      />
      <Toggle
        label="Ekranı açık tut"
        hint="Uygulama açıkken ekran uyku moduna geçmez (Wake Lock)."
        checked={draft.keepAwake}
        onChange={(v) => set('keepAwake', v)}
      />

      <SubHead icon={Bell}>Bildirim türleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Bir türü kapatınca o olay için masaüstü bildirimi gösterilmez. Uygulamanın gönderebileceği{' '}
        <b>tüm</b> bildirim türleri burada listelenir. Bu ayarlar bu cihaza özeldir ve anında
        uygulanır. 🔊 işaretli türlerin ayrıca bir ses uyarısı vardır (ses, Ses ekranındaki "Ses
        efektleri" tercihine bağlıdır; bu türü kapatmak yalnız masaüstü bildirimini susturur).
      </p>
      {NOTIFY_TYPES.map((t) => (
        <Toggle
          key={t.type}
          label={t.cue ? `${t.label} 🔊` : t.label}
          hint={t.hint}
          checked={!muted.has(t.type)}
          onChange={(v) => toggleType(t.type, v)}
        />
      ))}
    </>
  )
}
