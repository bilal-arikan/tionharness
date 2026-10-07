import { InfoPopover } from '@/shared/components/InfoPopover'
import { AnthropicOptions } from './AnthropicOptions'
import { CliOptions } from './CliOptions'
import { SettingsDisclosure } from './SettingsDisclosure'
import { SettingsSaveBar, type SettingsSaveBarProps } from './SettingsSaveBar'
import type { PanelProps } from './settingsPanelShared'
import { useTranslation } from 'react-i18next'

export function ProviderOptionsPanel({
  save,
  ...props
}: PanelProps & { save: SettingsSaveBarProps }) {
  const { t } = useTranslation('settings')
  return (
    <section className="space-y-4 border-t border-[var(--color-border)] pt-4">
      <h3 className="flex items-center gap-1 text-sm font-semibold">
        {t('providerOptions.title')}
        <InfoPopover text={t('providerOptions.description')} />
      </h3>
      <SettingsDisclosure title={t('providerOptions.anthropic')}>
        <AnthropicOptions {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('providerOptions.cli')}>
        <CliOptions {...props} />
      </SettingsDisclosure>
      <SettingsSaveBar {...save} />
    </section>
  )
}
