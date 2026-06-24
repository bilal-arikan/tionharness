import { useCallback, useEffect, useState } from 'react'
import { Globe, Plus, RefreshCw, Trash2, AlertTriangle, Server } from 'lucide-react'
import type { Registry } from '../../types'
import { api } from '../../api'
import { Button } from '../common'

interface Props {
  onClose: () => void
  /** Called after registries change (add/remove/refresh) so the catalog reloads. */
  onChanged: () => void
}

// RegistryManager is the "Kaynaklar" modal: list configured remote registries,
// add a new one by URL, remove one, or refresh all. A registry serves a
// swarmregistry/v1 index (registry.json) listing downloadable packs.
export function RegistryManager({ onClose, onChanged }: Props) {
  const [registries, setRegistries] = useState<Registry[]>([])
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setRegistries(await api.listRegistries())
    } catch (e) {
      setErr((e as Error).message)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const add = useCallback(async () => {
    if (!url.trim()) {
      setErr('Registry URL gerekli (registry.json adresine işaret etmeli).')
      return
    }
    setBusy(true)
    setErr(null)
    try {
      setRegistries(await api.addRegistry(name.trim(), url.trim()))
      setName('')
      setUrl('')
      onChanged()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }, [name, url, onChanged])

  const remove = useCallback(
    async (u: string) => {
      setBusy(true)
      setErr(null)
      try {
        setRegistries(await api.removeRegistry(u))
        onChanged()
      } catch (e) {
        setErr((e as Error).message)
      } finally {
        setBusy(false)
      }
    },
    [onChanged],
  )

  const refresh = useCallback(async () => {
    setBusy(true)
    setErr(null)
    try {
      await api.refreshRegistries()
      onChanged()
    } catch (e) {
      // A failing registry is reported but others may still have refreshed.
      setErr((e as Error).message)
      onChanged()
    } finally {
      setBusy(false)
    }
  }, [onChanged])

  const inputCls =
    'w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Market kaynakları"
        data-testid="market-registries-modal"
        className="flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-surface-2)]">
            <Server size={18} className="text-[var(--color-accent)]" />
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="text-sm font-semibold text-[var(--color-text)]">Market Kaynakları</h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              Uzak registry'lerden paket çek. Her kaynak bir <code>registry.json</code> (swarmregistry/v1) sunar.
            </p>
          </div>
          <button
            onClick={refresh}
            disabled={busy}
            title="Tüm kaynakları yenile"
            className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] disabled:opacity-50"
          >
            <RefreshCw size={13} className={busy ? 'animate-spin' : ''} /> Yenile
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
          {/* Add form */}
          <div className="space-y-2 rounded-md border border-[var(--color-border)] p-3">
            <label className="block">
              <span className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Registry URL</span>
              <input
                data-testid="registry-url"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="https://raw.githubusercontent.com/owner/repo/main/registry.json"
                className={`${inputCls} font-mono`}
              />
            </label>
            <div className="flex gap-2">
              <input
                data-testid="registry-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Ad (opsiyonel)"
                className={`${inputCls} flex-1`}
              />
              <Button data-testid="registry-add" onClick={add} disabled={busy} className="flex items-center gap-1.5">
                <Plus size={14} /> Ekle
              </Button>
            </div>
          </div>

          {err && (
            <p className="flex items-start gap-1.5 rounded-md bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] px-3 py-2 text-xs text-[var(--color-danger)]">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" /> {err}
            </p>
          )}

          {/* List */}
          {registries.length === 0 ? (
            <p className="py-6 text-center text-sm text-[var(--color-text-dim)]">
              Henüz kaynak yok. Yukarıdan bir registry URL'si ekle.
            </p>
          ) : (
            <ul className="space-y-2">
              {registries.map((r) => (
                <li
                  key={r.url}
                  data-testid="registry-item"
                  className="flex items-center gap-2 rounded-md border border-[var(--color-border)] p-2.5"
                >
                  <Globe size={15} className="shrink-0 text-[var(--color-text-dim)]" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{r.name}</div>
                    <div className="truncate text-[11px] text-[var(--color-text-dim)]">{r.url}</div>
                  </div>
                  <button
                    onClick={() => void remove(r.url)}
                    disabled={busy}
                    title="Kaynağı kaldır"
                    className="flex items-center gap-1 rounded px-2 py-1 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] disabled:opacity-50"
                  >
                    <Trash2 size={13} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-[var(--color-border)] px-5 py-4">
          <button onClick={onClose} className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]">
            Kapat
          </button>
        </div>
      </div>
    </div>
  )
}
