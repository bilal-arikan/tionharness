import { AnthropicOptions } from './AnthropicOptions'
import { CliOptions } from './CliOptions'
import { SettingsDisclosure } from './SettingsDisclosure'
import { SettingsSaveBar, type SettingsSaveBarProps } from './SettingsSaveBar'
import type { PanelProps } from './settingsPanelShared'

export function ProviderOptionsPanel({
  save,
  ...props
}: PanelProps & { save: SettingsSaveBarProps }) {
  return (
    <section className="space-y-4 border-t border-[var(--color-border)] pt-4">
      <h3 className="text-sm font-semibold">Advanced provider options</h3>
      <p className="text-xs text-[var(--color-text-dim)]">
        These options apply to all workspaces using the relevant provider. Save changes here;
        provider connections above are saved separately by their forms.
      </p>
      <SettingsDisclosure title="Anthropic API">
        <AnthropicOptions {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title="CLI sessions and auxiliary calls">
        <CliOptions {...props} />
      </SettingsDisclosure>
      <SettingsSaveBar {...save} />
    </section>
  )
}
