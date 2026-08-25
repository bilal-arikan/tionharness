import { useState, useEffect } from 'react'
import { Archive, RotateCcw, Trash2 } from 'lucide-react'
import type { BackupStatus, WorkspaceArchives } from '@/types'
import { api, getActiveWorkspace } from '@/api'
import { Field, Toggle, inputCls } from './primitives'
import { SubHead } from './settingsPanelShared'
import type { PanelProps } from './settingsPanelShared'
import { formatDateTime } from '@/shared/lib/intl'

// formatBytes renders a byte count as a compact human-readable size.
function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

export function BackupPanel({ draft, set }: PanelProps) {
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
      setMsg(
        `${res.archives.length} workspace yedeklendi${fails ? `, ${fails} hata` : ''} → ${res.dir}`,
      )
      refresh()
    } catch (e) {
      setMsg('Yedekleme başarısız: ' + (e as Error).message)
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
        setMsg(`Geri yüklendi: ${archive}. Aktif workspace yenileniyor…`)
        setTimeout(() => window.location.reload(), 1200)
        return // keep the row disabled until the reload lands
      }
      setMsg(`Geri yüklendi: ${archive}.`)
      refresh()
    } catch (e) {
      setMsg('Geri yükleme başarısız: ' + (e as Error).message)
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
      setMsg(`Silindi: ${archive}`)
      refresh()
    } catch (e) {
      setMsg('Silme başarısız: ' + (e as Error).message)
    } finally {
      setDeleting(null)
    }
  }

  const lastRunLabel = status?.lastRun
    ? formatDateTime(new Date(status.lastRun * 1000), { dateStyle: 'short', timeStyle: 'medium' })
    : 'henüz yok'

  const totalArchives = archives.reduce((n, w) => n + w.archives.length, 0)

  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        <span className="font-medium text-[var(--color-text)]">Uygulama-geneli ayar:</span> bu ayar{' '}
        <b>tüm</b> workspace&apos;ler için geçerlidir. Her workspace&apos;in tüm verisi (
        <code>store/</code>, <code>config/</code>, <code>workspace/</code>, ayarlar) belirli
        aralıklarla ayrı bir{' '}
        <span className="font-medium text-[var(--color-text)]">zip arşivine</span> alınır. Eski
        arşivler, tutulan sayı aşılınca otomatik silinir. Yedekler varsayılan olarak veri
        dizinindeki <code>backups/</code> klasörüne yazılır.
      </div>
      <Toggle
        label="Otomatik yedekleme"
        hint="Açıkken her workspace belirlenen aralıkta otomatik yedeklenir. İlk yedek bir aralık sonra alınır."
        checked={draft.backupEnabled}
        onChange={(v) => set('backupEnabled', v)}
      />
      <div className="grid grid-cols-2 gap-3">
        <Field
          label="Yedekleme aralığı (saat)"
          hint="İki otomatik yedek arası süre (en az 1 saat). 24 = günde bir."
        >
          <input
            type="number"
            min={1}
            value={draft.backupIntervalHours}
            onChange={(e) => set('backupIntervalHours', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
        <Field
          label="Saklanan yedek sayısı"
          hint="Workspace başına tutulan en yeni arşiv sayısı; eskiler budanır (en az 1)."
        >
          <input
            type="number"
            min={1}
            value={draft.backupRetain}
            onChange={(e) => set('backupRetain', Number(e.target.value))}
            className={inputCls}
          />
        </Field>
      </div>
      <Field
        label="Yedek klasörü"
        hint="Boş bırakılırsa veri dizinindeki backups/ kullanılır. Mutlak yol verebilirsin (ör. D:\\Backups\\TionHarness)."
      >
        <input
          value={draft.backupDir}
          onChange={(e) => set('backupDir', e.target.value)}
          placeholder="boş = <veri dizini>/backups"
          className={inputCls}
        />
      </Field>

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs">
        <span className="text-[var(--color-text-dim)]">Durum:</span>
        <span
          className={`rounded px-1.5 py-0.5 font-medium ${status?.enabled ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'}`}
        >
          {status?.enabled ? 'Açık' : 'Kapalı'}
        </span>
        <span className="text-[var(--color-text-dim)]">
          Son yedek: <span className="text-[var(--color-text)]">{lastRunLabel}</span>
        </span>
        {status?.dir && (
          <span className="text-[var(--color-text-dim)]">
            Klasör: <code className="text-[var(--color-text)]">{status.dir}</code>
          </span>
        )}
        {status?.lastError && (
          <span className="text-[var(--color-danger)]">Son hata: {status.lastError}</span>
        )}
      </div>

      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={runNow}
          disabled={running}
          className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-40"
        >
          {running ? 'Yedekleniyor…' : 'Şimdi yedekle'}
        </button>
        {msg && <span className="text-xs text-[var(--color-text-dim)]">{msg}</span>}
      </div>
      <p className="text-xs text-[var(--color-text-dim)]">
        Not: Aralık/saklama ayarları değişiklikleri <b>Kaydet</b>'ten sonra uygulanır. &quot;Şimdi
        yedekle&quot; kayıtlı ayarları kullanır.
      </p>

      {/* Archive list + one-click restore */}
      <SubHead icon={Archive}>Mevcut yedekler ({totalArchives})</SubHead>
      <div className="rounded-lg border border-[color-mix(in_srgb,var(--color-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_6%,transparent)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        ⚠ <b>Geri yükleme</b> seçilen workspace&apos;in <b>mevcut tüm verisini</b> arşivdekiyle
        değiştirir (geri alınamaz). İşlem sırasında o workspace kapatılıp arşivden yeniden açılır.
        Aktif workspace&apos;i geri yüklersen sayfa <b>otomatik yenilenir</b>; başka bir
        workspace&apos;i geri yüklersen ona geçtiğinde zaten taze yüklenir.
      </div>
      {totalArchives === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">
          Henüz yedek yok. &quot;Şimdi yedekle&quot; ile ilk yedeği oluşturabilirsin.
        </p>
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
                    · {w.workspaceId} · {w.archives.length} arşiv
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
                              className="rounded bg-[var(--color-danger)] px-2 py-1 text-[11px] font-medium text-white hover:opacity-90 disabled:opacity-40"
                            >
                              {isRestoring ? 'Geri yükleniyor…' : 'Eminim, geri yükle'}
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmArchive(null)}
                              disabled={isRestoring}
                              className="rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)]"
                            >
                              İptal
                            </button>
                          </div>
                        ) : confirmingDelete ? (
                          <div className="flex shrink-0 items-center gap-1">
                            <button
                              type="button"
                              onClick={() => del(w.workspaceId, a.name)}
                              disabled={isDeleting}
                              className="rounded bg-[var(--color-danger)] px-2 py-1 text-[11px] font-medium text-white hover:opacity-90 disabled:opacity-40"
                            >
                              {isDeleting ? 'Siliniyor…' : 'Eminim, sil'}
                            </button>
                            <button
                              type="button"
                              onClick={() => setConfirmDelete(null)}
                              disabled={isDeleting}
                              className="rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)]"
                            >
                              İptal
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
                              <RotateCcw size={12} /> Geri yükle
                            </button>
                            <button
                              type="button"
                              onClick={() => {
                                setConfirmArchive(null)
                                setConfirmDelete(a.name)
                              }}
                              disabled={busy}
                              title="Bu arşivi sil"
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
