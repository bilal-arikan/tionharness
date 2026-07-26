// Shared catalog + engine for the post-create workspace recommendations. Split out
// of the popup so both surfaces use ONE source of truth:
//   - WorkspaceRecommendations.tsx — the dismissible toast shown once per create.
//   - RecommendationsPanel.tsx      — the "Öneriler" workspace tab that lists every
//                                     rule, its live state, and its ignore toggle.
//
// Architecture: a flat RULES array. Each rule carries static `meta` (key/icon/title/
// summary — enough to list it with no probe) plus `detect(ctx)` which inspects the
// probed context and returns the dynamic card parts (desc/action/act/variant) or null
// when it does not apply. Adding a recommendation = one RULES entry.
import {
  Archive,
  Bot,
  Brain,
  FolderCog,
  Plug,
  TriangleAlert,
  Wrench,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import { systemApi } from '@/api/system'
import type { HookInput } from '@/api/hooks'
import type { AppSettings, ExternalToolStatus, Hook, MCPServer, WorkspaceSettings } from '@/types'

// Marker matched against an MCP server's command to tell whether codebase-memory is
// already wired — the same rule the backend uses to route it to the isolated store.
export const CBM_TOOL = 'codebase-memory-mcp'

// Hook templates identical to ExternalToolsPanel's, so wiring from here produces the
// exact same hooks the settings panel would.
const SQZ_HOOK: HookInput = {
  event: 'PreToolUse',
  matcher: 'Bash,PowerShell',
  command: 'sqz hook claude',
  timeoutSec: 30,
  enabled: true,
}
const RTK_HOOK: HookInput = {
  event: 'PreToolUse',
  matcher: 'Bash,PowerShell',
  command:
    "$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; $c=$j.tool_input.command; if($c -and -not ($c -like 'rtk *')){ $j.tool_input.command='rtk '+$c; @{updatedInput=$j.tool_input}|ConvertTo-Json -Compress }",
  timeoutSec: 30,
  enabled: true,
}

export type RecVariant = 'accent' | 'warning'

// Navigation helpers a rule's action may use.
export interface RecNav {
  view: (v: string) => void
  settings: (cat: string) => void
}

// Everything a rule inspects: the probe results plus navigation helpers.
export interface RecContext {
  tools: ExternalToolStatus[]
  servers: MCPServer[]
  hooks: Hook[]
  ws: WorkspaceSettings
  settings: AppSettings
  agentsCount: number
  nav: RecNav
}

// Static, probe-free metadata so a rule can be listed (settings panel) without a ctx.
export interface RecMeta {
  key: string
  icon: LucideIcon
  title: string
  summary: string
}

// The dynamic parts a rule produces when it applies (merged with meta into a Rec).
interface RecBody {
  desc: string
  actionLabel: string
  variant?: RecVariant
  act: () => void | Promise<void>
}

// A fully-built recommendation card.
export interface Rec extends RecMeta, RecBody {}

export interface Rule {
  meta: RecMeta
  detect: (ctx: RecContext) => RecBody | null
}

// Whether any enabled hook wires a given token optimizer (rtk/sqz).
const tokenHookLive = (hooks: Hook[], marker: string) =>
  hooks.some((h) => h.enabled && h.command.includes(marker))

// RULES run in this order; cards stack top-to-bottom, so warnings and the biggest
// gaps come first. Each rule is independent — no cross-rule state.
export const RULES: Rule[] = [
  {
    meta: {
      key: 'token-conflict',
      icon: TriangleAlert,
      title: 'İki token aracı birden aktif',
      summary: 'rtk ve sqz aynı anda hook olarak aktifse ikisi de komutu yeniden yazar, çakışabilir.',
    },
    detect: (ctx) => {
      if (!(tokenHookLive(ctx.hooks, 'rtk') && tokenHookLive(ctx.hooks, 'sqz'))) return null
      return {
        variant: 'warning',
        desc: 'rtk ve sqz ikisi de komutu yeniden yazıyor — çakışabilir. Birini kapatmalısın.',
        actionLabel: 'Harici araçları aç',
        act: () => ctx.nav.settings('exttools'),
      }
    },
  },
  {
    meta: {
      key: 'no-agents',
      icon: Bot,
      title: 'Henüz ajan yok',
      summary: 'Workspace’te hiç ajan yoksa oturumları yürütecek bir aktör yoktur.',
    },
    detect: (ctx) => {
      if (ctx.agentsCount > 0) return null
      return {
        desc: 'Bu workspace boş. Bir ajan oluştur ya da market’ten hazır bir şablon kur.',
        actionLabel: 'Ajanlara git',
        act: () => ctx.nav.view('agents'),
      }
    },
  },
  {
    meta: {
      key: 'workdir',
      icon: FolderCog,
      title: 'Çalışma dizini belirle',
      summary: 'Varsayılan çalışma dizini yoksa oturumlar fiziksel workspace dizininde başlar.',
    },
    detect: (ctx) => {
      if (ctx.ws.defaultWorkingDir?.trim()) return null
      return {
        desc: 'Bir varsayılan çalışma dizini seçersen oturumlar o kök + git dalı + CLAUDE.md bağlamıyla başlar.',
        actionLabel: 'Ayarları aç',
        act: () => ctx.nav.view('workspace'),
      }
    },
  },
  {
    meta: {
      key: 'cbm-add',
      icon: Brain,
      title: 'Kod bilgi-grafiğini ekle',
      summary: 'codebase-memory-mcp PATH’te kuruluysa ama MCP olarak eklenmemişse.',
    },
    detect: (ctx) => {
      const cbm = ctx.tools.find((t) => t.name === CBM_TOOL && t.found)
      const added = ctx.servers.some((s) => s.command.toLowerCase().includes(CBM_TOOL))
      if (!cbm || added) return null
      return {
        desc: 'codebase-memory-mcp kurulu ama bu workspace’e eklenmemiş. MCP olarak eklersen kod arama/gezinme sub-ms ve düşük token olur; izole store’a yönlenir.',
        actionLabel: 'MCP’yi ekle',
        act: async () => {
          await api.createMCPServer({ name: CBM_TOOL, transport: 'stdio', command: cbm.path ?? cbm.name })
        },
      }
    },
  },
  {
    meta: {
      key: 'cbm-enable',
      icon: Brain,
      title: 'Kod bilgi-grafiğini aç',
      summary: 'codebase-memory MCP ekliyken workspace toggle’ı kapalıysa.',
    },
    detect: (ctx) => {
      const added = ctx.servers.some((s) => s.command.toLowerCase().includes(CBM_TOOL))
      if (!added || ctx.ws.codebaseMemoryEnabled) return null
      return {
        desc: 'codebase-memory MCP ekli ama bu workspace’te kapalı. Açarsan otomatik indeksleme + codebase_workspace_search devreye girer.',
        actionLabel: 'Aç',
        act: async () => {
          await api.updateWorkspaceSettings({ codebaseMemoryEnabled: true })
        },
      }
    },
  },
  {
    meta: {
      key: 'token',
      icon: Zap,
      title: 'Token optimizasyonu bağla',
      summary: 'rtk/sqz kuruluyken hiçbir token-optimizer hook’u bağlı değilse.',
    },
    detect: (ctx) => {
      const rtk = ctx.tools.find((t) => t.name === 'rtk' && t.found)
      const sqz = ctx.tools.find((t) => t.name === 'sqz' && t.found)
      const live = tokenHookLive(ctx.hooks, 'rtk') || tokenHookLive(ctx.hooks, 'sqz')
      const pick = rtk ?? sqz
      if (!pick || live) return null
      const isRtk = pick.name === 'rtk'
      return {
        desc: `${pick.name} bu cihazda kurulu ama bağlı değil. Hook olarak bağlarsan araç çıktıları kayıpsız sıkışır, token tasarrufu sağlar.`,
        actionLabel: `${pick.name}’i bağla`,
        act: async () => {
          await api.createHook(isRtk ? RTK_HOOK : SQZ_HOOK)
        },
      }
    },
  },
  {
    meta: {
      key: 'shell-compress',
      icon: Zap,
      title: 'Shell çıktısı sıkıştırmayı aç',
      summary: 'sqz kuruluyken shell çıktısı in-process sıkıştırma pasifse (hook yok, otomatik devre dışı).',
    },
    detect: (ctx) => {
      const sqz = ctx.tools.find((t) => t.name === 'sqz' && t.found)
      if (!sqz) return null
      // 'on' zaten zorluyor; 'off' kullanıcının açık tercihi — ikisine de dokunma.
      if (ctx.ws.shellOutputCompression === 'on' || ctx.ws.shellOutputCompression === 'off') return null
      // Otomatik mod yalnız bir sqz hook bağlıyken aktiftir; bağlıysa sıkıştırma zaten çalışıyor.
      if (tokenHookLive(ctx.hooks, 'sqz')) return null
      return {
        desc: 'sqz kurulu ama bu workspace’te shell çıktısı sıkıştırma pasif. Açarsan ajanın shell çıktısı modele dönmeden in-process sqz ile kayıpsız kısaltılır — hook gerektirmez (hook yolu bridged araçta zaten çalışmaz).',
        actionLabel: 'Aç',
        act: async () => {
          await api.updateWorkspaceSettings({ shellOutputCompression: 'on' })
        },
      }
    },
  },
  {
    meta: {
      key: 'no-mcp',
      icon: Plug,
      title: 'MCP kaynağı yok',
      summary: 'Hiç MCP sunucusu yoksa (ve codebase-memory zaten önerilmiyorsa).',
    },
    detect: (ctx) => {
      if (ctx.servers.length > 0) return null
      const cbmPending = ctx.tools.some((t) => t.name === CBM_TOOL && t.found)
      if (cbmPending) return null
      return {
        desc: 'Harici veri/araç bağlamak için bir MCP sunucusu ekleyebilirsin (market veya .mcp.json import).',
        actionLabel: 'Market’i aç',
        act: () => ctx.nav.view('market'),
      }
    },
  },
  {
    meta: {
      key: 'backup-off',
      icon: Archive,
      title: 'Yedekleme kapalı',
      summary: 'Uygulama genelinde periyodik zip yedeği kapalıysa.',
    },
    detect: (ctx) => {
      if (ctx.settings.backupEnabled) return null
      return {
        desc: 'Periyodik zip yedeği kapalı. Açarsan her workspace’in verisi düzenli yedeklenir.',
        actionLabel: 'Yedekleme ayarları',
        act: () => ctx.nav.settings('backup'),
      }
    },
  },
  {
    meta: {
      key: 'cli-tools',
      icon: Wrench,
      title: 'Kurulu CLI araçları var',
      summary: 'PATH’te doğrudan Bash ile çağrılabilecek CLI araçları (mmdc/crabbox…) varsa.',
    },
    detect: (ctx) => {
      const cli = ctx.tools.filter((t) => t.found && t.wire === 'cli')
      if (cli.length === 0) return null
      return {
        desc: `${cli.map((t) => t.name).join(', ')} bu cihazda kurulu — ajan geliştirmede Bash ile doğrudan çağırabilir.`,
        actionLabel: 'Detay',
        act: () => ctx.nav.settings('exttools'),
      }
    },
  },
]

// fetchRecommendationData probes everything the rules inspect (minus nav, which the
// caller supplies). One place so both surfaces issue the same requests.
export async function fetchRecommendationData(): Promise<Omit<RecContext, 'nav'>> {
  const [tools, servers, hooks, ws, settings, agents] = await Promise.all([
    systemApi.externalTools(),
    api.listMCPServers(),
    api.listHooks(),
    api.getWorkspaceSettings(),
    api.getSettings(),
    api.listAgents(),
  ])
  return { tools, servers, hooks, ws, settings, agentsCount: agents.length }
}

// runRules evaluates every rule against ctx and returns the applicable cards.
export function runRules(ctx: RecContext): Rec[] {
  const out: Rec[] = []
  for (const rule of RULES) {
    const body = rule.detect(ctx)
    if (body) out.push({ ...rule.meta, ...body })
  }
  return out
}

// A no-op nav for surfaces that only need applicability (never invoke a card action),
// e.g. the settings panel deciding whether a rule currently applies.
export const NOOP_NAV: RecNav = { view: () => {}, settings: () => {} }
