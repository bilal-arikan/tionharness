import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'
import { useTranslation } from 'react-i18next'

export function ToolExecutionLimits({ draft, set }: PanelProps) {
  const { t } = useTranslation('settings')
  return (
    <>
      <NumberField
        label={t('toolLimits.defaultTimeout')}
        hint={t('toolLimits.defaultTimeoutHint')}
        min={1}
        max={3600}
        value={draft.shellDefaultTimeoutSec}
        onChange={(v) => set('shellDefaultTimeoutSec', v)}
      />
      <NumberField
        label={t('toolLimits.maxTimeout')}
        hint={t('toolLimits.maxTimeoutHint')}
        min={1}
        max={3600}
        value={draft.shellMaxTimeoutSec}
        onChange={(v) => set('shellMaxTimeoutSec', v)}
      />
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
