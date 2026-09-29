import { useTranslation } from 'react-i18next'
import { Toggle, Segmented } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function CliOptions({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <Toggle
        label={t('cli.hooks.label')}
        hint={t('cli.hooks.hint')}
        checked={draft.enableCliHooks}
        onChange={(v) => set('enableCliHooks', v)}
      />
      <Segmented
        label={t('cli.sessionMode.label')}
        value={draft.claudePersistentSession ? 'persistent' : draft.claudeResume ? 'resume' : 'off'}
        onChange={(mode) => {
          // Two mutually-exclusive booleans drive the runtime (toolloop.go picks
          // persistent when on; chat_resume.go gates --resume only when persistent is
          // off). Map each segment to a deterministic pair so exactly one path is live.
          set('claudePersistentSession', mode === 'persistent')
          set('claudeResume', mode === 'persistent' || mode === 'resume')
        }}
        options={[
          {
            value: 'persistent',
            label: t('cli.sessionMode.persistent.label'),
            hint: t('cli.sessionMode.persistent.hint'),
          },
          {
            value: 'resume',
            label: t('cli.sessionMode.resume.label'),
            hint: t('cli.sessionMode.resume.hint'),
          },
          {
            value: 'off',
            label: t('cli.sessionMode.off.label'),
            hint: t('cli.sessionMode.off.hint'),
          },
        ]}
      />
      <Toggle
        label={t('cli.promptFile.label')}
        hint={t('cli.promptFile.hint')}
        checked={draft.claudeSysPromptFile}
        onChange={(v) => set('claudeSysPromptFile', v)}
      />
      <Toggle
        label={t('cli.toolAllowlist.label')}
        hint={t('cli.toolAllowlist.hint')}
        checked={draft.claudeCliToolAllowlist}
        onChange={(v) => set('claudeCliToolAllowlist', v)}
      />
      <Toggle
        label={t('cli.researchAgents.label')}
        hint={t('cli.researchAgents.hint')}
        checked={draft.claudeCliNativeSubagents}
        onChange={(v) => set('claudeCliNativeSubagents', v)}
      />
      <Toggle
        label={t('cli.auxRouting.label')}
        hint={t('cli.auxRouting.hint')}
        checked={draft.auxNativeRouting}
        onChange={(v) => set('auxNativeRouting', v)}
      />
    </>
  )
}
