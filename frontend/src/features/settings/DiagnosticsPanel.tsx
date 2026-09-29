import { useTranslation } from 'react-i18next'
import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function DiagnosticsPanel({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('shared.appliesAll')}</p>
      <Toggle
        label={t('diagnostics.record.label')}
        hint={t('diagnostics.record.hint')}
        checked={draft.debugJournalEnabled}
        onChange={(v) => set('debugJournalEnabled', v)}
      />
      <NumberField
        label={t('diagnostics.retain.label')}
        hint={t('diagnostics.retain.hint')}
        min={0}
        value={draft.debugJournalCap}
        onChange={(v) => set('debugJournalCap', v)}
      />
    </>
  )
}
