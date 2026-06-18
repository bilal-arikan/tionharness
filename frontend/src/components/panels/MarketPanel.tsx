import { useCallback, useEffect, useMemo, useState } from 'react'
import { Download, RefreshCw, Store, Check, KeyRound } from 'lucide-react'
import type { Pack, PackKind, Secret } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'

interface Props {
  onError: (msg: string) => void
  // Jump to the Secrets screen to manage vault entries (provider keys live in
  // the vault, never typed as plaintext — mirrors the Settings providers panel).
  onManageSecrets?: () => void
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

const INSTALL_LABEL: Record<PackKind, string> = {
  skill: "Bu workspace'e kur",
  agent: 'Ajanı oluştur',
  provider: 'Sağlayıcıyı ekle',
  flow: 'Akışı içe aktar',
}

// Row is a small labelled key/value line used in the agent/provider preview.
function Row({ k, v }: { k: string; v?: string }) {
  if (!v) return null
  return (
    <div className="flex gap-2 text-xs">
      <span className="w-24 shrink-0 text-[var(--color-text-dim)]">{k}</span>
      <span className="min-w-0 break-words">{v}</span>
    </div>
  )
}

// PackPreview renders a kind-appropriate preview of the selected pack's payload.
function PackPreview({ pack }: { pack: Pack }) {
  const p = pack.payload
  if (pack.kind === 'skill' && p?.skill?.body) {
    return <Markdown>{stripFrontmatter(p.skill.body)}</Markdown>
  }
  if (pack.kind === 'agent' && p?.agent) {
    const a = p.agent
    return (
      <div className="space-y-3">
        {a.soul && <p className="text-xs leading-relaxed text-[var(--color-text)]">{a.soul}</p>}
        <div className="space-y-1">
          <Row k="Sağlayıcı" v={a.provider} />
          <Row k="Model" v={a.model || '(varsayılan)'} />
          <Row k="Düşünme" v={a.thinkingLevel} />
          <Row k="İzin modu" v={a.permissionMode} />
          <Row k="Beceriler" v={a.skills?.join(', ')} />
        </div>
      </div>
    )
  }
  if (pack.kind === 'provider' && p?.provider) {
    const pr = p.provider
    return (
      <div className="space-y-1">
        <Row k="Tür" v={pr.kind} />
        <Row k="Base URL" v={pr.baseUrl} />
        <Row k="Varsayılan model" v={pr.defaultModel} />
        {pr.models && (
          <div className="pt-1">
            <span className="text-xs text-[var(--color-text-dim)]">Modeller</span>
            <pre className="mt-1 whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-[11px]">{pr.models}</pre>
          </div>
        )}
      </div>
    )
  }
  if (pack.kind === 'flow' && p?.flow) {
    const nodes = flowNodeSummary(p.flow.graph)
    return (
      <div className="space-y-2">
        <p className="text-xs text-[var(--color-text-dim)]">{nodes.length} düğüm:</p>
        <ul className="space-y-1">
          {nodes.map((n, i) => (
            <li key={i} className="flex gap-2 text-xs">
              <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] uppercase text-[var(--color-text-dim)]">{n.type}</span>
              <span>{n.title || n.id}</span>
            </li>
          ))}
        </ul>
        <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">
          İçe aktarınca boş ajan slotları ilk ajana atanır; Akışlar ekranından düzenleyebilirsin.
        </p>
      </div>
    )
  }
  return <p className="text-xs text-[var(--color-text-dim)]">Önizleme yok.</p>
}

// flowNodeSummary safely parses a flow graph JSON string into a node list for
// the preview. Returns [] on any parse error.
function flowNodeSummary(graph: string): { id: string; type: string; title?: string }[] {
  try {
    const g = JSON.parse(graph) as { nodes?: { id: string; type: string; title?: string }[] }
    return g.nodes ?? []
  } catch {
    return []
  }
}

function KindBadge({ kind }: { kind: PackKind }) {
  return (
    <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
      {KIND_LABEL[kind]}
    </span>
  )
}

export function MarketPanel({ onError, onManageSecrets }: Props) {
  const [packs, setPacks] = useState<Pack[]>([])
  const [tab, setTab] = useState<PackKind | 'all'>('all')
  const [selected, setSelected] = useState<Pack | null>(null)
  const [busy, setBusy] = useState(false)
  const [installed, setInstalled] = useState<Set<string>>(new Set())
  // Provider key, resolved from the secret vault (never typed). pickedSecret is
  // the chosen secret's name (for display); apiKey holds its revealed value.
  const [secrets, setSecrets] = useState<Secret[]>([])
  const [pickedSecret, setPickedSecret] = useState('')
  const [apiKey, setApiKey] = useState('')

  const load = useCallback(async () => {
    try {
      setPacks(await api.listMarket())
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Market yüklenemedi')
    }
  }, [onError])

  const loadSecrets = useCallback(async () => {
    try {
      setSecrets(await api.listSecrets())
    } catch {
      // Secrets are optional; a load failure just means the picker is empty.
    }
  }, [])

  useEffect(() => {
    void load()
    void loadSecrets()
  }, [load, loadSecrets])

  // Resolve a chosen secret to its plaintext value (revealed on demand) and stage
  // it as the provider key for the next install.
  const pickSecret = useCallback(
    async (name: string) => {
      if (!name) {
        setPickedSecret('')
        setApiKey('')
        return
      }
      try {
        const { value } = await api.revealSecret(name)
        setPickedSecret(name)
        setApiKey(value)
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Sır çözülemedi')
      }
    },
    [onError],
  )

  const visible = useMemo(
    () => (tab === 'all' ? packs : packs.filter((p) => p.kind === tab)),
    [packs, tab],
  )

  const openDetail = useCallback(
    async (pack: Pack) => {
      setApiKey('')
      setPickedSecret('')
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
        const body: { overwrite?: boolean; apiKey?: string } = { overwrite }
        if (pack.kind === 'provider' && apiKey.trim()) body.apiKey = apiKey.trim()
        const res = await api.installPack(pack.id, body)
        setInstalled((prev) => new Set(prev).add(pack.id))
        onError(`✓ ${res.message}`)
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Kurulum başarısız')
      } finally {
        setBusy(false)
      }
    },
    [onError, apiKey],
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
            {selected.kind === 'provider' && (
              <div className="mt-3">
                <label className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  API anahtarı (sırlardan)
                </label>
                <div className="mt-1 flex items-center gap-2">
                  <select
                    value={pickedSecret}
                    onChange={(e) => void pickSecret(e.target.value)}
                    disabled={secrets.length === 0}
                    className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5 text-xs"
                  >
                    <option value="">
                      {secrets.length ? '🔑 Sırdan seç… (opsiyonel)' : 'Sır yok — önce ekle'}
                    </option>
                    {secrets.map((s) => (
                      <option key={s.name} value={s.name}>{s.name}</option>
                    ))}
                  </select>
                  {onManageSecrets && (
                    <button
                      onClick={onManageSecrets}
                      className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1.5 text-xs hover:border-[var(--color-accent)]"
                    >
                      <KeyRound size={12} /> Sırlar →
                    </button>
                  )}
                </div>
                <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
                  {pickedSecret
                    ? `🔑 "${pickedSecret}" kullanılacak`
                    : "Anahtarsız da kurulabilir; sonra Ayarlar → Sağlayıcılar'dan girilebilir."}
                </p>
              </div>
            )}
            <button
              onClick={() => void install(selected)}
              disabled={busy}
              className="mt-3 flex w-full items-center justify-center gap-1.5 rounded bg-[var(--color-accent)] px-3 py-1.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
            >
              <Download size={13} /> {INSTALL_LABEL[selected.kind]}
            </button>
          </div>

          {/* Payload preview — kind-specific */}
          <div className="flex-1 overflow-y-auto p-4">
            <PackPreview pack={selected} />
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
