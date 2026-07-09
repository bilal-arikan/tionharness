// External tools settings panel. Detects optional CLI tools used alongside
// TionSwarm (token optimisation, dev, render — e.g. `sqz`, `crabbox`, `mmdc`) on
// PATH and lets the user wire the hook-based ones into TionSwarm with one click.
// Read-only detection — nothing is installed or executed. Self-contained (own
// load), exempt from the global Save bar — like the Hooks panel. Split out of
// HooksPanel into its own settings category.
import { useEffect, useState } from 'react'
import { ScanSearch } from 'lucide-react'
import { api } from '@/api'
import { systemApi } from '@/api/system'
import type { Hook, ExternalToolStatus, MCPServer } from '@/types'
import type { HookInput } from '@/api/hooks'
import { displayPath } from '@/shared/lib/paths'

// The external-tool name whose MCP integration is wired one-click from this panel.
// Its detected PATH entry doubles as the stdio command; the backend auto-routes it
// to the workspace's isolated CBM store (CBM_CACHE_DIR) whenever the command
// contains this marker, so no env needs to be supplied at creation time.
const CBM_TOOL = 'codebase-memory-mcp'

interface Props {
  onError: (msg: string) => void
}

// One-click hook templates that wire a detected external tool into TionSwarm as a
// PreToolUse/PostToolUse hook. Only tools whose `wire` is 'hook' (from
// /api/external-tools) need an entry here; 'mcp'/'cli' tools render an info badge
// instead of a toggle (driven by ExternalToolStatus.wire, not this map).
const TOOL_HOOK_TEMPLATES: Record<string, HookInput> = {
  // RTK rewrites shell commands. PreToolUse adapter prepends `rtk ` to the command
  // (preserving other tool_input fields) so output is filtered before it returns.
  // Matcher covers BOTH shell tools: the `shell` tool was split into `Bash` +
  // `PowerShell` (2026-07-01), and on Windows the agent uses `PowerShell` — a
  // `Bash`-only matcher would silently never fire. The comma-alt matcher is
  // honoured by `hookMatches` (filepath.Match has no brace expansion). The
  // adapter self-guards on `.tool_input.command`, so non-shell tools pass through.
  rtk: {
    event: 'PreToolUse',
    matcher: 'Bash,PowerShell',
    command:
      "$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; $c=$j.tool_input.command; if($c -and -not ($c -like 'rtk *')){ $j.tool_input.command='rtk '+$c; @{updatedInput=$j.tool_input}|ConvertTo-Json -Compress }",
    timeoutSec: 30,
    enabled: true,
  },
  // sqz (v1.3.0) `sqz hook claude` is a PreToolUse rewriter: it reads the tool-call
  // JSON from stdin and rewrites shell commands to pipe through sqz, then emits the
  // modified JSON — same model as rtk. sqz ignores tool_name and rewrites any
  // payload carrying `.command` (Bash + PowerShell alike), so the matcher covers
  // both. NOTE: do not enable rtk and sqz at the same time; they both rewrite.
  sqz: { event: 'PreToolUse', matcher: 'Bash,PowerShell', command: 'sqz hook claude', timeoutSec: 30, enabled: true },
}

// Human-readable group headings for the tool categories returned by the backend.
const TOOL_CATEGORY_LABELS: Record<string, string> = {
  token: 'Token / bağlam optimizasyonu',
  dev: 'Geliştirme araçları',
  render: 'Render / diyagram',
}

export function ExternalToolsPanel({ onError }: Props) {
  const [hooks, setHooks] = useState<Hook[]>([])
  const [tools, setTools] = useState<ExternalToolStatus[] | null>(null)
  const [checking, setChecking] = useState(false)
  const [toolsErr, setToolsErr] = useState<string | null>(null)
  // Per-tool busy flag while creating/toggling that tool's wired hook.
  const [toolBusy, setToolBusy] = useState<string | null>(null)
  // MCP servers — used to reflect whether codebase-memory-mcp is already wired and
  // to add/remove it in one click from its callout below the tool row.
  const [servers, setServers] = useState<MCPServer[]>([])
  const [mcpBusy, setMcpBusy] = useState(false)
  // Busy flag (keyed by hook id) while repairing a token-optimizer hook's matcher.
  const [fixBusy, setFixBusy] = useState<string | null>(null)

  const checkTools = async () => {
    setChecking(true)
    setToolsErr(null)
    try {
      setTools(await systemApi.externalTools())
    } catch (e) {
      setToolsErr(e instanceof Error ? e.message : String(e))
    } finally {
      setChecking(false)
    }
  }

  const loadHooks = () =>
    api
      .listHooks()
      .then(setHooks)
      .catch((e) => onError((e as Error).message))

  const loadServers = () =>
    api
      .listMCPServers()
      .then(setServers)
      .catch((e) => onError((e as Error).message))

  useEffect(() => {
    // Hooks are needed to show which detected tools are already wired; tools are
    // auto-detected on open (presence-only — nothing installed or executed).
    loadHooks()
    loadServers()
    checkTools()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // The hook (if any) currently wiring a given external tool into TionSwarm,
  // matched by the tool name appearing in the hook command.
  const wiredHook = (toolName: string): Hook | undefined =>
    hooks.find((h) => h.command.includes(toolName))

  // Create/enable/disable the hook for a detected tool with one click.
  const toggleTool = async (toolName: string) => {
    const tpl = TOOL_HOOK_TEMPLATES[toolName]
    if (!tpl) return
    setToolBusy(toolName)
    try {
      const existing = wiredHook(toolName)
      if (existing) await api.toggleHook(existing.id, !existing.enabled)
      else await api.createHook(tpl)
      await loadHooks()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setToolBusy(null)
    }
  }

  // coversPowerShell reports whether a hook matcher fires for the PowerShell tool.
  // Mirrors backend hookMatches: empty matcher (or `*`) matches everything; a
  // comma-separated list must name PowerShell explicitly.
  const coversPowerShell = (matcher: string): boolean => {
    const parts = matcher.split(',').map((s) => s.trim()).filter(Boolean)
    return parts.length === 0 || parts.includes('*') || parts.includes('PowerShell')
  }

  // Wired token-optimizer hooks whose matcher omits PowerShell — on Windows the
  // agent uses the PowerShell tool, so a Bash-only matcher means the hook silently
  // never fires. Surfaced with a one-click repair in the token callout.
  const tokenHooksNeedingFix = (): Hook[] =>
    Object.keys(TOOL_HOOK_TEMPLATES)
      .map((name) => wiredHook(name))
      .filter((h): h is Hook => !!h && !coversPowerShell(h.matcher))

  // Repair one hook's matcher: merge PowerShell into the existing matcher (or set
  // Bash,PowerShell when empty), preserving every other field.
  const fixMatcher = async (h: Hook) => {
    setFixBusy(h.id)
    try {
      const parts = h.matcher.split(',').map((s) => s.trim()).filter(Boolean)
      const merged = parts.length ? Array.from(new Set([...parts, 'PowerShell'])).join(',') : 'Bash,PowerShell'
      await api.updateHook(h.id, {
        event: h.event,
        matcher: merged,
        command: h.command,
        timeoutSec: h.timeoutSec,
        enabled: h.enabled,
      })
      await loadHooks()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setFixBusy(null)
    }
  }

  // The codebase-memory MCP server, if one is already registered (matched by its
  // command carrying the marker — same rule the backend uses to route the store).
  const cbmServer = (): MCPServer | undefined =>
    servers.find((s) => s.command.toLowerCase().includes(CBM_TOOL))

  // Add or remove the codebase-memory MCP server in one click. Adding uses the
  // detected PATH executable as the stdio command; removing deletes the matched
  // server. Requires the tool to be present on PATH (detectedPath).
  const toggleCbmServer = async (detectedPath: string) => {
    setMcpBusy(true)
    try {
      const existing = cbmServer()
      if (existing) await api.deleteMCPServer(existing.id)
      else await api.createMCPServer({ name: CBM_TOOL, transport: 'stdio', command: detectedPath })
      await loadServers()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setMcpBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        TionSwarm ile birlikte kullanılabilecek isteğe bağlı CLI araçlarının (token optimizasyonu, geliştirme, render — ör.{' '}
        <code>sqz</code>, <code>crabbox</code>, <code>mmdc</code>) bu cihazda{' '}
        <span className="font-medium text-[var(--color-text)]">kurulu olup olmadığını</span> kontrol eder.
        Yalnız PATH'te aranır — araçlar <span className="font-medium text-[var(--color-text)]">kurulmaz, çalıştırılmaz, değiştirilmez</span>.
        <span className="font-mono"> hook</span> araçları tek tıkla bağlanır;{' '}
        <span className="font-mono">mcp</span>/<span className="font-mono">cli</span> araçları bilgi rozetiyle gösterilir.
      </div>
      <button
        type="button"
        onClick={checkTools}
        disabled={checking}
        className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-50"
      >
        <ScanSearch size={14} className="text-[var(--color-accent)]" />
        {checking ? 'Kontrol ediliyor…' : 'Kurulu mu kontrol et'}
      </button>
      {toolsErr && <p className="text-xs text-[var(--color-warning)]">{toolsErr}</p>}
      {tools && (
        <div className="flex flex-col gap-3">
          {/* Ordered unique categories as they first appear from the backend. */}
          {tools
            .map((t) => t.category)
            .filter((c, i, arr) => arr.indexOf(c) === i)
            .map((cat) => (
              <div key={cat} className="flex flex-col gap-1.5">
                <p className="text-[11px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                  {TOOL_CATEGORY_LABELS[cat] ?? cat}
                </p>
                {tools
                  .filter((t) => t.category === cat)
                  .map((t) => (
                    <div key={t.name} className="flex flex-col gap-2">
                    <div className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] px-3 py-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2 text-sm">
                          <code className="rounded bg-[var(--color-surface-2)] px-1 font-medium">{t.name}</code>
                          {t.found ? (
                            <span className="text-[var(--color-success)]">✓ kurulu</span>
                          ) : (
                            <span className="text-[var(--color-text-dim)]">— bulunamadı</span>
                          )}
                        </div>
                        <div className="truncate text-xs text-[var(--color-text-dim)]">{t.found ? displayPath(t.path ?? '') : t.desc}</div>
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        {t.found && t.wire === 'hook' && TOOL_HOOK_TEMPLATES[t.name] && (
                          <button
                            data-testid="tool-toggle"
                            data-tool={t.name}
                            disabled={toolBusy === t.name}
                            onClick={() => toggleTool(t.name)}
                            className={`rounded px-2 py-0.5 text-xs disabled:opacity-50 ${
                              wiredHook(t.name)?.enabled
                                ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                                : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                            }`}
                            title={
                              wiredHook(t.name)
                                ? 'TionSwarm hook bağlantısını aç/kapat'
                                : 'Bu araç için TionSwarm hook’u oluştur ve etkinleştir'
                            }
                          >
                            {toolBusy === t.name
                              ? '…'
                              : wiredHook(t.name)?.enabled
                                ? 'Aktif'
                                : wiredHook(t.name)
                                  ? 'Pasif'
                                  : 'Bağla'}
                          </button>
                        )}
                        {t.found && t.wire === 'mcp' && (
                          <span
                            className="rounded px-1.5 py-0.5 font-mono text-[10px] bg-[var(--color-surface-2)] text-[var(--color-text-dim)]"
                            title="MCP tabanlı — Ayarlar ▸ MCP'den eklenir, hook değil"
                          >
                            MCP
                          </span>
                        )}
                        {t.found && t.wire === 'cli' && (
                          <span
                            className="rounded px-1.5 py-0.5 font-mono text-[10px] bg-[var(--color-surface-2)] text-[var(--color-text-dim)]"
                            title="Düz CLI — ajan geliştirmede Bash ile doğrudan çağırır"
                          >
                            CLI
                          </span>
                        )}
                        <a href={t.url} target="_blank" rel="noreferrer" className="text-xs text-[var(--color-accent)] hover:underline">repo ↗</a>
                      </div>
                    </div>
                    {/* codebase-memory integration: explanation + one-click MCP wiring,
                        anchored directly under its own tool row. */}
                    {t.name === CBM_TOOL && (
                      <div className="rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
                        <span className="font-medium text-[var(--color-text)]">🧠 codebase-memory entegrasyonu:</span> Bir{' '}
                        <code>codebase-memory-mcp</code> sunucusu eklendiğinde, TionSwarm otomatik olarak{' '}
                        ajanın bağlamına <span className="font-medium text-[var(--color-text)]">"kod bilgi-grafiği mevcut"</span> ipucu ekler,
                        sunucuyu <span className="font-medium text-[var(--color-text)]">workspace'e özel izole bir store</span>'a yönlendirir
                        (indeksler workspace'ler arası karışmaz), çalışma dizinini otomatik indeksler ve{' '}
                        <code>codebase_workspace_search</code> aracını sunar. Bu sistem{' '}
                        <span className="font-medium text-[var(--color-text)]">Ayarlar ▸ Bu Workspace</span> altından açılıp kapatılabilir.
                        <div className="mt-2 flex items-center gap-2 border-t border-[color-mix(in_srgb,var(--color-accent)_20%,transparent)] pt-2">
                          <button
                            type="button"
                            data-testid="cbm-mcp-toggle"
                            disabled={mcpBusy || !t.found}
                            onClick={() => toggleCbmServer(t.path ?? t.name)}
                            className={`rounded px-2.5 py-1 text-xs font-medium disabled:opacity-50 ${
                              cbmServer()
                                ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                                : 'bg-[var(--color-accent)] text-white hover:opacity-90'
                            }`}
                            title={
                              !t.found
                                ? 'Önce bu araç PATH’te bulunmalı (yukarıda "kurulu" görünmeli)'
                                : cbmServer()
                                  ? 'MCP sunucusunu kaldır'
                                  : 'MCP sunucusunu otomatik ekle (izole store’a yönlenir)'
                            }
                          >
                            {mcpBusy ? '…' : cbmServer() ? 'MCP’yi kaldır' : 'MCP’yi otomatik ekle'}
                          </button>
                          <span className="text-[11px] text-[var(--color-text-dim)]">
                            {!t.found
                              ? 'Araç PATH’te bulunamadı.'
                              : cbmServer()
                                ? 'MCP sunucusu ekli — ajanlar kullanabilir.'
                                : 'MCP sunucusu ekli değil.'}
                          </span>
                        </div>
                      </div>
                    )}
                    </div>
                  ))}
                {/* Token-optimizer integration: mirrors the codebase-memory callout —
                    explains the prompt-side capability injection and flags any wired
                    hook whose matcher omits PowerShell (so it would never fire on
                    Windows), with a one-click repair. Rendered once under the group. */}
                {cat === 'token' && (
                  <div className="rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
                    <span className="font-medium text-[var(--color-text)]">⚡ Token optimizasyonu entegrasyonu:</span> Bir{' '}
                    <code>rtk</code> / <code>sqz</code> aracı hook olarak bağlandığında, TionSwarm ajanın bağlamına{' '}
                    <span className="font-medium text-[var(--color-text)]">"token optimizasyonu aktif"</span> bilgi bloğu ekler
                    (codebase-memory entegrasyonu gibi) — böylece ajan araç çıktısının otomatik kısaltıldığını{' '}
                    <span className="font-medium text-[var(--color-text)]">(kayıp değil)</span> bilir ve komutlardan çekinmez.
                    Hook <span className="font-medium text-[var(--color-text)]">matcher</span>'ı{' '}
                    <code>Bash,PowerShell</code> olmalı; yalnız <code>Bash</code> ise Windows'ta ajanın kullandığı{' '}
                    <code>PowerShell</code> aracında <span className="font-medium text-[var(--color-text)]">hiç ateşlenmez</span>.
                    rtk ve sqz'yi aynı anda açma (ikisi de komutu yeniden yazar).
                    {tokenHooksNeedingFix().length > 0 && (
                      <div className="mt-2 flex flex-col gap-1.5 border-t border-[color-mix(in_srgb,var(--color-accent)_20%,transparent)] pt-2">
                        {tokenHooksNeedingFix().map((h) => (
                          <div key={h.id} className="flex items-center justify-between gap-2">
                            <span className="min-w-0 truncate text-[var(--color-warning)]">
                              ⚠ matcher <code>{h.matcher}</code> — PowerShell kapsamıyor
                            </span>
                            <button
                              type="button"
                              data-testid="fix-matcher"
                              data-hook={h.id}
                              disabled={fixBusy === h.id}
                              onClick={() => fixMatcher(h)}
                              className="shrink-0 rounded bg-[var(--color-accent)] px-2 py-0.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
                              title="Matcher'a PowerShell ekle (Bash,PowerShell)"
                            >
                              {fixBusy === h.id ? '…' : 'Matcher’ı düzelt'}
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
        </div>
      )}
    </div>
  )
}
