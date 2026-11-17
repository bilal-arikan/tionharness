import { useTranslation } from 'react-i18next'
import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ToolsPanel({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      {/* Individual tool visibility is managed in the workspace Tools screen. */}
      <Toggle
        label={t('tools.codeMode.label')}
        hint={t('tools.codeMode.hint')}
        checked={draft.enableCodeMode}
        onChange={(v) => set('enableCodeMode', v)}
      />
    </>
  )
}
