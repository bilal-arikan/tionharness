import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Download,
  RefreshCw,
  Store,
  Check,
  KeyRound,
  Sparkles,
  Users,
  Plug,
  GitBranch,
  Boxes,
  Database,
  Wrench,
  Server,
  ArrowUpCircle,
  type LucideIcon,
} from 'lucide-react'
import type { Agent, Pack, PackKind, Secret } from '../../types'
import { api } from '../../api'
import { Markdown } from '../markdown/Markdown'
import { Button } from '../common'
import { SkillImportDialog } from './SkillImportDialog'
import { RegistryManager } from './RegistryManager'

// updateAvailable reports whether a pack's catalog version is newer than the
// version last installed here (best-effort dotted-numeric comparison).
function updateAvailable(pack: Pack): boolean {
  if (!pack.installedVersion || !pack.version) return false
  return compareVersions(pack.version, pack.installedVersion) > 0
}

// compareVersions: -1 if a<b, 0 if equal, 1 if a>b. Non-numeric segments → 0;
// any pre-release/build suffix after '-'/'+' is ignored. Mirrors the backend.
function compareVersions(a: string, b: string): number {
  const parts = (v: string) =>
    v.trim().replace(/^v/, '').split(/[-+]/)[0].split('.').map((s) => parseInt(s, 10) || 0)
  const pa = parts(a)
  const pb = parts(b)
  const n = Math.max(pa.length, pb.length)
  for (let i = 0; i < n; i++) {
    const x = pa[i] ?? 0
    const y = pb[i] ?? 0
    if (x < y) return -1
    if (x > y) return 1
  }
  return 0
}

interface Props {
  onError: (msg: string) => void
  // Jump to the Secrets screen to manage vault entries (provider keys live in
  // the vault, never typed as plaintext — mirrors the Settings providers panel).
  onManageSecrets?: () => void
  // Called after a successful install so the host can refresh the matching
  // collection (e.g. App-level agents state) without a full page reload.
  onInstalled?: (kind: PackKind) => void
}

// Kind filter entries shown as a left sidebar (no "all" option — one kind is
// always selected, defaulting to the first). Each maps to a market pack kind.
const KIND_NAV: { key: PackKind; label: string; icon: LucideIcon }[] = [
  { key: 'skill', label: 'Skills', icon: Sparkles },
  { key: 'agent', label: 'Agents', icon: Users },
  { key: 'provider', label: 'Providers', icon: Plug },
  { key: 'flow', label: 'Flows', icon: GitBranch },
  { key: 'workspace', label: 'Workspaces', icon: Boxes },
  { key: 'memory', label: 'Memories', icon: Database },
  { key: 'mcp', label: 'Tools (MCP)', icon: Wrench },
]

const KIND_LABEL: Record<PackKind, string> = {
  skill: 'Skill',
  agent: 'Ajan',
  provider: 'Sağlayıcı',
  flow: 'Akış',
  workspace: 'Workspace',
  memory: 'Bellek',
  mcp: 'MCP',
}

const INSTALL_LABEL: Record<PackKind, string> = {
  skill: "Bu workspace'e kur",
  agent: 'Ajanı oluştur',
  provider: 'Sağlayıcıyı ekle',
  flow: 'Akışı içe aktar',
  workspace: 'Workspace oluştur',
  memory: 'Belleğe ekle',
  mcp: 'Sunucuyu ekle',
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

// ColumnsPreview renders a kanban column layout as colored chips (shared by the
// board and workspace previews).
function ColumnsPreview({ columns }: { columns?: { key: string; label: string; color?: string }[] }) {
  if (!columns || columns.length === 0) return null
  return (
    <div className="flex flex-wrap gap-1.5">
      {columns.map((c) => (
        <span
          key={c.key}
          className="rounded px-2 py-0.5 text-[11px]"
          style={{
            background: c.color ? `${c.color}22` : 'var(--color-surface-2)',
            color: c.color || 'var(--color-text-dim)',
          }}
        >
          {c.label}
        </span>
      ))}
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
          <Row k="Skills" v={a.skills?.join(', ')} />
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
  if (pack.kind === 'workspace' && p?.workspace) {
    const wsp = p.workspace
    return (
      <div className="space-y-3">
        <div className="space-y-1">
          <Row k="Ad" v={wsp.name} />
          <Row k="Simge" v={wsp.icon} />
        </div>
        {wsp.instructions && (
          <div>
            <span className="text-xs text-[var(--color-text-dim)]">Yönergeler</span>
            <p className="mt-1 whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-[11px]">{wsp.instructions}</p>
          </div>
        )}
        {wsp.columns && wsp.columns.length > 0 && (
          <div>
            <span className="text-xs text-[var(--color-text-dim)]">Board kolonları</span>
            <div className="mt-1">
              <ColumnsPreview columns={wsp.columns} />
            </div>
          </div>
        )}
        <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">
          Kurunca bu şablondan yeni bir workspace oluşturulur.
        </p>
      </div>
    )
  }
  if (pack.kind === 'mcp' && p?.mcp) {
    const m = p.mcp
    return (
      <div className="space-y-1">
        <Row k="Ad" v={m.name} />
        <Row k="Transport" v={m.transport} />
        <Row k="Komut" v={m.command} />
        <Row k="Argümanlar" v={m.args} />
        <Row k="URL" v={m.url} />
        <p className="pt-2 text-[11px] text-[var(--color-text-dim)]">
          Kurunca workspace'e bir MCP sunucusu eklenir; araçları sonraki turda görünür.
        </p>
      </div>
    )
  }
  if (pack.kind === 'memory' && p?.memory) {
    return (
      <div className="space-y-2">
        <p className="text-xs text-[var(--color-text-dim)]">{p.memory.entries.length} bellek girdisi:</p>
        <ul className="space-y-1.5">
          {p.memory.entries.map((e, i) => (
            <li key={i} className="rounded bg-[var(--color-surface-2)] p-2 text-[11px] leading-relaxed">
              {e.content}
            </li>
          ))}
        </ul>
        <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">
          Eklenince bu girdiler workspace'in ilk ajanının belleğine yazılır.
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

// existingKeys holds, per kind, the identifiers of entities already present in
// the workspace, so the market can mark a pack as already installed and block a
// duplicate. Keys: skill→slug, agent→lowercased name, flow→lowercased name,
// provider→id, workspace→lowercased name, mcp→lowercased name. memory is a seed
// action (no entity identity), so it is never marked installed.
interface ExistingKeys {
  skills: Set<string>
  agents: Set<string>
  flows: Set<string>
  providers: Set<string>
  workspaces: Set<string>
  mcp: Set<string>
}

const emptyExisting = (): ExistingKeys => ({
  skills: new Set(),
  agents: new Set(),
  flows: new Set(),
  providers: new Set(),
  workspaces: new Set(),
  mcp: new Set(),
})

// packTargetKey returns the identifier a pack would occupy once installed, in
// the same shape as ExistingKeys. memory returns null (no dedup — it seeds entries
// rather than create a uniquely-named entity).
function packTargetKey(pack: Pack): { set: keyof ExistingKeys; key: string } | null {
  switch (pack.kind) {
    case 'skill':
      return { set: 'skills', key: pack.id.replace(/^skill\./, '') }
    case 'provider':
      return { set: 'providers', key: pack.id.replace(/^provider\./, '') }
    case 'agent':
      return { set: 'agents', key: pack.name.trim().toLowerCase() }
    case 'flow':
      return { set: 'flows', key: pack.name.trim().toLowerCase() }
    case 'workspace':
      return { set: 'workspaces', key: pack.name.trim().toLowerCase() }
    case 'mcp':
      return { set: 'mcp', key: pack.name.trim().toLowerCase() }
    case 'memory':
      return null
  }
}

export function MarketPanel({ onError, onManageSecrets, onInstalled }: Props) {
  const [packs, setPacks] = useState<Pack[]>([])
  const [tab, setTab] = useState<PackKind>(KIND_NAV[0].key)
  const [selected, setSelected] = useState<Pack | null>(null)
  const [busy, setBusy] = useState(false)
  const [installed, setInstalled] = useState<Set<string>>(new Set())
  const [existing, setExisting] = useState<ExistingKeys>(emptyExisting)
  // Provider key, resolved from the secret vault (never typed). pickedSecret is
  // the chosen secret's name (for display); apiKey holds its revealed value.
  const [secrets, setSecrets] = useState<Secret[]>([])
  const [pickedSecret, setPickedSecret] = useState('')
  const [apiKey, setApiKey] = useState('')
  // Target agent for a memory pack install (memory is per-agent). Defaults to the
  // first agent; the user can pick another in the detail drawer.
  const [agents, setAgents] = useState<Agent[]>([])
  const [pickedAgent, setPickedAgent] = useState('')
  // Skill import dialog (moved here from the Skills screen).
  const [importing, setImporting] = useState(false)
  // Remote registry manager modal ("Kaynaklar").
  const [managingRegistries, setManagingRegistries] = useState(false)

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

  // loadExisting snapshots the workspace's current entities so the catalog can
  // flag packs that are already installed.
  const loadExisting = useCallback(async () => {
    try {
      const [skills, agents, flows, providers, workspaces, mcp] = await Promise.all([
        api.listSkills(),
        api.listAgents(),
        api.listFlows(),
        api.listCustomProviders(),
        api.listWorkspaces(),
        api.listMCPServers(),
      ])
      setExisting({
        skills: new Set(skills.map((s) => s.slug)),
        agents: new Set(agents.map((a) => a.name.trim().toLowerCase())),
        flows: new Set(flows.map((f) => f.name.trim().toLowerCase())),
        providers: new Set(providers.map((p) => p.id)),
        workspaces: new Set(workspaces.map((wsp) => wsp.name.trim().toLowerCase())),
        mcp: new Set(mcp.map((m) => m.name.trim().toLowerCase())),
      })
      setAgents(agents) // keep the list for the memory-pack target picker
    } catch {
      // Non-fatal: without this snapshot, packs simply aren't pre-marked.
    }
  }, [])

  useEffect(() => {
    void load()
    void loadSecrets()
    void loadExisting()
  }, [load, loadSecrets, loadExisting])

  // Default the memory-pack target to the first agent whenever the selection or
  // the agent list changes.
  useEffect(() => {
    if (selected?.kind === 'memory') setPickedAgent(agents[0]?.id ?? '')
  }, [selected, agents])

  // isInstalled reports whether the entity a pack would create already exists.
  const isInstalled = useCallback(
    (pack: Pack) => {
      const t = packTargetKey(pack)
      return t ? existing[t.set].has(t.key) : false
    },
    [existing],
  )

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

  const visible = useMemo(() => packs.filter((p) => p.kind === tab), [packs, tab])

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
      await api.refreshRegistries().catch(() => {}) // pull remote indexes (best-effort)
      await api.reloadMarket()
      await load()
      await loadExisting()
    } finally {
      setBusy(false)
    }
  }, [load, loadExisting])

  const install = useCallback(
    async (pack: Pack, overwrite = false) => {
      setBusy(true)
      try {
        const body: { overwrite?: boolean; apiKey?: string; agentId?: string } = { overwrite }
        if (pack.kind === 'provider' && apiKey.trim()) body.apiKey = apiKey.trim()
        if (pack.kind === 'memory' && pickedAgent) body.agentId = pickedAgent
        const res = await api.installPack(pack.id, body)
        setInstalled((prev) => new Set(prev).add(pack.id))
        onError(`✓ ${res.message}`)
        await loadExisting() // re-mark the catalog (this pack is now installed)
        await load() // refresh installedVersion decoration (ledger updated)
        onInstalled?.(pack.kind) // let the host refresh its matching collection
      } catch (e) {
        onError(e instanceof Error ? e.message : 'Kurulum başarısız')
      } finally {
        setBusy(false)
      }
    },
    [onError, apiKey, pickedAgent, loadExisting, load, onInstalled],
  )

  return (
    <div className="flex h-full">
      {/* Kind filter rail */}
      <div className="flex w-44 shrink-0 flex-col gap-1 border-r border-[var(--color-border)] p-2">
        <span className="px-2 py-1 text-[10px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          Kategoriler
        </span>
        {KIND_NAV.map((k) => {
          const Icon = k.icon
          const count = packs.filter((p) => p.kind === k.key).length
          const active = tab === k.key
          return (
            <button
              key={k.key}
              data-testid="market-kind-tab"
              data-kind={k.key}
              onClick={() => setTab(k.key)}
              className={`flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition ${
                active
                  ? 'bg-[var(--color-accent-soft)] font-medium text-[var(--color-accent)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
              }`}
            >
              <Icon size={16} strokeWidth={2} className="shrink-0" />
              <span className="min-w-0 flex-1 truncate text-left">{k.label}</span>
              <span className="text-[10px] text-[var(--color-text-dim)]">{count}</span>
            </button>
          )
        })}
      </div>

      {/* Catalog */}
      <div className="flex flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between border-b border-[var(--color-border)] px-5 py-3">
          <div className="flex items-center gap-2">
            <Store size={18} className="text-[var(--color-accent)]" />
            <h2 className="text-sm font-semibold">Market</h2>
            <span className="text-xs text-[var(--color-text-dim)]">{visible.length} paket</span>
          </div>
          <div className="flex items-center gap-1.5">
            {tab === 'skill' && (
              <button
                data-testid="market-import"
                onClick={() => setImporting(true)}
                title="Claude Code skill içe aktar (yerel klasör / GitHub)"
                className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
              >
                <Download size={13} /> İçe Aktar
              </button>
            )}
            <button
              data-testid="market-registries"
              onClick={() => setManagingRegistries(true)}
              title="Uzak kaynakları yönet (registry ekle/çıkar)"
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
            >
              <Server size={13} /> Kaynaklar
            </button>
            <button
              onClick={reload}
              disabled={busy}
              title="Uzak kaynakları yenile + katalogu tara"
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] disabled:opacity-50"
            >
              <RefreshCw size={13} className={busy ? 'animate-spin' : ''} /> Yenile
            </button>
          </div>
        </header>

        <div className="grid flex-1 grid-cols-[repeat(auto-fill,minmax(240px,1fr))] content-start gap-3 overflow-y-auto p-4">
          {visible.length === 0 && (
            <p className="col-span-full mt-8 text-center text-sm text-[var(--color-text-dim)]">
              Bu türde paket yok.
            </p>
          )}
          {visible.map((p) => {
            const done = installed.has(p.id) || isInstalled(p)
            return (
              <button
                key={p.id}
                data-testid="market-pack"
                data-pack-id={p.id}
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
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">
                    {p.source === 'remote' ? p.registryName || 'Uzak' : 'Yerel'}
                  </span>
                  {updateAvailable(p) && (
                    <span className="flex items-center gap-1 text-[10px] text-[var(--color-accent)]">
                      <ArrowUpCircle size={11} /> Güncelle (v{p.installedVersion}→v{p.version})
                    </span>
                  )}
                  {done && !updateAvailable(p) && (
                    <span className="flex items-center gap-1 text-[10px] text-[var(--color-success)]">
                      <Check size={11} /> Kuruldu
                    </span>
                  )}
                </div>
              </button>
            )
          })}
        </div>
      </div>

      {/* Detail popup (centered modal) */}
      {selected && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
          onClick={() => setSelected(null)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label={selected.name}
            data-testid="market-detail-modal"
            className="flex max-h-[85vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
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
            {selected.kind === 'memory' && (
              <div className="mt-3">
                <label className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  Hedef ajan
                </label>
                <select
                  data-testid="market-memory-agent"
                  value={pickedAgent}
                  onChange={(e) => setPickedAgent(e.target.value)}
                  disabled={agents.length === 0}
                  className="mt-1 w-full rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1.5 text-xs"
                >
                  {agents.length === 0 && <option value="">Ajan yok — önce bir ajan oluştur</option>}
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>{a.name}</option>
                  ))}
                </select>
                <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">
                  Bellek girdileri seçili ajanın belleğine yazılır.
                </p>
              </div>
            )}
            {(() => {
              const here = isInstalled(selected)
              const canUpdate = updateAvailable(selected)
              // Providers are id-keyed (Upsert) so re-installing just updates the
              // config/key — allowed and labelled "Güncelle". memory seeds entries
              // and is always re-runnable. A pack with a newer version than the one
              // recorded in the ledger is always updatable (overwrite). Other kinds
              // with identity are blocked once present to avoid duplicates.
              const reRunnable = selected.kind === 'memory'
              const blocked = here && selected.kind !== 'provider' && !reRunnable && !canUpdate
              // A memory pack needs a target agent; block install when none exist.
              const noAgentForMemory = selected.kind === 'memory' && agents.length === 0
              const label = canUpdate
                ? `Güncelle (v${selected.installedVersion}→v${selected.version})`
                : blocked
                  ? 'Zaten kurulu'
                  : here && selected.kind === 'provider'
                    ? 'Güncelle'
                    : INSTALL_LABEL[selected.kind]
              return (
                <div data-testid="market-pack-install" data-pack-id={selected.id} className="contents">
                  <Button
                    onClick={() => void install(selected, canUpdate)}
                    disabled={busy || blocked || noAgentForMemory}
                    className="mt-3 flex w-full items-center justify-center gap-1.5"
                  >
                    {canUpdate ? <ArrowUpCircle size={13} /> : blocked ? <Check size={13} /> : <Download size={13} />} {label}
                  </Button>
                </div>
              )
            })()}
          </div>

            {/* Payload preview — kind-specific */}
            <div className="flex-1 overflow-y-auto p-4">
              <PackPreview pack={selected} />
            </div>
          </div>
        </div>
      )}

      {importing && (
        <SkillImportDialog
          onClose={() => setImporting(false)}
          onImported={() => {
            void loadExisting() // mark the imported skill as installed in the catalog
          }}
        />
      )}

      {managingRegistries && (
        <RegistryManager
          onClose={() => setManagingRegistries(false)}
          onChanged={() => void reload()} // re-fetch remote indexes + catalog
        />
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
