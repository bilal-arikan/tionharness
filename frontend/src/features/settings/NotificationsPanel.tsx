import { useState } from 'react'
import { Bell } from 'lucide-react'
import { NOTIFY_TYPES, mutedTypes, setTypeEnabled } from '@/shared/lib/notifyPrefs'
import { Toggle } from './primitives'
import { SubHead } from './settingsPanelShared'
import { useTranslation } from 'react-i18next'

// Both scopes apply immediately. A failed server request leaves the saved
// global value intact; device preferences retain their existing local storage.
export function NotificationsPanel({
  enabled,
  saving,
  onChange,
}: {
  enabled: boolean
  saving: boolean
  onChange: (enabled: boolean) => void
}) {
  const { t } = useTranslation('settings')
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
    <section className="flex flex-col gap-4">
      <SubHead icon={Bell}>{t('notifications.title')}</SubHead>
      <p className="text-xs text-[var(--color-text-dim)]">{t('notifications.scope')}</p>
      <fieldset disabled={saving} className="min-w-0">
        <Toggle
          label={t('notifications.masterLabel')}
          checked={enabled}
          onChange={onChange}
          hint={t('notifications.masterHint')}
        />
      </fieldset>
      {saving && (
        <p role="status" className="text-xs">
          {t('common.saving')}
        </p>
      )}
      <SubHead icon={Bell}>{t('notifications.typesTitle')}</SubHead>
      <p className="text-xs text-[var(--color-text-dim)]">{t('notifications.typesHint')}</p>
      {NOTIFY_TYPES.map((type) => (
        <Toggle
          key={type.type}
          label={`${t(`notifications.types.${type.type}.label`)}${type.cue ? ' 🔊' : ''}`}
          hint={t(`notifications.types.${type.type}.hint`)}
          checked={!muted.has(type.type)}
          onChange={(on) => toggleType(type.type, on)}
        />
      ))}
    </section>
  )
}
