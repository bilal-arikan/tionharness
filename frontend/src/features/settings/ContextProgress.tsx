import { useTranslation } from 'react-i18next'
import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ContextProgress({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <Toggle
        label={t('contextProgress.save.label')}
        hint={t('contextProgress.save.hint')}
        checked={draft.progressPersist}
        onChange={(v) => set('progressPersist', v)}
      />
      <Toggle
        label={t('contextProgress.restore.label')}
        hint={t('contextProgress.restore.hint')}
        checked={draft.progressResume}
        onChange={(v) => set('progressResume', v)}
      />
    </>
  )
}
