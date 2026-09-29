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
  Scissors,
  Search,
  Wrench,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { api } from '@/api'
import { systemApi } from '@/api/system'
import { i18next } from '@/i18n'
import type { HookInput } from '@/api/hooks'
import type {
  AppSettings,
  ExternalToolStatus,
  ExternalToolUpdate,
  Hook,
  MCPServer,
  WorkspaceSettings,
} from '@/types'
import {
  ZVEC_GREP_SERVER_ARGS,
  ZVEC_GREP_SERVER_NAME,
  ZVEC_GREP_TOOL,
  isZvecGrepServer,
} from '@/shared/lib/zvecGrep'

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

type RecVariant = 'accent' | 'warning'

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
interface RecMeta {
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

// Rule metadata is read by both the settings list and the post-create cards.
// Getters resolve at access time so a language switch never leaves module-level
// labels frozen in the locale that happened to load this file first.
const recText = (key: string, options?: Record<string, unknown>): string =>
  i18next.t(key, { ns: 'workspace', ...options })

const ruleMeta = (key: string, icon: LucideIcon): RecMeta => ({
  key,
  icon,
  get title() {
    return recText(`recommendations.rules.${key}.title`)
  },
  get summary() {
    return recText(`recommendations.rules.${key}.summary`)
  },
})

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
    meta: ruleMeta('no-agents', Bot),
    detect: (ctx) => {
      if (ctx.agentsCount > 0) return null
      return {
        desc: recText('recommendations.rules.no-agents.desc'),
        actionLabel: recText('recommendations.rules.no-agents.action'),
        act: () => ctx.nav.view('agents'),
      }
    },
  },
  {
    meta: ruleMeta('workdir', FolderCog),
    detect: (ctx) => {
      if (ctx.ws.defaultWorkingDir?.trim()) return null
      return {
        desc: recText('recommendations.rules.workdir.desc'),
        actionLabel: recText('recommendations.rules.workdir.action'),
        act: () => ctx.nav.view('workspace'),
      }
    },
  },
  {
    meta: ruleMeta('cbm-add', Brain),
    detect: (ctx) => {
      const cbm = ctx.tools.find((t) => t.name === CBM_TOOL && t.found)
      const added = ctx.servers.some((s) => s.command.toLowerCase().includes(CBM_TOOL))
      if (!cbm || added) return null
      return {
        desc: recText('recommendations.rules.cbm-add.desc'),
        actionLabel: recText('recommendations.rules.cbm-add.action'),
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
    meta: ruleMeta('cbm-enable', Brain),
    detect: (ctx) => {
      const added = ctx.servers.some((s) => s.command.toLowerCase().includes(CBM_TOOL))
      if (!added || ctx.ws.codebaseMemoryEnabled) return null
      return {
        desc: recText('recommendations.rules.cbm-enable.desc'),
        actionLabel: recText('recommendations.rules.cbm-enable.action'),
        act: async () => {
          await api.updateWorkspaceSettings({ codebaseMemoryEnabled: true })
        },
      }
    },
  },
  {
    meta: ruleMeta('zvec-add', Search),
    detect: (ctx) => {
      const zg = ctx.tools.find((t) => t.name === ZVEC_GREP_TOOL && t.found)
      if (!zg || ctx.servers.some(isZvecGrepServer)) return null
      return {
        desc: recText('recommendations.rules.zvec-add.desc'),
        actionLabel: recText('recommendations.rules.zvec-add.action'),
        act: async () => {
          await api.createMCPServer({
            name: ZVEC_GREP_SERVER_NAME,
            transport: 'stdio',
            command: zg.path ?? zg.name,
            args: [...ZVEC_GREP_SERVER_ARGS],
          })
        },
      }
    },
  },
  {
    meta: ruleMeta('zvec-enable', Search),
    detect: (ctx) => {
      if (!ctx.servers.some(isZvecGrepServer) || ctx.ws.zvecGrepEnabled) return null
      return {
        desc: recText('recommendations.rules.zvec-enable.desc'),
        actionLabel: recText('recommendations.rules.zvec-enable.action'),
        act: async () => {
          await api.updateWorkspaceSettings({ zvecGrepEnabled: true })
        },
      }
    },
  },
  {
    meta: ruleMeta('token', Zap),
    detect: (ctx) => {
      // Only sqz is offered here now. rtk is wired by a workspace SETTING, not a
      // hook (see the note on RTK_HOOK's removal above), and it has its own toggle
      // in Settings ▸ External Tools.
      const sqz = ctx.tools.find((t) => t.name === 'sqz' && t.found)
      if (!sqz || tokenHookLive(ctx.hooks, 'sqz')) return null
      return {
        desc: recText('recommendations.rules.token.desc'),
        actionLabel: recText('recommendations.rules.token.action'),
        act: async () => {
          await api.createHook(SQZ_HOOK)
        },
      }
    },
  },
  {
    meta: ruleMeta('shell-compress', Zap),
    detect: (ctx) => {
      const sqz = ctx.tools.find((t) => t.name === 'sqz' && t.found)
      if (!sqz) return null
      // 'on' zaten zorluyor; 'off' kullanıcının açık tercihi — ikisine de dokunma.
      if (ctx.ws.shellOutputCompression === 'on' || ctx.ws.shellOutputCompression === 'off')
        return null
      // Otomatik mod yalnız bir sqz hook bağlıyken aktiftir; bağlıysa sıkıştırma zaten çalışıyor.
      if (tokenHookLive(ctx.hooks, 'sqz')) return null
      return {
        desc: recText('recommendations.rules.shell-compress.desc'),
        actionLabel: recText('recommendations.rules.shell-compress.action'),
        act: async () => {
          await api.updateWorkspaceSettings({ shellOutputCompression: 'on' })
        },
      }
    },
  },
  {
    meta: ruleMeta('terse-mode', Scissors),
    detect: (ctx) => {
      if (ctx.ws.terseMode) return null
      return {
        desc: recText('recommendations.rules.terse-mode.desc'),
        actionLabel: recText('recommendations.rules.terse-mode.action'),
        act: async () => {
          await api.updateWorkspaceSettings({ terseMode: true })
        },
      }
    },
  },
  {
    meta: ruleMeta('no-mcp', Plug),
    detect: (ctx) => {
      if (ctx.servers.length > 0) return null
      // A specific "add this MCP" card (codebase-memory, zvec-grep) already covers
      // the empty list; a second, generic card about it would just stack.
      const specificPending = ctx.tools.some(
        (t) => (t.name === CBM_TOOL || t.name === ZVEC_GREP_TOOL) && t.found,
      )
      if (specificPending) return null
      return {
        desc: recText('recommendations.rules.no-mcp.desc'),
        actionLabel: recText('recommendations.rules.no-mcp.action'),
        act: () => ctx.nav.view('market'),
      }
    },
  },
  {
    meta: ruleMeta('backup-off', Archive),
    detect: (ctx) => {
      if (ctx.settings.backupEnabled) return null
      return {
        desc: recText('recommendations.rules.backup-off.desc'),
        actionLabel: recText('recommendations.rules.backup-off.action'),
        act: () => ctx.nav.settings('backup'),
      }
    },
  },
  {
    meta: ruleMeta('tool-update', ArrowUpCircle),
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
          recText('recommendations.rules.tool-update.published', { names }) +
          (oneClick > 0
            ? recText('recommendations.rules.tool-update.oneClick', { count: oneClick })
            : recText('recommendations.rules.tool-update.manual')),
        actionLabel: recText('recommendations.rules.tool-update.action'),
        variant: 'warning',
        act: () => ctx.nav.settings('exttools'),
      }
    },
  },
  {
    meta: ruleMeta('cli-tools', Wrench),
    detect: (ctx) => {
      const cli = ctx.tools.filter((t) => t.found && t.wire === 'cli')
      if (cli.length === 0) return null
      return {
        desc: recText('recommendations.rules.cli-tools.desc', {
          tools: cli.map((t) => t.name).join(', '),
        }),
        actionLabel: recText('recommendations.rules.cli-tools.action'),
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
