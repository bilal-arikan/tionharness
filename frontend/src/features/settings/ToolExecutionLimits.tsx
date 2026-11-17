import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'
import { useTranslation } from 'react-i18next'

export function ToolExecutionLimits({ draft, set }: PanelProps) {
  const { t } = useTranslation('settings')
  return (
    <>
      <NumberField
        label={t('toolLimits.outputLimit')}
        hint={t('toolLimits.outputLimitHint')}
        min={1}
        max={4096}
        value={draft.maxToolOutputKB}
        onChange={(v) => set('maxToolOutputKB', v)}
      />
    </>
  )
}
