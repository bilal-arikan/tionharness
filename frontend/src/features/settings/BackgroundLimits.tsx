import { useTranslation } from 'react-i18next'
import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function BackgroundLimits({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">{t('background.description')}</p>
      <NumberField
        label={t('background.concurrent.label')}
        hint={t('background.concurrent.hint')}
        min={1}
        max={128}
        value={draft.spawnMaxConcurrent}
        onChange={(v) => set('spawnMaxConcurrent', v)}
      />
      <NumberField
        label={t('background.queued.label')}
        hint={t('background.queued.hint')}
        min={1}
        max={128}
        value={draft.spawnQueueMax}
        onChange={(v) => set('spawnQueueMax', v)}
      />
      <NumberField
        label={t('background.flowRetention.label')}
        hint={t('background.flowRetention.hint')}
        min={0}
        max={1000}
        value={draft.flowRunRetention}
        onChange={(v) => set('flowRunRetention', v)}
      />
      <NumberField
        label={t('background.perTurn.label')}
        hint={t('background.perTurn.hint')}
        min={1}
        max={64}
        value={draft.spawnMaxPerTurn}
        onChange={(v) => set('spawnMaxPerTurn', v)}
      />
      <NumberField
        label={t('background.idle.label')}
        hint={t('background.idle.hint')}
        min={1}
        max={1440}
        value={draft.spawnIdleTimeoutMin}
        onChange={(v) => set('spawnIdleTimeoutMin', v)}
      />
      <NumberField
        label={t('background.chatIdle.label')}
        hint={t('background.chatIdle.hint')}
        min={0}
        max={1440}
        value={draft.chatTurnIdleTimeoutMin}
        onChange={(v) => set('chatTurnIdleTimeoutMin', v)}
      />
      <NumberField
        label={t('background.codexIdle.label')}
        hint={t('background.codexIdle.hint')}
        min={0}
        max={86400}
        value={draft.codexStdoutIdleSec}
        onChange={(v) => set('codexStdoutIdleSec', v)}
      />
      <NumberField
        label={t('background.recovery.label')}
        hint={t('background.recovery.hint')}
        min={0}
        max={5}
        value={draft.idleResumeMax}
        onChange={(v) => set('idleResumeMax', v)}
      />
      <NumberField
        label={t('background.watchdog.label')}
        hint={t('background.watchdog.hint')}
        min={1}
        max={1440}
        value={draft.turnIdleWatchdogMin}
        onChange={(v) => set('turnIdleWatchdogMin', v)}
      />
    </>
  )
}
