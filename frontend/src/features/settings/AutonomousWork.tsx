import { useTranslation } from 'react-i18next'
import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function AutonomousWork({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('autonomous.description')}</p>
      <Toggle
        label={t('autonomous.confine.label')}
        hint={t('autonomous.confine.hint')}
        checked={draft.autonomousConfine}
        onChange={(v) => set('autonomousConfine', v)}
      />
      <Toggle
        label={t('autonomous.checklist.label')}
        hint={t('autonomous.checklist.hint')}
        checked={draft.autonomousBootSeq}
        onChange={(v) => set('autonomousBootSeq', v)}
      />
      <Toggle
        label={t('autonomous.continue.label')}
        hint={t('autonomous.continue.hint')}
        checked={draft.autonomousAutoContinue}
        onChange={(v) => set('autonomousAutoContinue', v)}
      />
      {draft.autonomousAutoContinue && (
        <NumberField
          label={t('autonomous.maxTurns.label')}
          hint={t('autonomous.maxTurns.hint')}
          min={0}
          value={draft.autonomousAutoContinueMax}
          onChange={(v) => set('autonomousAutoContinueMax', v)}
        />
      )}
    </>
  )
}
