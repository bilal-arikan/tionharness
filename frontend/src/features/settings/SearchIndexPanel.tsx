// Search-index lifecycle panel, anchored in the External Tools settings screen
// under the zvec-grep callout. It answers the question the capability prompt
// raises but nothing used to show: TionHarness claims to manage the agent's
// search indexes, so the user must be able to SEE which ones exist, which one is
// building, and which one failed and why.
//
// The three actions map to the three things a user can legitimately want from a
// store they did not build by hand:
//   - Refresh: re-embed in place, keeping the existing vectors. Cheap.
//   - Rebuild: discard the store and build it again. The only cure for vectors
//     written by a different embedding model, and for a run that half-finished.
//   - Drop: delete it. Confirmation-gated and never automatic.
import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Hammer, Trash2, ScanSearch } from 'lucide-react'
import { api } from '@/api'
import type { SearchIndexStatus } from '@/types'
import { displayPath } from '@/shared/lib/paths'
import {
  canActOn,
  formatIndexTime,
  isAnyIndexing,
  phaseLook,
  refreshVerb,
} from '@/shared/lib/searchIndexState'

// How often to re-read the list while a run is in flight. Indexing takes
// minutes and the endpoint is an in-memory read, so this is cheap; polling stops
// entirely once nothing is indexing (see the effect below).
const POLL_MS = 2000

interface Props {
  onError: (msg: string) => void
}

export function SearchIndexPanel({ onError }: Props) {
  const [rows, setRows] = useState<SearchIndexStatus[] | null>(null)
  const [loading, setLoading] = useState(true)
  // Key of the row with a request in flight, so only its buttons go busy.
  const [busy, setBusy] = useState<string | null>(null)
  // Root of the row whose drop confirmation is open, plus what the user typed.
  const [confirming, setConfirming] = useState<string | null>(null)
  const [confirmText, setConfirmText] = useState('')
  // Outcome of the last action, kept next to the list rather than as a toast:
  // a 409 ("already running", "confirmation does not match") is something the
  // user has to read and act on.
  const [notice, setNotice] = useState<{ ok: boolean; text: string } | null>(null)

  const rowKey = (r: SearchIndexStatus) => `${r.tool}:${r.root}`

  // run lands the list through callbacks only (so the mount effect and the
  // poll may call it); load is the manual entry point that re-arms the spinner.
  const run = useCallback(
    () =>
      api
        .listSearchIndexes()
        .then(setRows)
        .catch((e) => onError((e as Error).message))
        .finally(() => setLoading(false)),
    [onError],
  )
  const load = useCallback(() => {
    setLoading(true)
    return run()
  }, [run])

  useEffect(() => {
    void run()
  }, [run])

  // Poll only while something is actually building, and stop the moment nothing
  // is: indexing is the only phase that changes on its own, so polling a settled
  // list would hit the endpoint forever for no new information.
  //
  // `load` is a stable useCallback, so the interval is torn down and rebuilt on
  // the indexing flag alone rather than on every render — which is what keeps
  // two timers from stacking.
  const indexing = !!rows && isAnyIndexing(rows)
  useEffect(() => {
    if (!indexing) return
    const id = setInterval(() => void load(), POLL_MS)
    return () => clearInterval(id)
  }, [indexing, load])

  // Run one action against one row, then re-read the list so what is shown is
  // the server's state rather than what we hoped happened.
  const act = async (row: SearchIndexStatus, run: () => Promise<unknown>, done: string) => {
    setBusy(rowKey(row))
    setNotice(null)
    try {
      await run()
      setNotice({ ok: true, text: done })
      await load()
    } catch (e) {
      // The backend's own message is the useful one here (409 "a run is already
      // in flight", "the confirmation does not match the root"), so it is shown
      // verbatim instead of a generic failure line.
      setNotice({ ok: false, text: (e as Error).message })
    } finally {
      setBusy(null)
    }
  }

  const refresh = (row: SearchIndexStatus, rebuild: boolean) =>
    act(
      row,
      () => api.refreshSearchIndex(row.tool, row.root, rebuild),
      rebuild
        ? 'Yeniden kurma başlatıldı — tamamlanması dakikalar sürebilir.'
        : refreshVerb(row) === 'Create'
          ? 'Kurulum başlatıldı — tamamlanması dakikalar sürebilir.'
          : 'Yenileme başlatıldı — tamamlanması dakikalar sürebilir.',
    )

  const drop = async (row: SearchIndexStatus) => {
    await act(
      row,
      () => api.dropSearchIndex(row.tool, row.root, confirmText.trim()),
      'İndeks silindi.',
    )
    setConfirming(null)
    setConfirmText('')
  }

  return (
    <div
      data-testid="search-index-panel"
      className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium text-[var(--color-text)]">🗂 Search index durumu</span>
        <button
          type="button"
          data-testid="search-index-reload"
          disabled={loading}
          onClick={load}
          className="inline-flex items-center gap-1 rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[11px] hover:text-[var(--color-text)] disabled:opacity-50"
          title="Listeyi yeniden oku"
        >
          <ScanSearch size={11} />
          {loading ? '…' : 'Yenile'}
        </button>
      </div>
      <p className="mt-1">
        TionHarness'in ajan adına yönettiği arama indeksleri — <code>(araç, kök)</code> çifti başına
        bir satır. Liste <span className="font-medium text-[var(--color-text)]">süreç geneli</span>:
        aynı depoyu açan iki workspace diskte tek bir store paylaşır.{' '}
        <span className="font-medium text-[var(--color-text)]">Yenile</span> mevcut vektörleri
        koruyarak yeniden gömer,{' '}
        <span className="font-medium text-[var(--color-text)]">Yeniden kur</span> store'u silip
        baştan kurar (gömme modeli değiştiyse tek çare),{' '}
        <span className="font-medium text-[var(--color-text)]">Sil</span> ise indeksi diskten
        kaldırır ve kök yolunun elle yazılmasını ister. codebase-memory satırlarında yenile artımlı
        yeniden indeksler, yeniden kur ve sil projeyi sunucunun kendi deposundan kaldırır.
      </p>

      {rows && rows.length === 0 && (
        <p data-testid="search-index-empty" className="mt-2">
          Henüz yönetilen bir indeks yok. zvec-grep veya codebase-memory MCP sunucusu ekliyken bir
          oturum açıldığında çalışma dizininin indeksi arka planda kurulur ve burada görünür.
        </p>
      )}

      {rows && rows.length > 0 && (
        <div className="mt-2 flex flex-col gap-1.5">
          {rows.map((row) => {
            const look = phaseLook(row.phase)
            const key = rowKey(row)
            const rowBusy = busy === key
            const actionable = canActOn(row)
            return (
              <div
                key={key}
                data-testid="search-index-row"
                data-root={row.root}
                data-phase={row.phase}
                className="rounded border border-[var(--color-border)] px-2.5 py-1.5"
              >
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <code className="rounded bg-[var(--color-surface-2)] px-1 font-medium">
                      {row.tool}
                    </code>
                    <span
                      data-testid="search-index-phase"
                      className="rounded px-1.5 py-0.5 font-mono text-[10px]"
                      style={{
                        color: look.tone,
                        backgroundColor: `color-mix(in srgb, ${look.tone} 14%, transparent)`,
                      }}
                    >
                      {look.label}
                    </span>
                    {row.action && (
                      <span className="font-mono text-[10px] text-[var(--color-text-dim)]">
                        {row.action}
                      </span>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <span
                      data-testid="search-index-updated"
                      className="font-mono text-[10px]"
                      title={row.updatedAt ?? ''}
                    >
                      {formatIndexTime(row.updatedAt)}
                    </span>
                    <button
                      type="button"
                      data-testid="search-index-refresh"
                      data-root={row.root}
                      disabled={rowBusy || !actionable}
                      onClick={() => refresh(row, false)}
                      title={
                        actionable
                          ? 'Mevcut store korunarak yeniden gömülür'
                          : 'Bu indeks için bir çalışma zaten sürüyor'
                      }
                      className="inline-flex items-center gap-1 rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[11px] hover:text-[var(--color-text)] disabled:opacity-50"
                    >
                      <RefreshCw size={10} />
                      {rowBusy ? '…' : refreshVerb(row) === 'Create' ? 'Kur' : 'Yenile'}
                    </button>
                    <button
                      type="button"
                      data-testid="search-index-rebuild"
                      data-root={row.root}
                      disabled={rowBusy || !actionable}
                      onClick={() => refresh(row, true)}
                      title="Store silinip baştan kurulur — gömme modeli değiştiyse gerekli"
                      className="inline-flex items-center gap-1 rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[11px] hover:text-[var(--color-text)] disabled:opacity-50"
                    >
                      <Hammer size={10} />
                      {rowBusy ? '…' : 'Yeniden kur'}
                    </button>
                    <button
                      type="button"
                      data-testid="search-index-drop-open"
                      data-root={row.root}
                      disabled={rowBusy || !actionable}
                      onClick={() => {
                        setConfirming(confirming === key ? null : key)
                        setConfirmText('')
                        setNotice(null)
                      }}
                      title="İndeksi diskten sil (kök yolunu yazarak onay ister)"
                      className="inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] disabled:opacity-50"
                    >
                      <Trash2 size={10} />
                      Sil
                    </button>
                  </div>
                </div>

                <div className="mt-0.5 truncate font-mono text-[11px]" title={row.root}>
                  {displayPath(row.root)}
                </div>

                {row.embedding && (
                  <div className="text-[10px] text-[var(--color-text-dim)]">
                    {row.embedding}
                    {row.toolVersion ? ` · v${row.toolVersion}` : ''}
                  </div>
                )}

                {/* A failure must state its reason: an index that silently reads
                    as unusable is exactly what the ledger exists to prevent. */}
                {row.phase === 'failed' && row.error && (
                  <p
                    data-testid="search-index-error"
                    data-root={row.root}
                    className="mt-1 rounded bg-[color-mix(in_srgb,var(--color-danger)_8%,transparent)] px-2 py-1 text-[11px] text-[var(--color-danger)]"
                  >
                    {row.error}
                  </p>
                )}

                {/* Drop confirmation. The typed path is sent as confirmRoot and
                    must match exactly; the backend refuses with 409 otherwise, so
                    the button stays disabled until it does. */}
                {confirming === key && (
                  <div
                    data-testid="search-index-drop-confirm"
                    className="mt-1.5 rounded border border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_6%,transparent)] px-2 py-1.5"
                  >
                    <p className="text-[11px] text-[var(--color-text)]">
                      Bu indeks diskten silinecek ve yeniden kurulması dakikalar sürer. Onaylamak
                      için kök yolunu birebir yaz:
                    </p>
                    <code className="mt-1 block break-all text-[10px]">{row.root}</code>
                    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
                      <input
                        data-testid="search-index-drop-input"
                        value={confirmText}
                        onChange={(e) => setConfirmText(e.target.value)}
                        placeholder={row.root}
                        spellCheck={false}
                        className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-[11px] text-[var(--color-text)]"
                      />
                      <button
                        type="button"
                        data-testid="search-index-drop-confirm-btn"
                        disabled={rowBusy || confirmText.trim() !== row.root}
                        onClick={() => drop(row)}
                        className="rounded bg-[var(--color-danger)] px-2 py-1 text-[11px] font-medium text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-40"
                      >
                        {rowBusy ? '…' : 'İndeksi sil'}
                      </button>
                      <button
                        type="button"
                        data-testid="search-index-drop-cancel"
                        onClick={() => {
                          setConfirming(null)
                          setConfirmText('')
                        }}
                        className="rounded px-2 py-1 text-[11px] hover:text-[var(--color-text)]"
                      >
                        Vazgeç
                      </button>
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}

      {notice && (
        <p
          data-testid="search-index-notice"
          className={`mt-2 text-[11px] ${
            notice.ok ? 'text-[var(--color-success)]' : 'text-[var(--color-warning)]'
          }`}
        >
          {notice.text}
        </p>
      )}
    </div>
  )
}
