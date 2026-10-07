import { useTranslation } from 'react-i18next'
import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'
import { SettingsDisclosureInfo } from './SettingsDisclosure'

export function ContextRecovery({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <SettingsDisclosureInfo text={t('contextRecovery.description')} />
      <Toggle
        label={t('contextRecovery.compact.label')}
        hint={t('contextRecovery.compact.hint')}
        checked={draft.reactiveCompact}
        onChange={(v) => set('reactiveCompact', v)}
      />
      <NumberField
        label={t('contextRecovery.continue.label')}
        hint={t('contextRecovery.continue.hint')}
        min={0}
        value={draft.maxTokenRetries}
        onChange={(v) => set('maxTokenRetries', v)}
      />
      <NumberField
        label={t('contextRecovery.recent.label')}
        hint={t('contextRecovery.recent.hint')}
        min={2}
        max={50}
        value={draft.reactiveKeepRecent}
        onChange={(v) => set('reactiveKeepRecent', v)}
      />
      <NumberField
        label={t('contextRecovery.outputLimit.label')}
        hint={t('contextRecovery.outputLimit.hint')}
        min={0}
        value={draft.maxOutputTokens}
        onChange={(v) => set('maxOutputTokens', v)}
      />
      <NumberField
        label={t('contextRecovery.retries.label')}
        hint={t('contextRecovery.retries.hint')}
        min={0}
        max={5}
        value={draft.maxProviderRetries}
        onChange={(v) => set('maxProviderRetries', v)}
      />
    </>
  )
}
