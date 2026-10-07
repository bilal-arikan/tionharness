import { useTranslation } from 'react-i18next'
import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'
import { SettingsDisclosureInfo } from './SettingsDisclosure'

export function ContextHandoff({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <SettingsDisclosureInfo text={t('contextHandoff.description')} />
      <Toggle
        label={t('contextHandoff.auto.label')}
        hint={t('contextHandoff.auto.hint')}
        checked={draft.handoffAuto}
        onChange={(v) => set('handoffAuto', v)}
      />
      <NumberField
        label={t('contextHandoff.maxChain.label')}
        hint={t('contextHandoff.maxChain.hint')}
        min={1}
        max={100}
        value={draft.handoffMaxChain || 20}
        onChange={(v) => set('handoffMaxChain', v)}
      />
      <Toggle
        label={t('contextHandoff.writeFile.label')}
        hint={t('contextHandoff.writeFile.hint')}
        checked={draft.handoffWriteFile}
        onChange={(v) => set('handoffWriteFile', v)}
      />
    </>
  )
}
