import { useTranslation } from 'react-i18next'
import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ToolsPanel({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <Toggle
        label={t('tools.shell.label')}
        hint={t('tools.shell.hint')}
        checked={draft.enableShell}
        onChange={(v) => set('enableShell', v)}
      />
      {/* Individual tool visibility is managed in the workspace Tools screen. */}
      <Toggle
        label={t('tools.codeMode.label')}
        hint={t('tools.codeMode.hint')}
        checked={draft.enableCodeMode}
        onChange={(v) => set('enableCodeMode', v)}
      />
      {draft.enableCodeMode && !draft.enableShell && (
        <div className="rounded-lg border border-[var(--color-warning)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
          <b>{t('tools.codeMode.warningTitle')}</b> {t('tools.codeMode.warningBody')}
        </div>
      )}
    </>
  )
}
