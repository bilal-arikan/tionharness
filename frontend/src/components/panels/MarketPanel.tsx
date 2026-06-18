import { useCallback, useEffect, useMemo, useState } from 'react'
import { Download, RefreshCw, Store, Check } from 'lucide-react'
import type { Pack, PackKind } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'

interface Props {
  onError: (msg: string) => void
}

// Kind tabs. Only "skill" is installable in the MVP; the others list (when
// present) but show a "yakında" (coming soon) hint on the install action.
const KIND_TABS: { key: PackKind | 'all'; label: string }[] = [
  { key: 'all', label: 'Tümü' },
  { key: 'skill', label: 'Beceri' },
  { key: 'agent', label: 'Ajan' },
  { key: 'provider', label: 'Sağlayıcı' },
  { key: 'flow', label: 'Akış' },
]

const KIND_LABEL: Record<PackKind, string> = {
  skill: 'Beceri',
  agent: 'Ajan',
  provider: 'Sağlayıcı',
  flow: 'Akış',
}

function KindBadge({ kind }: { kind: PackKind }) {
  return (
    <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
      {KIND_LABEL[kind]}
    </span>
  )
}

export function MarketPanel({ onError }: Props) {
  const [packs, setPacks] = useState<Pack[]>([])
  const [tab, setTab] = useState<PackKind | 'all'>('all')
  const [selected, setSelected] = useState<Pack | null>(null)
  const [busy, setBusy] = useState(false)
  const [installed, setInstalled] = useState<Set<string>>(new Set())

  const load = useCallback(async () => {
    try {
      setPacks(await api.listMarket())
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Market yüklenemedi')
    }
  }, [onError])

  useEffect(() => {
    void load()
  }, [load])

  const visible = useMemo(
    () => (tab === 'all' ? packs : packs.filter((p) => p.kind === tab)),
    [packs, tab],
  )

  const openDetail = useCallback(
    async (pack: Pack) => {
      try {
        setSelected(await api.getPack(pack.id))
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Paket açılamadı')
      }
    },
    [onError],
  )

  const reload = useCallback(async () => {
    setBusy(true)
    try {
      await api.reloadMarket()
      await load()
    } finally {
      setBusy(false)
    }
  }, [load])

  const install = useCallback(
    async (pack: Pack, overwrite = false) => {
      setBusy(true)
      try {
        const res = await api.installPack(pack.id, { overwrite })
        setInstalled((prev) => new Set(prev).add(pack.id))
        onError(`✓ ${res.message}`)
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Kurulum başarısız')
      } finally {
        setBusy(false)
      }
    },
    [onError],
  )

  return (
    <div className="flex h-full">
      {/* Catalog */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3">
          <div className="flex items-center gap-2">
            <Store size={18} className="text-[var(--color-accent)]" />
            <h2 className="text-sm font-semibold">Market</h2>
            <span className="text-xs text-[var(--color-text-dim)]">{visible.length} paket</span>
          </div>
          <button
            onClick={reload}
            disabled={busy}
            className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] disabled:opacity-50"
          >
            <RefreshCw size={13} className={busy ? 'animate-spin' : ''} /> Yenile
          </button>
        </header>

        <div className="flex gap-1 border-b border-[var(--color-border)] px-4 py-2">
          {KIND_TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`rounded px-2.5 py-1 text-xs font-medium ${
                tab === t.key
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              {t.label}
            </button>
          ))}
        </div>

        <div className="grid flex-1 grid-cols-[repeat(auto-fill,minmax(240px,1fr))] content-start gap-3 overflow-y-auto p-4">
          {visible.length === 0 && (
            <p className="col-span-full mt-8 text-center text-sm text-[var(--color-text-dim)]">
              Bu türde paket yok.
            </p>
          )}
          {visible.map((p) => {
            const done = installed.has(p.id)
            return (
              <button
                key={p.id}
                onClick={() => void openDetail(p)}
                className={`flex flex-col gap-2 rounded-lg border p-3 text-left transition hover:border-[var(--color-accent)] ${
                  selected?.id === p.id
                    ? 'border-[var(--color-accent)] bg-[var(--color-surface-2)]'
                    : 'border-[var(--color-border)]'
                }`}
              >
                <div className="flex items-center gap-2">
                  <span
                    className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-lg"
                    style={{ background: p.color ? `${p.color}22` : 'var(--color-surface-2)' }}
                  >
                    {p.icon || '📦'}
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{p.name}</div>
                    <div className="flex items-center gap-1.5">
                      <KindBadge kind={p.kind} />
                      {p.version && (
                        <span className="text-[10px] text-[var(--color-text-dim)]">v{p.version}</span>
                      )}
                    </div>
                  </div>
                </div>
                <p className="line-clamp-3 text-xs text-[var(--color-text-dim)]">{p.description}</p>
                {done && (
                  <span className="flex items-center gap-1 text-[10px] text-[var(--color-success)]">
                    <Check size={11} /> Kuruldu
                  </span>
                )}
              </button>
            )
          })}
        </div>
      </div>

      {/* Detail drawer */}
      {selected && (
        <aside className="flex w-96 shrink-0 flex-col overflow-hidden border-l border-[var(--color-border)]">
          <header className="flex items-start justify-between gap-2 border-b border-[var(--color-border)] px-4 py-3">
            <div className="flex items-center gap-2">
              <span className="text-xl">{selected.icon || '📦'}</span>
              <div>
                <div className="text-sm font-semibold">{selected.name}</div>
                <div className="flex items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]">
                  <KindBadge kind={selected.kind} />
                  {selected.author && <span>· {selected.author}</span>}
                  {selected.version && <span>· v{selected.version}</span>}
                </div>
              </div>
            </div>
            <button
              onClick={() => setSelected(null)}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              ✕
            </button>
          </header>

          <div className="border-b border-[var(--color-border)] p-4">
            <p className="text-xs text-[var(--color-text-dim)]">{selected.description}</p>
            {selected.kind === 'skill' ? (
              <button
                onClick={() => void install(selected)}
                disabled={busy}
                className="mt-3 flex w-full items-center justify-center gap-1.5 rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
              >
                <Download size={13} /> Bu workspace'e kur
              </button>
            ) : (
              <p className="mt-3 rounded bg-[var(--color-surface-2)] px-3 py-1.5 text-center text-xs text-[var(--color-text-dim)]">
                {KIND_LABEL[selected.kind]} kurulumu yakında
              </p>
            )}
          </div>

          {/* Payload preview — skill body as rendered markdown */}
          <div className="flex-1 overflow-y-auto p-4">
            {selected.payload?.skill?.body ? (
              <Markdown>{stripFrontmatter(selected.payload.skill.body)}</Markdown>
            ) : (
              <p className="text-xs text-[var(--color-text-dim)]">Önizleme yok.</p>
            )}
          </div>
        </aside>
      )}
    </div>
  )
}

// stripFrontmatter removes a leading --- delimited block for the preview so the
// reader sees the instructions, not the YAML header.
function stripFrontmatter(text: string): string {
  if (!text.startsWith('---')) return text
  const end = text.indexOf('\n---', 3)
  if (end < 0) return text
  return text.slice(text.indexOf('\n', end + 1) + 1).trimStart()
}
