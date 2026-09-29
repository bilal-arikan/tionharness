// Per-workspace category: identity (icon), stats, instructions,
// provider/model overrides, autonomy pause and the delete danger zone. The
// publish/export-as-template flow now lives in its own "Dışa Aktar" sub-tab.
import type { WorkspaceSettings } from '@/types'
import { Field, Toggle, inputCls, type WsSet } from './primitives'
import { CodexPluginsSection } from './CodexPluginsSection'
import { EmojiField } from '@/shared/components/EmojiField'
import { formatDate } from '@/shared/lib/intl'
import { WorkspaceDataFolder } from '@/features/workspace/WorkspaceDataFolder'
import { useTranslation } from 'react-i18next'

interface Props {
  ws: WorkspaceSettings
  setWsField: WsSet
  onDeleteWorkspace?: () => void
}

export function WorkspacePanel({ ws, setWsField, onDeleteWorkspace }: Props) {
  const { t } = useTranslation('settings')
  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        {t('workspace.introPrefix')}{' '}
        <span className="font-medium text-[var(--color-text)]">{ws.name}</span>{' '}
        {t('workspace.introSuffix')}
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {[
          { label: t('workspace.stats.agents'), value: ws.agentCount },
          { label: t('workspace.stats.sessions'), value: ws.sessionCount },
          { label: t('workspace.stats.tasks'), value: ws.taskCount },
          {
            label: t('workspace.stats.created'),
            value: formatDate(new Date(ws.createdAt * 1000), { dateStyle: 'short' }),
          },
        ].map((s) => (
          <div
            key={s.label}
            className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-2 text-center"
          >
            <div className="text-sm font-semibold">{s.value}</div>
            <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
              {s.label}
            </div>
          </div>
        ))}
      </div>

      <WorkspaceDataFolder path={ws.dataDir} />

      <div className="flex items-end gap-3">
        <Field label={t('workspace.name')}>
          <input
            value={ws.name}
            onChange={(e) => setWsField('name', e.target.value)}
            className={inputCls}
          />
        </Field>
        <Field label={t('workspace.icon')}>
          <EmojiField value={ws.icon} onChange={(e) => setWsField('icon', e)} clearLabel="⬡" />
        </Field>
      </div>

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.responseStyle')}
      </div>
      <Toggle
        label={t('workspace.terseLabel')}
        hint={t('workspace.terseHint')}
        checked={ws.terseMode}
        onChange={(v) => setWsField('terseMode', v)}
      />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.codebaseMemoryTitle')}
      </div>
      <Toggle
        label={t('workspace.codebaseMemoryLabel')}
        hint={t('workspace.codebaseMemoryHint')}
        checked={ws.codebaseMemoryEnabled}
        onChange={(v) => setWsField('codebaseMemoryEnabled', v)}
      />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.zvecTitle')}
      </div>
      <Toggle
        label={t('workspace.zvecLabel')}
        hint={t('workspace.zvecHint')}
        checked={ws.zvecGrepEnabled}
        onChange={(v) => setWsField('zvecGrepEnabled', v)}
      />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.codexPluginsTitle')}
      </div>
      <Toggle
        label={t('workspace.codexPluginsLabel')}
        hint={t('workspace.codexPluginsHint')}
        checked={ws.codexPluginsEnabled}
        onChange={(v) => setWsField('codexPluginsEnabled', v)}
      />
      {ws.codexPluginsEnabled && (
        <CodexPluginsSection
          marketplaces={ws.codexMarketplaces}
          plugins={ws.codexPlugins}
          onChangeMarketplaces={(v) => setWsField('codexMarketplaces', v)}
          onChangePlugins={(v) => setWsField('codexPlugins', v)}
        />
      )}

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.promptEpochTitle')}
      </div>
      <Toggle
        label={t('workspace.promptEpochLabel')}
        hint={t('workspace.promptEpochHint')}
        checked={ws.promptEpochEnabled}
        onChange={(v) => setWsField('promptEpochEnabled', v)}
      />

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.sqzTitle')}
      </div>
      <Field label={t('workspace.sqzLabel')} hint={t('workspace.sqzHint')}>
        <select
          value={ws.shellOutputCompression || ''}
          onChange={(e) =>
            setWsField('shellOutputCompression', e.target.value as '' | 'on' | 'off')
          }
          className={inputCls}
        >
          <option value="">{t('workspace.sqzAuto')}</option>
          <option value="on">{t('workspace.forceOn')}</option>
          <option value="off">{t('workspace.off')}</option>
        </select>
      </Field>

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('workspace.rtkTitle')}
      </div>
      <Field label={t('workspace.rtkLabel')} hint={t('workspace.rtkHint')}>
        <select
          value={ws.shellCommandRewrite || ''}
          onChange={(e) => setWsField('shellCommandRewrite', e.target.value as '' | 'on' | 'off')}
          className={inputCls}
        >
          <option value="">{t('workspace.rtkAuto')}</option>
          <option value="on">{t('workspace.forceOn')}</option>
          <option value="off">{t('workspace.off')}</option>
        </select>
      </Field>

      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('common.delete')}
      </div>

      {onDeleteWorkspace && (
        <div className="mt-2 flex items-center justify-between rounded-lg border border-[color-mix(in_srgb,var(--color-danger)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_6%,transparent)] px-3 py-2">
          <span className="text-xs text-[var(--color-text-dim)]">{t('workspace.deleteHint')}</span>
          <button
            onClick={onDeleteWorkspace}
            className="rounded border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
          >
            {t('workspace.delete')}
          </button>
        </div>
      )}
    </>
  )
}
