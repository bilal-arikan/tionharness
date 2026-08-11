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
  ArrowUpCircle,
  Bot,
  Brain,
  FolderCog,
  Plug,
  Wrench,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import { systemApi } from '@/api/system'
import type { HookInput } from '@/api/hooks'
import type {
  AppSettings,
  ExternalToolStatus,
  ExternalToolUpdate,
  Hook,
  MCPServer,
  WorkspaceSettings,
} from '@/types'

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
// There is deliberately no RTK_HOOK. The template that used to live here was a
// PowerShell one-liner; claude-cli runs hooks through BASH, so it failed with
// `syntax error near unexpected token '|'`, and a failing PreToolUse hook BLOCKS
// the tool call — clicking this card bricked Bash for the whole workspace
// (2026-07-31, WS10/SES63). rtk is now wired by the shellCommandRewrite SETTING,
// which additionally limits rewriting to the commands measured to benefit.

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
  /**
   * Upstream release check per tool. The ONLY part of the probe that leaves the
   * machine, so it is allowed to come back empty (offline, GitHub rate-limited)
   * — rules that read it must treat empty as "no opinion", never as "all current".
   */
  updates: ExternalToolUpdate[]
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
  // Both optimizers active used to be flagged as a conflict ("ikisi de komutu
  // yeniden yazıyor — birini kapat"). That was wrong: they act at OPPOSITE ends
  // (rtk reshapes the command, sqz compresses the output) and measurement shows
  // stacking beats either alone — `git log -30`: 6595 raw → sqz 2027 → rtk 2157 →
  // rtk+sqz 1167 tokens. The card told users to disable their best configuration,
  // so it is gone rather than reworded.
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
        desc: 'codebase-memory-mcp kurulu ama bu workspace’e eklenmemiş. MCP olarak eklersen kod arama/gezinme sub-ms ve düşük token olur.',
        actionLabel: 'MCP’yi ekle',
        act: async () => {
          await api.createMCPServer({
            name: CBM_TOOL,
            transport: 'stdio',
            command: cbm.path ?? cbm.name,
          })
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
      summary: 'sqz kuruluyken çıktı sıkıştırma hook’u bağlı değilse.',
    },
    detect: (ctx) => {
      // Only sqz is offered here now. rtk is wired by a workspace SETTING, not a
      // hook (see the note on RTK_HOOK's removal above), and it has its own toggle
      // in Settings ▸ External Tools.
      const sqz = ctx.tools.find((t) => t.name === 'sqz' && t.found)
      if (!sqz || tokenHookLive(ctx.hooks, 'sqz')) return null
      return {
        desc: 'sqz bu cihazda kurulu ama bağlı değil. Hook olarak bağlarsan araç çıktıları kayıpsız sıkışır, token tasarrufu sağlar.',
        actionLabel: 'sqz’i bağla',
        act: async () => {
          await api.createHook(SQZ_HOOK)
        },
      }
    },
  },
  {
    meta: {
      key: 'shell-compress',
      icon: Zap,
      title: 'Shell çıktısı sıkıştırmayı aç',
      summary:
        'sqz kuruluyken shell çıktısı in-process sıkıştırma pasifse (hook yok, otomatik devre dışı).',
    },
    detect: (ctx) => {
      const sqz = ctx.tools.find((t) => t.name === 'sqz' && t.found)
      if (!sqz) return null
      // 'on' zaten zorluyor; 'off' kullanıcının açık tercihi — ikisine de dokunma.
      if (ctx.ws.shellOutputCompression === 'on' || ctx.ws.shellOutputCompression === 'off')
        return null
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
      key: 'tool-update',
      icon: ArrowUpCircle,
      title: 'Harici araç güncellemesi var',
      summary: 'Kurulu bir harici aracın daha yeni bir sürümü yayımlanmışsa.',
    },
    detect: (ctx) => {
      // Only 'outdated' counts. 'unknown' means a version could not be parsed on
      // one side or the other — nudging an upgrade on that would be a guess, and
      // for the manual tools a wrong guess costs the user a risky binary swap.
      const stale = ctx.updates.filter((u) => u.status === 'outdated')
      if (stale.length === 0) return null
      const oneClick = stale.filter(
        (u) => ctx.tools.find((t) => t.name === u.name)?.updateKind === 'command',
      ).length
      const names = stale.map((u) => `${u.name} → ${u.latest}`).join(', ')
      return {
        desc:
          `${names} yayımlanmış. ` +
          (oneClick > 0
            ? `${oneClick} tanesi tek tıkla güncellenebilir; kalanlar elle (çalışan alt-süreç ikiliyi kilitlediği için TionSwarm üzerine yazmaz).`
            : 'Bu araçlar elle güncellenir — çalışan bir alt-süreç ikiliyi kilitlediği için TionSwarm üzerine yazmaz; ekranda adım adım talimat var.'),
        actionLabel: 'Harici araçlar',
        variant: 'warning',
        act: () => ctx.nav.settings('exttools'),
      }
    },
  },
  {
    meta: {
      key: 'cli-tools',
      icon: Wrench,
      title: 'Kurulu CLI araçları var',
      summary: 'PATH’te doğrudan Bash ile çağrılabilecek CLI araçları (mmdc…) varsa.',
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
  const [tools, servers, hooks, ws, settings, agents, updates] = await Promise.all([
    systemApi.externalTools(),
    api.listMCPServers(),
    api.listHooks(),
    api.getWorkspaceSettings(),
    api.getSettings(),
    api.listAgents(),
    // The update check is the one probe that hits the network (GitHub, via the
    // backend's 6h cache — so this is usually a cache read, not a request). It is
    // caught HERE rather than left to reject the Promise.all: an offline machine
    // must not wipe out every other recommendation just because this one could
    // not be answered. Empty means "no opinion", and the rule then returns null.
    systemApi.checkExternalToolUpdates().catch(() => [] as ExternalToolUpdate[]),
  ])
  return { tools, servers, hooks, ws, settings, agentsCount: agents.length, updates }
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
