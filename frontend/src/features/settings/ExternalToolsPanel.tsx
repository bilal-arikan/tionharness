// External tools settings panel. Detects optional CLI tools used alongside
// TionSwarm (token optimisation, dev, render — e.g. `sqz`, `mmdc`, `piper`),
// reports the version each one has installed, checks that against the latest
// published release, and lets the user wire the hook-based ones into TionSwarm
// with one click. Self-contained (own load), exempt from the global Save bar —
// like the Hooks panel. Split out of HooksPanel into its own settings category.
//
// Detection is local and instant, so it runs on open. The update CHECK leaves
// the machine (GitHub API) and is therefore explicit — a button, never automatic.
import { useEffect, useState } from 'react'
import { ScanSearch, Eraser, FileCog, RefreshCw, ArrowUpCircle, Copy } from 'lucide-react'
import { api } from '@/api'
import { systemApi } from '@/api/system'
import type {
  Hook,
  ExternalToolStatus,
  ExternalToolUpdate,
  MCPServer,
  TokenToolReport,
  WorkspaceSettings,
} from '@/types'
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
// NOTE — there is deliberately NO `rtk` entry here any more.
//
// It used to ship a PreToolUse hook whose body was a PowerShell one-liner
// (`$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; …`). TionSwarm's own hook runner
// is PowerShell on Windows, so it worked on the native path — but claude-cli runs
// hooks through BASH, which died on the first `|` with
// `syntax error near unexpected token '|'`. A failing PreToolUse hook BLOCKS the
// tool call, so clicking "Bağla" made every Bash call in the workspace fail
// (observed 2026-07-31 in WS10/SES63, where the agent gave up and switched to
// PowerShell).
//
// It is not replaced with a bash-safe rewrite, because the hook was also the wrong
// mechanism: it prefixed `rtk ` onto EVERY command, including `git diff` and `cat`
// where rtk measurably loses to sqz. rtk is now wired by the ShellCommandRewrite
// setting (`wire: 'setting'`), which applies the measured allowlist and the
// Degraded guard on both the native and bridged paths. See _Docs/17.
const TOOL_HOOK_TEMPLATES: Record<string, HookInput> = {
  // sqz (v1.3.0) `sqz hook claude` is a PreToolUse rewriter: it reads the tool-call
  // JSON from stdin and rewrites shell commands to pipe through sqz, then emits the
  // modified JSON. sqz ignores tool_name and rewrites any payload carrying
  // `.command` (Bash + PowerShell alike), so the matcher covers both. Unlike the
  // removed rtk template this is a plain command word, valid in bash and PowerShell
  // alike, so it survives whichever runner executes it.
  sqz: {
    event: 'PreToolUse',
    matcher: 'Bash,PowerShell',
    command: 'sqz hook claude',
    timeoutSec: 30,
    enabled: true,
  },
}

// Human-readable group headings for the tool categories returned by the backend.
const TOOL_CATEGORY_LABELS: Record<string, string> = {
  provider: 'LLM sağlayıcı CLI’ları',
  token: 'Token / bağlam optimizasyonu',
  dev: 'Geliştirme araçları',
  render: 'Render / diyagram',
  voice: 'Ses (TTS / STT)',
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
  // Token-optimizer maintenance: each tool's own savings report + rtk config path.
  const [report, setReport] = useState<TokenToolReport | null>(null)
  const [maintBusy, setMaintBusy] = useState<string | null>(null)
  const [maintMsg, setMaintMsg] = useState<string | null>(null)

  // Workspace settings, for tools wired by a setting rather than a hook (rtk).
  const [ws, setWs] = useState<WorkspaceSettings | null>(null)

  // Upstream release check, keyed by tool name. Empty until the user asks for
  // it — the check hits GitHub, so it never runs on open.
  const [updates, setUpdates] = useState<Record<string, ExternalToolUpdate>>({})
  const [checkingUpdates, setCheckingUpdates] = useState(false)
  const [updateBusy, setUpdateBusy] = useState<string | null>(null)
  // Output of the last update run, kept next to the button: a package-manager
  // install is a long, noisy operation the user needs to be able to read.
  const [updateLog, setUpdateLog] = useState<{ name: string; ok: boolean; text: string } | null>(
    null,
  )
  const [copied, setCopied] = useState<string | null>(null)

  // Ask the backend to compare each installed tool with its latest release.
  // `refresh` bypasses the 6h server cache — offered so a user who just released
  // or updated something is not stuck looking at a stale answer.
  const checkUpdates = async (refresh = false) => {
    setCheckingUpdates(true)
    try {
      const rows = await systemApi.checkExternalToolUpdates(refresh)
      setUpdates(Object.fromEntries(rows.map((r) => [r.name, r])))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setCheckingUpdates(false)
    }
  }

  // Run one tool's update command, then re-read the installed versions so the
  // chip reflects reality rather than what we hoped happened.
  const runUpdate = async (t: ExternalToolStatus) => {
    setUpdateBusy(t.name)
    setUpdateLog(null)
    try {
      const res = await systemApi.updateExternalTool(t.name)
      setUpdateLog({ name: t.name, ok: res.ok, text: res.output?.trim() || '(çıktı yok)' })
      await checkTools()
      await checkUpdates(true)
    } catch (e) {
      setUpdateLog({ name: t.name, ok: false, text: (e as Error).message })
    } finally {
      setUpdateBusy(null)
    }
  }

  const copyCommand = async (name: string, cmd: string) => {
    try {
      await navigator.clipboard.writeText(cmd)
      setCopied(name)
      setTimeout(() => setCopied(null), 1500)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const loadWs = () =>
    api
      .getWorkspaceSettings()
      .then(setWs)
      .catch((e) => onError((e as Error).message))

  // Toggle the setting that wires a `wire: 'setting'` tool. Tri-state on disk
  // ('' = auto), but from here it is a plain on/off: '' would mean "follow hook
  // detection", and the whole point of this row is that rtk no longer HAS a hook.
  const toggleSettingTool = async (toolName: string) => {
    if (toolName !== 'rtk' || !ws) return
    setToolBusy(toolName)
    try {
      setWs(
        await api.updateWorkspaceSettings({
          shellCommandRewrite: ws.shellCommandRewrite === 'on' ? 'off' : 'on',
        }),
      )
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setToolBusy(null)
    }
  }

  const loadReport = () =>
    systemApi
      .tokenToolReport()
      .then(setReport)
      .catch((e) => onError((e as Error).message))

  // Run one maintenance action, keeping its outcome next to the buttons rather
  // than as a toast: these are diagnostics the user reads, not fire-and-forget.
  const runMaint = async (key: string, fn: () => Promise<unknown>, done: (r: never) => string) => {
    setMaintBusy(key)
    setMaintMsg(null)
    try {
      const res = await fn()
      setMaintMsg(done(res as never))
      if (key === 'sqz-reset') await loadReport()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setMaintBusy(null)
    }
  }

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
    loadReport()
    loadWs()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Whether to offer the one-click "Güncelle" button for a tool.
  //
  // The rule is deliberately strict: a package-manager install runs ONLY when we
  // can point at a concrete reason — a release feed that says this exact tool is
  // behind. Tools with no feed (ffmpeg, npm, node, python, and git off Windows)
  // therefore NEVER show the button, because their status can only ever be
  // 'unknown' and "I don't know" is not grounds for mutating the user's machine.
  //
  // This used to be an emergent side effect of the inline `status === 'outdated'`
  // check; naming it makes the intent explicit so a future change to the status
  // logic cannot silently start offering unjustified updates. The copy-command
  // chip below the row stays as the manual escape hatch for exactly these tools.
  const canOfferUpdate = (t: ExternalToolStatus): boolean =>
    t.found && t.updateKind === 'command' && updates[t.name]?.status === 'outdated'

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
    const parts = matcher
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
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
      const parts = h.matcher
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
      const merged = parts.length
        ? Array.from(new Set([...parts, 'PowerShell'])).join(',')
        : 'Bash,PowerShell'
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
        TionSwarm ile birlikte kullanılabilecek isteğe bağlı CLI araçlarının (token optimizasyonu,
        geliştirme, render — ör. <code>sqz</code>, <code>mmdc</code>, <code>piper</code>) bu cihazda{' '}
        <span className="font-medium text-[var(--color-text)]">kurulu olup olmadığını</span> kontrol
        eder. Önce Ayarlar'daki yol geçersiz kılması (varsa), yoksa PATH aranır — araçlar{' '}
        <span className="font-medium text-[var(--color-text)]">
          kurulmaz, çalıştırılmaz, değiştirilmez
        </span>
        .<span className="font-mono"> hook</span> araçları tek tıkla bağlanır;{' '}
        <span className="font-mono">mcp</span>/<span className="font-mono">cli</span> araçları bilgi
        rozetiyle gösterilir. Kurulu sürüm aracın kendi <code>--version</code> çıktısından okunur.
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={checkTools}
          disabled={checking}
          className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-50"
        >
          <ScanSearch size={14} className="text-[var(--color-accent)]" />
          {checking ? 'Kontrol ediliyor…' : 'Kurulu mu kontrol et'}
        </button>
        <button
          type="button"
          data-testid="ext-tools-check-updates"
          onClick={() => checkUpdates(false)}
          disabled={checkingUpdates}
          title="Kurulu araçları GitHub'daki en son yayımlanmış sürümle karşılaştırır. Sonuç sunucuda 6 saat önbelleklenir (GitHub API limiti saatte 60 istek)."
          className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-50"
        >
          <ArrowUpCircle size={14} className="text-[var(--color-accent)]" />
          {checkingUpdates ? 'Sürümler alınıyor…' : 'Güncellemeleri kontrol et'}
        </button>
        {Object.keys(updates).length > 0 && (
          <button
            type="button"
            data-testid="ext-tools-refresh-updates"
            onClick={() => checkUpdates(true)}
            disabled={checkingUpdates}
            title="Önbelleği atlayıp GitHub'a yeniden sor"
            className="inline-flex items-center gap-1 rounded px-2 py-1 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)] disabled:opacity-50"
          >
            <RefreshCw size={11} />
            Önbelleği atla
          </button>
        )}
      </div>
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
                            <code className="rounded bg-[var(--color-surface-2)] px-1 font-medium">
                              {t.name}
                            </code>
                            {t.found ? (
                              <span className="text-[var(--color-success)]">✓ kurulu</span>
                            ) : (
                              <span className="text-[var(--color-text-dim)]">— bulunamadı</span>
                            )}
                            {t.found && t.version && (
                              <span
                                data-testid="tool-version"
                                data-tool={t.name}
                                className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-text-dim)]"
                              >
                                v{t.version}
                              </span>
                            )}
                            {t.found && !t.version && t.versionError && (
                              <span
                                className="font-mono text-[10px] text-[var(--color-text-dim)]"
                                title={t.versionError}
                              >
                                sürüm okunamadı
                              </span>
                            )}
                            {/* Update verdict. Only ever rendered from a real
                              comparison — 'unknown' shows nothing rather than an
                              ambiguous badge the user would have to decode. */}
                            {t.found && updates[t.name]?.status === 'outdated' && (
                              <a
                                data-testid="tool-outdated"
                                data-tool={t.name}
                                href={updates[t.name].releaseUrl ?? t.url}
                                target="_blank"
                                rel="noreferrer"
                                title={`En son yayımlanan sürüm: ${updates[t.name].latest}${
                                  updates[t.name].stale
                                    ? ' (önbellekten — GitHub’a ulaşılamadı)'
                                    : ''
                                }`}
                                className="rounded bg-[var(--color-warning-soft,var(--color-surface-2))] px-1.5 py-0.5 font-mono text-[10px] text-[var(--color-warning)] hover:underline"
                              >
                                ↑ {updates[t.name].latest}
                              </a>
                            )}
                            {t.found && updates[t.name]?.status === 'up-to-date' && (
                              <span
                                className="font-mono text-[10px] text-[var(--color-success)]"
                                title="En son sürüm"
                              >
                                güncel
                              </span>
                            )}
                            {t.found && updates[t.name]?.error && (
                              <span
                                className="font-mono text-[10px] text-[var(--color-text-dim)]"
                                title={updates[t.name].error}
                              >
                                sürüm karşılaştırılamadı
                              </span>
                            )}
                          </div>
                          <div className="truncate text-xs text-[var(--color-text-dim)]">
                            {t.found ? displayPath(t.path ?? '') : t.desc}
                          </div>
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
                          {t.found && t.wire === 'setting' && (
                            <button
                              data-testid="tool-setting-toggle"
                              data-tool={t.name}
                              disabled={toolBusy === t.name || !ws}
                              onClick={() => toggleSettingTool(t.name)}
                              className={`rounded px-2 py-0.5 text-xs disabled:opacity-50 ${
                                ws?.shellCommandRewrite === 'on'
                                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                              }`}
                              title="Bu araç hook ile değil, workspace ayarıyla bağlanır (Ayarlar ▸ Workspace ▸ Shell komutu yeniden yazma). Yalnız ölçülmüş kazanç veren test/build komutları yeniden yazılır."
                            >
                              {toolBusy === t.name
                                ? '…'
                                : ws?.shellCommandRewrite === 'on'
                                  ? 'Aktif'
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
                          {t.found && t.wire === 'provider' && (
                            <span
                              className="rounded px-1.5 py-0.5 font-mono text-[10px] bg-[var(--color-surface-2)] text-[var(--color-text-dim)]"
                              title="Bir LLM sağlayıcısını çalıştırır — Ayarlar ▸ Sağlayıcılar'dan yapılandırılır, hook değil"
                            >
                              Sağlayıcı
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
                          {/* One-click update — see canOfferUpdate for the rule.
                            Package-manager-backed AND proven behind by a release
                            feed. For manual-kind tools the backend answers 409 by
                            design; see the callout below the row. */}
                          {canOfferUpdate(t) && (
                            <button
                              type="button"
                              data-testid="tool-update"
                              data-tool={t.name}
                              disabled={updateBusy === t.name}
                              onClick={() => runUpdate(t)}
                              title={`Çalıştırılacak komut: ${t.updateCommand}`}
                              className="rounded bg-[var(--color-accent)] px-2 py-0.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-50"
                            >
                              {updateBusy === t.name ? 'Güncelleniyor…' : 'Güncelle'}
                            </button>
                          )}
                          <a
                            href={t.url}
                            target="_blank"
                            rel="noreferrer"
                            className="text-xs text-[var(--color-accent)] hover:underline"
                          >
                            repo ↗
                          </a>
                        </div>
                      </div>
                      {/* Manual upgrade instructions, shown only once an update is
                        actually available. TionSwarm will not overwrite these
                        binaries itself: on Windows a running child (an MCP stdio
                        server holding its own .exe, a piper synth mid-render)
                        locks the file, and a half-applied copy leaves a broken
                        tool with no way back. */}
                      {t.found &&
                        updates[t.name]?.status === 'outdated' &&
                        t.updateKind === 'manual' && (
                          <div
                            data-testid="tool-manual-update"
                            data-tool={t.name}
                            className="rounded-lg border border-[color-mix(in_srgb,var(--color-warning)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_6%,transparent)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]"
                          >
                            <span className="font-medium text-[var(--color-text)]">
                              ⬆ {updates[t.name].latest} yayımlanmış — elle güncellenir.
                            </span>{' '}
                            {t.updateNote}{' '}
                            <a
                              href={updates[t.name].releaseUrl ?? t.url}
                              target="_blank"
                              rel="noreferrer"
                              className="text-[var(--color-accent)] hover:underline"
                            >
                              release sayfası ↗
                            </a>
                          </div>
                        )}
                      {/* Update run output, anchored under the tool it belongs to. */}
                      {updateLog?.name === t.name && (
                        <div className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-xs">
                          <div
                            className={
                              updateLog.ok
                                ? 'text-[var(--color-success)]'
                                : 'text-[var(--color-warning)]'
                            }
                          >
                            {updateLog.ok ? '✓ Güncelleme tamamlandı' : '✕ Güncelleme başarısız'}
                          </div>
                          <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2 text-[11px] text-[var(--color-text-dim)]">
                            {updateLog.text}
                          </pre>
                        </div>
                      )}
                      {/* The exact command, so a user who would rather run it in
                        their own terminal (or has no such package manager) is not
                        left guessing what the button would have done. */}
                      {t.found && t.updateKind === 'command' && t.updateCommand && (
                        <button
                          type="button"
                          data-testid="tool-copy-update-cmd"
                          data-tool={t.name}
                          onClick={() => copyCommand(t.name, t.updateCommand ?? '')}
                          className="inline-flex w-fit items-center gap-1 rounded px-1 text-[11px] text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
                          title="Güncelleme komutunu panoya kopyala"
                        >
                          <Copy size={10} />
                          <code>{t.updateCommand}</code>
                          {copied === t.name && (
                            <span className="text-[var(--color-success)]">kopyalandı</span>
                          )}
                        </button>
                      )}
                      {/* codebase-memory integration: explanation + one-click MCP wiring,
                        anchored directly under its own tool row. */}
                      {t.name === CBM_TOOL && (
                        <div className="rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
                          <span className="font-medium text-[var(--color-text)]">
                            🧠 codebase-memory entegrasyonu:
                          </span>{' '}
                          Bir <code>codebase-memory-mcp</code> sunucusu eklendiğinde, TionSwarm
                          otomatik olarak ajanın bağlamına{' '}
                          <span className="font-medium text-[var(--color-text)]">
                            "kod bilgi-grafiği mevcut"
                          </span>{' '}
                          ipucu ekler, sunucuyu{' '}
                          <span className="font-medium text-[var(--color-text)]">
                            workspace'e özel izole bir store
                          </span>
                          'a yönlendirir (indeksler workspace'ler arası karışmaz), çalışma dizinini
                          otomatik indeksler ve <code>codebase_workspace_search</code> aracını
                          sunar. Bu sistem{' '}
                          <span className="font-medium text-[var(--color-text)]">
                            Ayarlar ▸ Bu Workspace
                          </span>{' '}
                          altından açılıp kapatılabilir.
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
                              {mcpBusy
                                ? '…'
                                : cbmServer()
                                  ? 'MCP’yi kaldır'
                                  : 'MCP’yi otomatik ekle'}
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
                    <span className="font-medium text-[var(--color-text)]">
                      ⚡ Token optimizasyonu entegrasyonu:
                    </span>{' '}
                    Bir <code>rtk</code> / <code>sqz</code> aracı hook olarak bağlandığında,
                    TionSwarm ajanın bağlamına{' '}
                    <span className="font-medium text-[var(--color-text)]">
                      "token optimizasyonu aktif"
                    </span>{' '}
                    bilgi bloğu ekler (codebase-memory entegrasyonu gibi) — böylece ajan araç
                    çıktısının otomatik kısaltıldığını{' '}
                    <span className="font-medium text-[var(--color-text)]">(kayıp değil)</span>{' '}
                    bilir ve komutlardan çekinmez. Hook{' '}
                    <span className="font-medium text-[var(--color-text)]">matcher</span>'ı{' '}
                    <code>Bash,PowerShell</code> olmalı; yalnız <code>Bash</code> ise Windows'ta
                    ajanın kullandığı <code>PowerShell</code> aracında{' '}
                    <span className="font-medium text-[var(--color-text)]">hiç ateşlenmez</span>.
                    {/* This used to warn "do not enable rtk and sqz together — both rewrite
                        the command". Measurement showed the opposite: they act at opposite
                        ends and stacking wins (git log -30: 6595 ham → sqz 2027 → rtk 2157
                        → rtk+sqz 1167 token). The old text steered users away from their
                        best configuration. */}
                    <span className="font-medium text-[var(--color-text)]">
                      {' '}
                      İkisini birden açmak önerilir
                    </span>{' '}
                    — rtk komutu <span className="italic">çalışmadan önce</span> şekillendirir, sqz
                    çıktıyı <span className="italic">sonra</span> sıkıştırır; ölçümde istifleme her
                    ikisinden de iyi çıktı.
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
                {/* Maintenance ACTIONS for the token optimizers. Deliberately not
                    settings: rtk/sqz config is machine-global while this screen is
                    workspace-scoped, so mirroring their keys here would promise a
                    scope the setting cannot honour. What IS offered: the report the
                    tools already produce, the reset sqz itself prescribes, and a
                    door to rtk's config where it actually lives. */}
                {cat === 'token' && report && (report.rtkFound || report.sqzFound) && (
                  <div className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-medium text-[var(--color-text)]">
                        🔧 Bakım ve tasarruf raporu
                      </span>
                      <button
                        type="button"
                        data-testid="token-report-refresh"
                        disabled={maintBusy === 'refresh'}
                        onClick={() => runMaint('refresh', loadReport, () => 'Rapor yenilendi.')}
                        className="inline-flex items-center gap-1 rounded bg-[var(--color-surface-2)] px-2 py-0.5 text-[11px] hover:text-[var(--color-text)] disabled:opacity-50"
                        title="rtk gain / sqz gain çıktısını yeniden al"
                      >
                        <RefreshCw size={11} />
                        {maintBusy === 'refresh' ? '…' : 'Yenile'}
                      </button>
                    </div>
                    <p className="mt-1">
                      Aşağıdaki rakamlar{' '}
                      <span className="font-medium text-[var(--color-text)]">araçların kendi</span>{' '}
                      <code>gain</code> çıktısıdır — TionSwarm yeniden hesaplamaz, böylece araçların
                      muhasebesinden sapamaz.
                    </p>
                    {(['rtk', 'sqz'] as const).map((name) => {
                      const found = name === 'rtk' ? report.rtkFound : report.sqzFound
                      const gain = name === 'rtk' ? report.rtkGain : report.sqzGain
                      if (!found) return null
                      return (
                        <div key={name} className="mt-2">
                          <div className="mb-1 font-mono text-[10px] uppercase tracking-wide">
                            {name} gain
                          </div>
                          <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2 text-[11px]">
                            {gain?.trim() ? gain : '(rapor boş — henüz veri yok)'}
                          </pre>
                        </div>
                      )
                    })}
                    <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-2">
                      {report.sqzFound && (
                        <button
                          type="button"
                          data-testid="sqz-reset-cache"
                          disabled={maintBusy === 'sqz-reset'}
                          onClick={() =>
                            runMaint(
                              'sqz-reset',
                              systemApi.sqzResetCache,
                              () => 'sqz dedup önbelleği temizlendi.',
                            )
                          }
                          className="inline-flex items-center gap-1.5 rounded bg-[var(--color-surface-2)] px-2.5 py-1 font-medium hover:text-[var(--color-text)] disabled:opacity-50"
                          title="sqz'nin dedup önbelleğini temizler (istatistikler korunur). Bayat §ref:…§ işaretçileri ajanı şaşırttığında sqz'nin kendi önerdiği işlem."
                        >
                          <Eraser size={12} />
                          {maintBusy === 'sqz-reset' ? '…' : 'sqz dedup önbelleğini temizle'}
                        </button>
                      )}
                      {report.rtkFound && report.rtkConfigPath && (
                        <button
                          type="button"
                          data-testid="rtk-config-reveal"
                          disabled={maintBusy === 'rtk-config'}
                          onClick={() =>
                            runMaint(
                              'rtk-config',
                              systemApi.revealRtkConfig,
                              () => 'rtk config klasörü açıldı.',
                            )
                          }
                          className="inline-flex items-center gap-1.5 rounded bg-[var(--color-surface-2)] px-2.5 py-1 font-medium hover:text-[var(--color-text)] disabled:opacity-50"
                          title={report.rtkConfigPath}
                        >
                          <FileCog size={12} />
                          {maintBusy === 'rtk-config' ? '…' : 'rtk config dosyasını aç'}
                        </button>
                      )}
                      {maintMsg && <span className="text-[var(--color-success)]">{maintMsg}</span>}
                    </div>
                    {report.rtkFound && report.rtkConfigPath && (
                      <p className="mt-1.5 text-[11px]">
                        <code className="break-all">{displayPath(report.rtkConfigPath)}</code>
                        {!report.rtkConfigExists && (
                          <>
                            {' '}
                            — dosya{' '}
                            <span className="font-medium text-[var(--color-text)]">henüz yok</span>;
                            rtk yerleşik varsayılanlarla çalışıyor. Oluşturmak için terminalde{' '}
                            <code>rtk config --create</code>.
                          </>
                        )}
                        <br />
                        Bu dosya{' '}
                        <span className="font-medium text-[var(--color-text)]">
                          makine geneli
                        </span>{' '}
                        — workspace'e özel değil. Bu yüzden anahtarları buraya ayar olarak
                        taşınmadı: burada değiştirdiğin şey diğer tüm workspace'leri de etkilerdi.
                      </p>
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
