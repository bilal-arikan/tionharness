import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
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
  Globe2,
  Clock,
  Bot,
  Zap,
  Timer,
  type LucideIcon,
} from 'lucide-react'
import type { Agent, Pack, PackKind, Secret, WorkspacePayload, WorkspaceTemplateFlow } from '../../types'
import { api } from '../../api'
import type { PriceTable } from '../../api/providers'
import type { PreviewItem } from '../../api/ingest'
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

// cacheLabel turns a provider's promptCache mode into a human label for the badge.
function cacheLabel(mode?: string): string {
  switch (mode) {
    case 'native':
      return 'Cache: ✅ cache_control'
    case 'auto':
      return 'Cache: ✅ otomatik'
    case 'none':
      return 'Cache: ❌ yok'
    default:
      return 'Cache: ? bilinmiyor'
  }
}

// CapBadge is a small capability pill (green when the capability is on, muted
// otherwise) used in the provider preview for cache / reasoning support.
function CapBadge({ label, on }: { label: string; on: boolean }) {
  return (
    <span
      className="rounded px-2 py-0.5 text-[11px]"
      style={{
        background: on ? 'var(--color-success, #16a34a)22' : 'var(--color-surface-2)',
        color: on ? 'var(--color-success, #16a34a)' : 'var(--color-text-dim)',
      }}
    >
      {label}
    </span>
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

// fmtPrice formats a USD/1M-token figure compactly (e.g. "$0.30", "$15").
function fmtPrice(n: number): string {
  if (n === 0) return 'ücretsiz'
  return '$' + (n < 1 ? n.toFixed(2).replace(/0+$/, '').replace(/\.$/, '') : String(n))
}

// ModelList renders a provider pack's models, each with its ballpark price
// (input / output per 1M tokens) when known. providerId is the pack slug used to
// look up prices[providerId][model].
function ModelList({ models, providerId, prices }: { models?: string; providerId: string; prices: PriceTable }) {
  if (!models) return null
  const ids = models.split('\n').map((s) => s.trim()).filter(Boolean)
  const table = prices[providerId] || {}
  return (
    <div className="pt-1">
      <span className="text-xs text-[var(--color-text-dim)]">Modeller ({ids.length})</span>
      <div className="mt-1 max-h-64 overflow-y-auto rounded bg-[var(--color-surface-2)] p-1.5">
        {ids.map((id) => {
          const pr = table[id]
          return (
            <div key={id} className="flex items-center justify-between gap-3 px-1 py-0.5 text-[11px]">
              <span className="min-w-0 break-all font-mono">{id}</span>
              {pr ? (
                <span className="shrink-0 tabular-nums text-[var(--color-text-dim)]" title="giriş / çıkış — $/1M token">
                  {fmtPrice(pr.inputPerMTok)} / {fmtPrice(pr.outputPerMTok)}
                </span>
              ) : (
                <span className="shrink-0 text-[var(--color-text-dim)] opacity-50">—</span>
              )}
            </div>
          )
        })}
      </div>
      <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">$/1M token (giriş / çıkış) — yaklaşık liste fiyatı.</p>
    </div>
  )
}

// PackPreview renders a kind-appropriate preview of the selected pack's payload.
function PackPreview({ pack, prices }: { pack: Pack; prices: PriceTable }) {
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
        <div className="flex flex-wrap gap-1.5 pt-1.5">
          <CapBadge label={cacheLabel(pr.promptCache)} on={pr.promptCache === 'native' || pr.promptCache === 'auto'} />
          <CapBadge label={pr.reasoning ? 'Düşünme: ✅ destekli' : 'Düşünme: —'} on={!!pr.reasoning} />
        </div>
        <ModelList models={pr.models} providerId={pack.id.replace(/^provider\./, '')} prices={prices} />
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
    return <WorkspacePackPreview wsp={p.workspace} />
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

// WorkspacePackPreview renders the full starter ecosystem of a workspace-template
// pack: a stat strip plus the agent team (with config), flows (with node types),
// schedules, embedded skills, instructions and board layout.
function WorkspacePackPreview({ wsp }: { wsp: WorkspacePayload }) {
  const agents = wsp.agents ?? []
  const flows = wsp.flows ?? []
  const schedules = wsp.schedules ?? []
  const skills = wsp.skills ?? []
  return (
    <div className="space-y-4">
      {/* Stat strip */}
      <div className="grid grid-cols-4 gap-2">
        <StatChip icon={Users} label="Ajan" value={agents.length} />
        <StatChip icon={GitBranch} label="Akış" value={flows.length} />
        <StatChip icon={Clock} label="Zamanlama" value={schedules.length} />
        <StatChip icon={Sparkles} label="Skill" value={skills.length} />
      </div>

      {wsp.instructions && (
        <PreviewSection title="Yönergeler">
          <p className="whitespace-pre-wrap rounded bg-[var(--color-surface-2)] p-2 text-[11px] leading-relaxed">{wsp.instructions}</p>
        </PreviewSection>
      )}

      {agents.length > 0 && (
        <PreviewSection title={`Ajanlar (${agents.length})`}>
          <div className="space-y-1.5">
            {agents.map((a) => (
              <div key={a.key} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2">
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="text-base leading-none">{a.avatar || '🤖'}</span>
                  <span className="text-xs font-medium">{a.name}</span>
                  {a.permissionMode && <MiniChip>{a.permissionMode}</MiniChip>}
                  {a.thinkingLevel && <MiniChip>🧠 {a.thinkingLevel}</MiniChip>}
                  {a.mcpEnabled && <MiniChip>MCP</MiniChip>}
                </div>
                {(a.provider || a.model) && (
                  <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">
                    {a.provider || '(varsayılan sağlayıcı)'}{a.model ? ` · ${a.model}` : ''}
                  </div>
                )}
                {a.soul && <p className="mt-1 line-clamp-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">{a.soul}</p>}
                {a.skills && a.skills.length > 0 && (
                  <div className="mt-1 flex flex-wrap gap-1">
                    {a.skills.map((s) => <MiniChip key={s}>📚 {s}</MiniChip>)}
                  </div>
                )}
              </div>
            ))}
          </div>
        </PreviewSection>
      )}

      {flows.length > 0 && (
        <PreviewSection title={`Akışlar (${flows.length})`}>
          <div className="space-y-1.5">
            {flows.map((f, i) => {
              const nodes = flowSummary(f)
              const hasBranch = nodes.some((n) => n.type === 'branch')
              const hasParallel = nodes.some((n) => n.type === 'parallel')
              return (
                <div key={i} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2">
                  <div className="flex flex-wrap items-center gap-1.5 text-xs font-medium">
                    {f.name}
                    {hasBranch && <MiniChip>branch</MiniChip>}
                    {hasParallel && <MiniChip>parallel</MiniChip>}
                  </div>
                  {f.description && <div className="mt-0.5 text-[10px] text-[var(--color-text-dim)]">{f.description}</div>}
                  <div className="mt-1.5 flex flex-wrap items-center gap-1">
                    {nodes.map((n, j) => {
                      const NIcon = NODE_ICON[n.type]
                      return (
                        <span key={j} className="flex items-center gap-1">
                          <span className="inline-flex items-center gap-1 rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px]" title={n.type}>
                            {NIcon ? <NIcon size={11} className="text-[var(--color-text-dim)]" /> : <span className="text-[var(--color-text-dim)]">•</span>}
                            {n.title || n.id}
                          </span>
                          {j < nodes.length - 1 && <span className="text-[10px] text-[var(--color-text-dim)]">→</span>}
                        </span>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        </PreviewSection>
      )}

      {schedules.length > 0 && (
        <PreviewSection title={`Zamanlamalar (${schedules.length})`}>
          <div className="space-y-1.5">
            {schedules.map((s, i) => (
              <div key={i} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 text-[11px]">
                <div className="flex items-center gap-2">
                  <code className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px]">{s.cronExpr}</code>
                  <span className="text-[var(--color-text-dim)]">→ {s.agentKey}</span>
                </div>
                {s.prompt && <p className="mt-1 line-clamp-2 leading-relaxed text-[var(--color-text-dim)]">{s.prompt}</p>}
              </div>
            ))}
          </div>
          <p className="mt-1 text-[10px] text-[var(--color-text-dim)]">Zamanlamalar pasif (disabled) kurulur — Zamanlamalar ekranından açılır.</p>
        </PreviewSection>
      )}

      {skills.length > 0 && (
        <PreviewSection title={`Gömülü Skill'ler (${skills.length})`}>
          <div className="space-y-1.5">
            {skills.map((s) => {
              const meta = skillMeta(s.body)
              return (
                <div key={s.slug} className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2">
                  <div className="flex items-center gap-1.5 text-xs font-medium">
                    📚 {meta.name || s.slug}
                    <code className="text-[10px] text-[var(--color-text-dim)]">{s.slug}</code>
                  </div>
                  {meta.description && <p className="mt-0.5 line-clamp-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">{meta.description}</p>}
                </div>
              )
            })}
          </div>
        </PreviewSection>
      )}

      {wsp.columns && wsp.columns.length > 0 && (
        <PreviewSection title="Board kolonları">
          <ColumnsPreview columns={wsp.columns} />
        </PreviewSection>
      )}

      <p className="pt-1 text-[11px] text-[var(--color-text-dim)]">Kurunca bu şablondan yeni bir workspace oluşturulur.</p>
    </div>
  )
}

// NODE_ICON maps an orchestration node type to a monochrome lucide glyph for the
// flow-chain preview (replaces colored emojis).
const NODE_ICON: Record<string, LucideIcon> = {
  agent: Bot, branch: GitBranch, parallel: Zap, delay: Timer, transform: Wrench,
}

// StatChip is one cell of the workspace stat strip. The icon is a monochrome
// lucide glyph (inherits the muted text color) rather than a colored emoji.
function StatChip({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: number }) {
  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-2 text-center">
      <div className="flex items-center justify-center gap-1 text-sm font-semibold">
        <Icon size={13} className="text-[var(--color-text-dim)]" /> {value}
      </div>
      <div className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">{label}</div>
    </div>
  )
}

// PreviewSection is a titled block used inside the workspace preview.
function PreviewSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <div className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">{title}</div>
      {children}
    </div>
  )
}

// MiniChip is a small inline label (modes, skills, flow tags).
function MiniChip({ children }: { children: ReactNode }) {
  return (
    <span className="rounded bg-[var(--color-bg)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]">{children}</span>
  )
}

// SOURCE_LABEL labels which tier/source a pack came from.
const SOURCE_LABEL: Record<string, string> = {
  bundled: '📦 Gömülü', global: '💾 Yerel', workspace: '🗂 Workspace', remote: '🌐 Uzak',
}

// PackMeta renders a row of metadata chips (source, installed version, created
// date, tags) shown under the description for every pack kind.
function PackMeta({ pack }: { pack: Pack }) {
  const created = pack.createdAt ? new Date(pack.createdAt * 1000).toLocaleDateString('tr-TR') : null
  const src = pack.source ? (SOURCE_LABEL[pack.source] ?? pack.source) : null
  if (!src && !created && !pack.installedVersion && !(pack.tags && pack.tags.length)) return null
  return (
    <div className="mt-2 flex flex-wrap items-center gap-1.5">
      {src && <MiniChip>{src}{pack.registryName ? ` · ${pack.registryName}` : ''}</MiniChip>}
      {pack.installedVersion && <MiniChip>Kurulu: v{pack.installedVersion}</MiniChip>}
      {created && <MiniChip>📅 {created}</MiniChip>}
      {pack.tags?.map((t) => <MiniChip key={t}>#{t}</MiniChip>)}
    </div>
  )
}

// flowSummary returns a flow's node list for the preview, from either its full
// graph JSON or its linear steps.
function flowSummary(flow: WorkspaceTemplateFlow): { id: string; type: string; title?: string }[] {
  if (flow.graph) return flowNodeSummary(flow.graph)
  if (flow.steps) return flow.steps.map((s) => ({ id: s.id, type: 'agent', title: s.title }))
  return []
}

// skillMeta parses a skill's SKILL.md frontmatter for its name/description.
function skillMeta(body: string): { name?: string; description?: string } {
  const m = /^---\s*\n([\s\S]*?)\n---/.exec(body)
  if (!m) return {}
  const fm = m[1]
  const name = /(?:^|\n)name:\s*"?(.+?)"?\s*(?:\n|$)/.exec(fm)?.[1]
  const description = /(?:^|\n)description:\s*"?(.+?)"?\s*(?:\n|$)/.exec(fm)?.[1]
  return { name, description }
}

// SourceRefPreview renders the GitHub-fetched preview of a directory-site (source-ref)
// catalog entry: the source link plus each discovered artifact's rendered body.
function SourceRefPreview({ url, items, loading }: { url: string; items: PreviewItem[] | null; loading: boolean }) {
  return (
    <div className="space-y-3">
      <a
        href={url}
        target="_blank"
        rel="noreferrer"
        className="flex items-center gap-1.5 text-xs text-[var(--color-accent)] hover:underline"
      >
        <Globe2 size={12} /> {url}
      </a>
      {loading && <p className="text-xs text-[var(--color-text-dim)]">Önizleme GitHub'dan yükleniyor…</p>}
      {!loading && items && items.length === 0 && (
        <p className="text-xs text-[var(--color-text-dim)]">Önizleme alınamadı (kurulumda yine de denenir).</p>
      )}
      {!loading &&
        items &&
        items.map((it, i) => (
          <div key={i} className="space-y-1">
            {items.length > 1 && (
              <div className="flex items-center gap-1.5 text-xs font-medium">
                <KindBadge kind={it.kind} /> <code>{it.slug}</code>
              </div>
            )}
            {it.warnings && it.warnings.length > 0 && (
              <p className="text-[10px] text-[var(--color-warning,#d97706)]">{it.warnings.join(' · ')}</p>
            )}
            <div className="text-xs">
              <Markdown>{stripFrontmatter(it.body || it.description || '')}</Markdown>
            </div>
          </div>
        ))}
    </div>
  )
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
  const [query, setQuery] = useState('') // catalog search (name/description/author)
  // Live directory-site (connector) search results — skill tab only.
  const [remoteResults, setRemoteResults] = useState<Pack[]>([])
  const [remoteWarnings, setRemoteWarnings] = useState<string[]>([])
  const [searching, setSearching] = useState(false)
  // Preview body for a selected source-ref pack (fetched on demand from GitHub).
  const [preview, setPreview] = useState<PreviewItem[] | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
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
  // Ballpark list prices (provider id → model → price), loaded once for the
  // provider preview's per-model cost hints.
  const [prices, setPrices] = useState<PriceTable>({})

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
    void api.prices().then(setPrices).catch(() => {})
  }, [load, loadSecrets, loadExisting])

  // Default the memory-pack target to the first agent whenever the selection or
  // the agent list changes.
  useEffect(() => {
    if (selected?.kind === 'memory') setPickedAgent(agents[0]?.id ?? '')
  }, [selected, agents])

  // Live directory-site (connector) search — skill tab only, debounced. The sites
  // hold thousands of skills, so results come from a search query, not a bulk list.
  useEffect(() => {
    const q = query.trim()
    if (tab !== 'skill' || q.length < 2) {
      setRemoteResults([])
      setRemoteWarnings([])
      setSearching(false)
      return
    }
    setSearching(true)
    const handle = setTimeout(async () => {
      try {
        const res = await api.searchConnectors(q)
        setRemoteResults(res.results ?? [])
        setRemoteWarnings(res.warnings ?? [])
      } catch {
        setRemoteResults([])
      } finally {
        setSearching(false)
      }
    }, 400)
    return () => clearTimeout(handle)
  }, [query, tab])

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

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return packs.filter((p) => {
      if (p.kind !== tab) return false
      if (!q) return true
      return (
        p.name.toLowerCase().includes(q) ||
        (p.description ?? '').toLowerCase().includes(q) ||
        (p.author ?? '').toLowerCase().includes(q)
      )
    })
  }, [packs, tab, query])

  const openDetail = useCallback(
    async (pack: Pack) => {
      setApiKey('')
      setPickedSecret('')
      setPreview(null)
      // A source-ref pack (directory-site result) isn't in the catalog and has no
      // downloadable payload — use it as-is and fetch its preview from GitHub.
      if (pack.sourceRef?.url) {
        setSelected(pack)
        setPreviewLoading(true)
        try {
          const res = await api.ingestPreview({ source: 'github', url: pack.sourceRef.url })
          setPreview(res.items)
        } catch {
          setPreview([])
        } finally {
          setPreviewLoading(false)
        }
        return
      }
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
        // A source-ref pack (directory-site result) installs by ingesting its GitHub
        // source, not by downloading a payload.
        if (pack.sourceRef?.url) {
          const res = await api.ingestInstall({ source: 'github', url: pack.sourceRef.url })
          setInstalled((prev) => new Set(prev).add(pack.id))
          onError(`✓ ${res.message}`)
          await loadExisting()
          onInstalled?.('skill')
          return
        }
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
            <input
              data-testid="market-search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Ara…"
              className="w-40 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]"
            />
            <button
              data-testid="market-import"
              onClick={() => setImporting(true)}
              title="GitHub repo / plugin veya yerel klasörden içe aktar (skill / agent / komut / MCP)"
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
            >
              <Download size={13} /> İçe Aktar
            </button>
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

          {/* Live directory-site search results (skill tab) */}
          {tab === 'skill' && (searching || remoteResults.length > 0 || remoteWarnings.length > 0) && (
            <div className="col-span-full mt-2 border-t border-[var(--color-border)] pt-3">
              <div className="mb-2 flex items-center gap-2 text-xs font-medium text-[var(--color-text-dim)]">
                <Globe2 size={13} />
                İnternet sonuçları (SkillsMP · CrossAITools)
                {searching && <span className="text-[var(--color-text-dim)]">aranıyor…</span>}
                {!searching && <span>· {remoteResults.length}</span>}
              </div>
              {remoteWarnings.length > 0 && (
                <p className="mb-2 text-[10px] text-[var(--color-warning,#d97706)]">{remoteWarnings.join(' · ')}</p>
              )}
            </div>
          )}
          {tab === 'skill' &&
            remoteResults.map((p) => {
              const done = installed.has(p.id)
              return (
                <button
                  key={p.id}
                  data-testid="market-remote-pack"
                  data-pack-id={p.id}
                  onClick={() => void openDetail(p)}
                  className={`flex flex-col gap-2 rounded-lg border p-3 text-left transition hover:border-[var(--color-accent)] ${
                    selected?.id === p.id ? 'border-[var(--color-accent)] bg-[var(--color-surface-2)]' : 'border-[var(--color-border)]'
                  }`}
                >
                  <div className="flex items-center gap-2">
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded text-lg bg-[var(--color-surface-2)]">
                      {p.icon || '🌐'}
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-medium">{p.name}</div>
                      <KindBadge kind={p.kind} />
                    </div>
                  </div>
                  <p className="line-clamp-3 text-xs text-[var(--color-text-dim)]">{p.description}</p>
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[9px] uppercase tracking-wide text-[var(--color-text-dim)]">
                      {p.registryName || 'Uzak'}
                    </span>
                    {done && (
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
            className="flex max-h-[88vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
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
            <PackMeta pack={selected} />
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

            {/* Payload preview — kind-specific, or a fetched GitHub preview for source-ref */}
            <div className="flex-1 overflow-y-auto p-4">
              {selected.sourceRef?.url ? (
                <SourceRefPreview url={selected.sourceRef.url} items={preview} loading={previewLoading} />
              ) : (
                <PackPreview pack={selected} prices={prices} />
              )}
            </div>
          </div>
        </div>
      )}

      {importing && (
        <SkillImportDialog
          onClose={() => setImporting(false)}
          onImported={(kinds) => {
            void loadExisting() // re-mark installed entities in the catalog
            kinds.forEach((k) => onInstalled?.(k)) // let the host refresh matching collections
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
