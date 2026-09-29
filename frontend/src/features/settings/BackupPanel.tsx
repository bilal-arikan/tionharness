import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { Archive, RotateCcw, Trash2 } from 'lucide-react'
import type { BackupStatus, WorkspaceArchives } from '@/types'
import { api, getActiveWorkspace } from '@/api'
import { Field, NumberField, Toggle, inputCls } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'
import { formatDateTime } from '@/shared/lib/intl'
import { formatBytes } from '@/shared/lib/format'

export function BackupPanel({ draft, set }: PanelProps) {
  const { t } = useTranslation('settingsMain')
  const [status, setStatus] = useState<BackupStatus | null>(null)
  const [archives, setArchives] = useState<WorkspaceArchives[]>([])
  const [running, setRunning] = useState(false)
  const [msg, setMsg] = useState<string | null>(null)
  // archive name awaiting a second-click restore confirm (one at a time).
  const [confirmArchive, setConfirmArchive] = useState<string | null>(null)
  // archive name currently being restored (disables its row).
  const [restoring, setRestoring] = useState<string | null>(null)
  // archive name awaiting a second-click delete confirm.
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null)
  // archive name currently being deleted.
  const [deleting, setDeleting] = useState<string | null>(null)

  const refresh = () => {
    api
      .getBackupStatus()
      .then(setStatus)
      .catch(() => {})
    api
      .listBackupArchives()
      .then(setArchives)
      .catch(() => {})
  }
  useEffect(() => {
    refresh()
  }, [])

  const runNow = async () => {
    setRunning(true)
    setMsg(null)
    try {
      const res = await api.runBackup()
      const fails = res.failures ? Object.keys(res.failures).length : 0
      setMsg(t('backup.runSuccess', { count: res.archives.length, failures: fails, dir: res.dir }))
      refresh()
    } catch (e) {
      setMsg(t('backup.runError', { error: (e as Error).message }))
    } finally {
      setRunning(false)
    }
  }

  const restore = async (workspaceId: string, archive: string) => {
    setRestoring(archive)
    setConfirmArchive(null)
    setMsg(null)
    try {
      await api.restoreBackup(workspaceId, archive)
      // Only the workspace currently open in THIS window has stale in-memory
      // state (lists, open chat). If we just restored it, reload to resync;
      // restoring any other workspace leaves this view correct (it reloads fresh
      // when the user switches to it), so we don't disrupt them with a reload.
      if (getActiveWorkspace() === workspaceId) {
        setMsg(t('backup.restoreActiveSuccess', { archive }))
        setTimeout(() => window.location.reload(), 1200)
        return // keep the row disabled until the reload lands
      }
      setMsg(t('backup.restoreSuccess', { archive }))
      refresh()
    } catch (e) {
      setMsg(t('backup.restoreError', { error: (e as Error).message }))
    } finally {
      setRestoring(null)
    }
  }

  const del = async (workspaceId: string, archive: string) => {
    setDeleting(archive)
    setConfirmDelete(null)
    setMsg(null)
    try {
      await api.deleteBackupArchive(workspaceId, archive)
      setMsg(t('backup.deleteSuccess', { archive }))
      refresh()
    } catch (e) {
      setMsg(t('backup.deleteError', { error: (e as Error).message }))
    } finally {
      setDeleting(null)
    }
  }

  const lastRunLabel = status?.lastRun
    ? formatDateTime(new Date(status.lastRun * 1000), { dateStyle: 'short', timeStyle: 'medium' })
    : t('backup.never')

  const totalArchives = archives.reduce((n, w) => n + w.archives.length, 0)

  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        <span className="font-medium text-[var(--color-text)]">{t('backup.scope.title')}</span>{' '}
        {t('backup.scope.body')}
      </div>
      <Toggle
        label={t('backup.auto.label')}
        hint={t('backup.auto.hint')}
        checked={draft.backupEnabled}
        onChange={(v) => set('backupEnabled', v)}
      />
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <NumberField
          label={t('backup.interval.label')}
          hint={t('backup.interval.hint')}
          min={1}
          value={draft.backupIntervalHours}
          onChange={(v) => set('backupIntervalHours', v)}
        />
        <NumberField
          label={t('backup.retain.label')}
          hint={t('backup.retain.hint')}
          min={1}
          value={draft.backupRetain}
          onChange={(v) => set('backupRetain', v)}
        />
      </div>
      <Field label={t('backup.folder.label')} hint={t('backup.folder.hint')}>
        <input
          value={draft.backupDir}
          onChange={(e) => set('backupDir', e.target.value)}
          placeholder={t('backup.folder.placeholder')}
          className={inputCls}
        />
      </Field>

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs">
        <span className="text-[var(--color-text-dim)]">{t('backup.status.label')}</span>
        <span
          className={`rounded px-1.5 py-0.5 font-medium ${status?.enabled ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'}`}
        >
          {status?.enabled ? t('shared.on') : t('shared.off')}
        </span>
        <span className="text-[var(--color-text-dim)]">
          {t('backup.status.lastBackup')}:{' '}
          <span className="text-[var(--color-text)]">{lastRunLabel}</span>
        </span>
        {status?.dir && (
          <span className="text-[var(--color-text-dim)]">
            {t('backup.status.folder')}:{' '}
            <code className="text-[var(--color-text)]">{status.dir}</code>
          </span>
        )}
        {status?.lastError && (
          <span className="text-[var(--color-danger)]">
            {t('backup.status.lastError')}: {status.lastError}
          </span>
        )}
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={runNow}
          disabled={running}
          className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-40"
        >
          {running ? t('backup.running') : t('backup.runNow')}
        </button>
        {msg && <span className="text-xs text-[var(--color-text-dim)]">{msg}</span>}
      </div>
      <p className="text-xs text-[var(--color-text-dim)]">{t('backup.note')}</p>

      {/* Archive list + one-click restore */}
      <SubHead icon={Archive}>{t('backup.archives.title', { count: totalArchives })}</SubHead>
      <div className="rounded-lg border border-[color-mix(in_srgb,var(--color-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_6%,transparent)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        ⚠ {t('backup.archives.warning')}
      </div>
      {totalArchives === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">{t('backup.archives.empty')}</p>
      ) : (
        <div className="space-y-3">
          {archives
            .filter((w) => w.archives.length > 0)
            .map((w) => (
              <div
                key={w.workspaceId}
                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-2"
              >
                <div className="px-1 pb-1 text-xs font-semibold text-[var(--color-text)]">
                  {w.workspaceName}{' '}
                  <span className="font-normal text-[var(--color-text-dim)]">
                    · {w.workspaceId} · {t('backup.archives.count', { count: w.archives.length })}
                  </span>
                </div>
                <div className="divide-y divide-[var(--color-border)]">
                  {w.archives.map((a) => {
                    const confirmingRestore = confirmArchive === a.name
                    const confirmingDelete = confirmDelete === a.name
                    const isRestoring = restoring === a.name
                    const isDeleting = deleting === a.name
                    const busy = !!restoring || !!deleting
                    return (
                      <div
                        key={a.name}
                        className="flex items-center justify-between gap-2 px-1 py-1.5"
                      >
                        <div className="min-w-0">
                          <div className="truncate font-mono text-[11px] text-[var(--color-text)]">
                            {a.name}
                          </div>
                          <div className="text-[10px] text-[var(--color-text-dim)]">
                            {formatDateTime(new Date(a.modified * 1000), {
                              dateStyle: 'short',
                              timeStyle: 'medium',
                            })}{' '}
                            · {formatBytes(a.bytes)}
                          </div>
                        </div>
                        {confirmingRestore ? (
                          <div className="flex shrink-0 items-center gap-1">
                            <button
                              type="button"
                              onClick={() => restore(w.workspaceId, a.name)}
                              disabled={isRestoring}
                              className="rounded bg-[var(--color-danger)] px-2 py-1 text-[11px] font-medium text-[var(--color-on-danger)] hover:opacity-90 disabled:opacity-40"
                            >
                              {isRestoring ? t('backup.restoring') : t('backup.confirmRestore')}
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmArchive(null)}
                              disabled={isRestoring}
                              className="rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)]"
                            >
                              {t('shared.cancel')}
                            </button>
                          </div>
                        ) : confirmingDelete ? (
                          <div className="flex shrink-0 items-center gap-1">
                            <button
                              type="button"
                              onClick={() => del(w.workspaceId, a.name)}
                              disabled={isDeleting}
                              className="rounded bg-[var(--color-danger)] px-2 py-1 text-[11px] font-medium text-[var(--color-on-danger)] hover:opacity-90 disabled:opacity-40"
                            >
                              {isDeleting ? t('shared.deleting') : t('backup.confirmDelete')}
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmDelete(null)}
                              disabled={isDeleting}
                              className="rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)]"
                            >
                              {t('shared.cancel')}
                            </button>
                          </div>
                        ) : (
                          <div className="flex shrink-0 items-center gap-1">
                            <button
                              type="button"
                              onClick={() => {
                                setConfirmDelete(null)
                                setConfirmArchive(a.name)
                              }}
                              disabled={busy}
                              className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)] disabled:opacity-40"
                            >
                              <RotateCcw size={12} /> {t('backup.restore')}
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                setConfirmArchive(null)
                                setConfirmDelete(a.name)
                              }}
                              disabled={busy}
                              title={t('backup.deleteArchive')}
                              className="flex items-center rounded border border-[var(--color-border)] px-2 py-1 text-[11px] text-[var(--color-danger)] hover:border-[var(--color-danger)] disabled:opacity-40"
                            >
                              <Trash2 size={12} />
                            </button>
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            ))}
        </div>
      )}
    </>
  )
}
