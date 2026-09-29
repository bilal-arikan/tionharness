import { useTranslation } from 'react-i18next'
import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function CoordinatorRecovery({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <Toggle
        label={t('coordinatorRecovery.detect.label')}
        hint={t('coordinatorRecovery.detect.hint')}
        checked={draft.coordinatorStallGuard}
        onChange={(v) => set('coordinatorStallGuard', v)}
      />
      <NumberField
        label={t('coordinatorRecovery.window.label')}
        hint={t('coordinatorRecovery.window.hint')}
        min={-1}
        max={1440}
        value={draft.coordinatorStallSweepMin}
        onChange={(v) => set('coordinatorStallSweepMin', v)}
      />
      <NumberField
        label={t('coordinatorRecovery.nudges.label')}
        hint={t('coordinatorRecovery.nudges.hint')}
        min={0}
        max={10}
        value={draft.coordinatorStallMaxNudges}
        onChange={(v) => set('coordinatorStallMaxNudges', v)}
      />
      <Toggle
        label={t('coordinatorRecovery.notes.label')}
        hint={t('coordinatorRecovery.notes.hint')}
        checked={draft.coordinatorStallNoteVisible}
        onChange={(v) => set('coordinatorStallNoteVisible', v)}
      />
    </>
  )
}
