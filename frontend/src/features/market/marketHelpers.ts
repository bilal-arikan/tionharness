import {
  Sparkles,
  Users,
  Plug,
  GitBranch,
  Boxes,
  Wrench,
  Bot,
  Zap,
  Timer,
  Webhook,
  Play,
  Square,
  Repeat,
  MessageCircleQuestion,
  Workflow,
  Rocket,
  Merge,
  type LucideIcon,
} from 'lucide-react'
import type { Pack, PackKind, WorkspaceTemplateFlow } from '@/types'
import { i18next } from '@/i18n'

// updateAvailable reports whether a pack's catalog version is newer than the
// version last installed here (best-effort dotted-numeric comparison).
export function updateAvailable(pack: Pack): boolean {
  if (!pack.installedVersion || !pack.version) return false
  return compareVersions(pack.version, pack.installedVersion) > 0
}

// compareVersions: -1 if a<b, 0 if equal, 1 if a>b. Non-numeric segments → 0;
// any pre-release/build suffix after '-'/'+' is ignored. Mirrors the backend.
function compareVersions(a: string, b: string): number {
  const parts = (v: string) =>
    v
      .trim()
      .replace(/^v/, '')
      .split(/[-+]/)[0]
      .split('.')
      .map((s) => parseInt(s, 10) || 0)
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

// Kind filter entries shown as a left sidebar (no "all" option — one kind is
// always selected, defaulting to the first). Each maps to a market pack kind.
// Workspaces lead because they are the only kind that ships bundled packs, so a
// fresh install opens on a non-empty catalog.
export const KIND_NAV: { key: PackKind; icon: LucideIcon }[] = [
  { key: 'workspace', icon: Boxes },
  { key: 'skill', icon: Sparkles },
  { key: 'agent', icon: Users },
  { key: 'provider', icon: Plug },
  { key: 'flow', icon: GitBranch },
  { key: 'mcp', icon: Wrench },
  { key: 'hook', icon: Webhook },
]

export const packKindKey = (kind: PackKind) => `kind.${kind}` as const
export const packKindPluralKey = (kind: PackKind) => `kindPlural.${kind}` as const

export const installLabelKey = (kind: PackKind) => `install.${kind}` as const

// cacheLabel turns a provider's promptCache mode into a human label for the badge.
export function cacheLabel(mode?: string): string {
  switch (mode) {
    case 'native':
      return i18next.t('preview.cache.native', { ns: 'market' })
    case 'auto':
      return i18next.t('preview.cache.auto', { ns: 'market' })
    case 'none':
      return i18next.t('preview.cache.none', { ns: 'market' })
    default:
      return i18next.t('preview.cache.unknown', { ns: 'market' })
  }
}

// fmtPrice formats a USD/1M-token figure compactly (e.g. "$0.30", "$15").
export function fmtPrice(n: number): string {
  if (n === 0) return i18next.t('preview.free', { ns: 'market' })
  return '$' + (n < 1 ? n.toFixed(2).replace(/0+$/, '').replace(/\.$/, '') : String(n))
}

// NODE_ICON maps an orchestration node type to a monochrome lucide glyph for the
// flow-chain preview (replaces colored emojis). Covers every engine node type;
// an unknown type falls back to a bullet in the preview.
export const NODE_ICON: Record<string, LucideIcon> = {
  start: Play,
  end: Square,
  agent: Bot,
  branch: GitBranch,
  parallel: Zap,
  delay: Timer,
  transform: Wrench,
  loop: Repeat,
  'await-input': MessageCircleQuestion,
  subflow: Workflow,
  spawn: Rocket,
  join: Merge,
}

// SOURCE_LABEL labels which tier/source a pack came from.
export function sourceLabel(source: string): string {
  const icons: Record<string, string> = { bundled: '📦', global: '💾', remote: '🌐' }
  const translated = i18next.t(`source.${source}`, { ns: 'market', defaultValue: source })
  return icons[source] ? `${icons[source]} ${translated}` : translated
}

// flowSummary returns a flow's node list for the preview, from either its full
// graph JSON or its linear steps.
export function flowSummary(
  flow: WorkspaceTemplateFlow,
): { id: string; type: string; title?: string }[] {
  if (flow.graph) return flowNodeSummary(flow.graph)
  if (flow.steps) return flow.steps.map((s) => ({ id: s.id, type: 'agent', title: s.title }))
  return []
}

// skillMeta parses a skill's SKILL.md frontmatter for its name/description.
export function skillMeta(body: string): { name?: string; description?: string } {
  const m = /^---\s*\n([\s\S]*?)\n---/.exec(body)
  if (!m) return {}
  const fm = m[1]
  const name = /(?:^|\n)name:\s*"?(.+?)"?\s*(?:\n|$)/.exec(fm)?.[1]
  const description = /(?:^|\n)description:\s*"?(.+?)"?\s*(?:\n|$)/.exec(fm)?.[1]
  return { name, description }
}

// flowNodeSummary safely parses a flow graph JSON string into a node list for
// the preview. Returns [] on any parse error.
export function flowNodeSummary(graph: string): { id: string; type: string; title?: string }[] {
  try {
    const g = JSON.parse(graph) as { nodes?: { id: string; type: string; title?: string }[] }
    return g.nodes ?? []
  } catch {
    return []
  }
}

// existingKeys holds, per kind, the identifiers of entities already present in
// the workspace, so the market can mark a pack as already installed and block a
// duplicate. Keys: skill→slug, agent→lowercased name, flow→lowercased name,
// provider→id, workspace→lowercased name, mcp→lowercased name. hook has no
// identity the installer dedups on, so it is never marked installed.
export interface ExistingKeys {
  skills: Set<string>
  agents: Set<string>
  flows: Set<string>
  providers: Set<string>
  workspaces: Set<string>
  mcp: Set<string>
}

export const emptyExisting = (): ExistingKeys => ({
  skills: new Set(),
  agents: new Set(),
  flows: new Set(),
  providers: new Set(),
  workspaces: new Set(),
  mcp: new Set(),
})

// packTargetKey returns the identifier a pack would occupy once installed, in
// the same shape as ExistingKeys. hook returns null: the installer creates a hook
// unconditionally (no name/slug to dedup on), so the catalog must not claim it is
// already installed.
export function packTargetKey(pack: Pack): { set: keyof ExistingKeys; key: string } | null {
  switch (pack.kind) {
    case 'hook':
      return null
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
  }
}

// stripFrontmatter removes a leading --- delimited block for the preview so the
// reader sees the instructions, not the YAML header.
export function stripFrontmatter(text: string): string {
  if (!text.startsWith('---')) return text
  const end = text.indexOf('\n---', 3)
  if (end < 0) return text
  return text.slice(text.indexOf('\n', end + 1) + 1).trimStart()
}
