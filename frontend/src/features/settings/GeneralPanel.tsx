import { useTranslation } from 'react-i18next'
import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function GeneralPanel({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('shared.appliesAll')}</p>
      <Toggle
        label={t('general.keepAwake.label')}
        hint={t('general.keepAwake.hint')}
        checked={draft.keepAwake}
        onChange={(value) => set('keepAwake', value)}
      />
      <Toggle
        label={t('general.autoTitle.label')}
        hint={t('general.autoTitle.hint')}
        checked={draft.autoTitleEnabled}
        onChange={(value) => set('autoTitleEnabled', value)}
      />
      <Toggle
        label={t('general.autoTag.label')}
        hint={t('general.autoTag.hint')}
        checked={draft.autoTagSessions}
        onChange={(v) => set('autoTagSessions', v)}
      />
    </>
  )
}
