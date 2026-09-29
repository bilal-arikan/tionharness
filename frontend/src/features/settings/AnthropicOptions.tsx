import { useTranslation } from 'react-i18next'
import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function AnthropicOptions({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('anthropic.description')}</p>
      <Toggle
        label={t('anthropic.extendedCache.label')}
        hint={t('anthropic.extendedCache.hint')}
        checked={draft.extendedPromptCache}
        onChange={(v) => set('extendedPromptCache', v)}
      />
      <Toggle
        label={t('anthropic.contextEditing.label')}
        hint={t('anthropic.contextEditing.hint')}
        checked={draft.anthropicContextEditing}
        onChange={(v) => set('anthropicContextEditing', v)}
      />
      <Toggle
        label={t('anthropic.toolSearch.label')}
        hint={t('anthropic.toolSearch.hint')}
        checked={draft.anthropicNativeToolSearch}
        onChange={(v) => set('anthropicNativeToolSearch', v)}
      />
      <Toggle
        label={t('anthropic.programmaticTools.label')}
        hint={t('anthropic.programmaticTools.hint')}
        checked={draft.anthropicProgrammaticTools}
        onChange={(v) => set('anthropicProgrammaticTools', v)}
      />
      <Toggle
        label={t('anthropic.webTools.label')}
        hint={t('anthropic.webTools.hint')}
        checked={draft.anthropicWebTools}
        onChange={(v) => set('anthropicWebTools', v)}
      />
      <Toggle
        label={t('anthropic.compaction.label')}
        hint={t('anthropic.compaction.hint')}
        checked={draft.anthropicServerCompaction}
        onChange={(v) => set('anthropicServerCompaction', v)}
      />
      <Toggle
        label={t('anthropic.refusalFallback.label')}
        hint={t('anthropic.refusalFallback.hint')}
        checked={draft.anthropicRefusalFallback}
        onChange={(v) => set('anthropicRefusalFallback', v)}
      />
      <NumberField
        label={t('anthropic.taskBudget.label')}
        hint={t('anthropic.taskBudget.hint')}
        min={0}
        step={1000}
        value={draft.autonomousTaskBudgetTokens}
        onChange={(v) => set('autonomousTaskBudgetTokens', v)}
      />
    </>
  )
}
