import { useEffect, useState } from 'react'
import { Bell } from 'lucide-react'
import { api } from '@/api'
import { NOTIFY_TYPES, mutedTypes, setTypeEnabled } from '@/shared/lib/notifyPrefs'
import type { DesktopNotificationsMode } from '@/types/workspace'
import { Segmented, Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'

interface Props extends PanelProps {
  onError: (msg: string) => void
  // Re-resolve the effective notification gate live after the workspace override
  // is saved, so it applies without a workspace switch or a global reload.
  onWorkspaceNotifySaved?: (mode: DesktopNotificationsMode) => void
}

const WS_NOTIFY_OPTIONS: { value: DesktopNotificationsMode; label: string; hint?: string }[] = [
  { value: 'inherit', label: 'Genel ayarı kullan', hint: 'Uygulama genelindeki masaüstü bildirimleri ayarını izler.' },
  { value: 'on', label: 'Açık', hint: 'Bu workspace için bildirimler her zaman açık.' },
  { value: 'off', label: 'Kapalı', hint: 'Bu workspace için bildirimler her zaman kapalı.' },
]

// NotificationsPanel owns desktop-notification + screen behaviour only. Audio
// settings (sound effects, STT, TTS) live on the dedicated "Ses" page (SoundPanel).
//
// The master toggle (draft.desktopNotifications) is the app-global default; the
// per-workspace override below (three-state) lets a user force notifications on or
// off for the active workspace regardless of the global toggle, consistent with
// TionSwarm's physical workspace isolation. The override is self-contained (own
// load/save via the workspace-settings endpoint), like AppearancePanel.
export function NotificationsPanel({ draft, set, onError, onWorkspaceNotifySaved }: Props) {
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

  // Active workspace's three-state override. Loaded on mount and saved immediately
  // on change (own persistence, not the outer app-settings Save button).
  const [wsMode, setWsMode] = useState<DesktopNotificationsMode>('inherit')
  useEffect(() => {
    let cancelled = false
    api.getWorkspaceSettings()
      .then((w) => { if (!cancelled) setWsMode(w.desktopNotifications) })
      .catch(() => {})
    return () => { cancelled = true }
  }, [])
  const saveWsMode = async (mode: DesktopNotificationsMode) => {
    const prev = wsMode
    setWsMode(mode) // optimistic
    try {
      await api.updateWorkspaceSettings({ desktopNotifications: mode })
      onWorkspaceNotifySaved?.(mode)
    } catch (e) {
      setWsMode(prev) // revert on failure so the UI never lies about persisted state
      onError((e as Error).message)
    }
  }

  return (
    <>
      <Toggle label="Masaüstü bildirimleri" hint="Pencere arkadayken olay gerçekleşince tarayıcı bildirimi gösterir (izin ister). Bu, tüm workspace'ler için genel varsayılandır." checked={draft.desktopNotifications} onChange={(v) => set('desktopNotifications', v)} />
      <Segmented
        label="Bu workspace için bildirimler"
        value={wsMode}
        options={WS_NOTIFY_OPTIONS}
        onChange={saveWsMode}
      />
      <Toggle label="Ekranı açık tut" hint="Uygulama açıkken ekran uyku moduna geçmez (Wake Lock)." checked={draft.keepAwake} onChange={(v) => set('keepAwake', v)} />

      <SubHead icon={Bell}>Bildirim türleri</SubHead>
      <p className="-mt-1 text-xs text-[var(--color-text-dim)]">
        Bir türü kapatınca o olay için masaüstü bildirimi gösterilmez. Uygulamanın gönderebileceği <b>tüm</b> bildirim türleri burada listelenir. Bu ayarlar bu cihaza özeldir ve anında uygulanır. 🔊 işaretli türlerin ayrıca bir ses uyarısı vardır (ses, Ses ekranındaki "Ses efektleri" tercihine bağlıdır; bu türü kapatmak yalnız masaüstü bildirimini susturur).
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
