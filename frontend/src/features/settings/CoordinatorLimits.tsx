import { useTranslation } from 'react-i18next'
import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function CoordinatorLimits({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('coordinatorLimits.description')}</p>
      <NumberField
        label={t('coordinatorLimits.workers.label')}
        hint={t('coordinatorLimits.workers.hint')}
        min={1}
        max={64}
        value={draft.coordinatorMaxWorkers}
        onChange={(v) => set('coordinatorMaxWorkers', v)}
      />
      <NumberField
        label={t('coordinatorLimits.depth.label')}
        hint={t('coordinatorLimits.depth.hint')}
        min={-1}
        max={12}
        value={draft.coordinatorMaxDepth}
        onChange={(v) => set('coordinatorMaxDepth', v)}
      />
      <NumberField
        label={t('coordinatorLimits.grace.label')}
        hint={t('coordinatorLimits.grace.hint')}
        min={5}
        max={1800}
        value={draft.coordinatorSettleGraceSec}
        onChange={(v) => set('coordinatorSettleGraceSec', v)}
      />
    </>
  )
}
