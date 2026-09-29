import { useTranslation } from 'react-i18next'
import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function DelegationLimits({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('delegation.description')}</p>
      <NumberField
        label={t('delegation.depth.label')}
        hint={t('delegation.depth.hint')}
        min={1}
        max={10}
        value={draft.delegationMaxDepth}
        onChange={(v) => set('delegationMaxDepth', v)}
      />
      <NumberField
        label={t('delegation.perTurn.label')}
        hint={t('delegation.perTurn.hint')}
        min={1}
        max={100}
        value={draft.delegationMaxCalls}
        onChange={(v) => set('delegationMaxCalls', v)}
      />
    </>
  )
}
