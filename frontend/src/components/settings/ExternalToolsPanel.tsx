// External tools settings panel. Detects optional CLI tools used alongside
// SwarmGo (token optimisation, dev, render — e.g. `sqz`, `crabbox`, `mmdc`) on
// PATH and lets the user wire the hook-based ones into SwarmGo with one click.
// Read-only detection — nothing is installed or executed. Self-contained (own
// load), exempt from the global Save bar — like the Hooks panel. Split out of
// HooksPanel into its own settings category.
import { useEffect, useState } from 'react'
import { ScanSearch } from 'lucide-react'
import { api } from '../../api'
import { systemApi } from '../../api/system'
import type { Hook, ExternalToolStatus } from '../../types'
import type { HookInput } from '../../api/hooks'
import { displayPath } from '../../lib/paths'

interface Props {
  onError: (msg: string) => void
}

// One-click hook templates that wire a detected external tool into SwarmGo as a
// PreToolUse/PostToolUse hook. Only tools whose `wire` is 'hook' (from
// /api/external-tools) need an entry here; 'mcp'/'cli' tools render an info badge
// instead of a toggle (driven by ExternalToolStatus.wire, not this map).
const TOOL_HOOK_TEMPLATES: Record<string, HookInput> = {
  // RTK rewrites Bash commands. PreToolUse adapter prepends `rtk ` to the command
  // (preserving other tool_input fields) so output is filtered before it returns.
  rtk: {
    event: 'PreToolUse',
    matcher: 'Bash',
    command:
      "$j=[Console]::In.ReadToEnd()|ConvertFrom-Json; $c=$j.tool_input.command; if($c -and -not ($c -like 'rtk *')){ $j.tool_input.command='rtk '+$c; @{updatedInput=$j.tool_input}|ConvertTo-Json -Compress }",
    timeoutSec: 30,
    enabled: true,
  },
  // sqz (v1.3.0) `sqz hook claude` is a PreToolUse rewriter: it reads the tool-call
  // JSON from stdin and rewrites Bash commands to pipe through sqz, then emits the
  // modified JSON — same model as rtk, so matcher is Bash. NOTE: do not enable rtk
  // and sqz on Bash at the same time; they both rewrite the command.
  sqz: { event: 'PreToolUse', matcher: 'Bash', command: 'sqz hook claude', timeoutSec: 30, enabled: true },
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

  useEffect(() => {
    // Hooks are needed to show which detected tools are already wired; tools are
    // auto-detected on open (presence-only — nothing installed or executed).
    loadHooks()
    checkTools()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // The hook (if any) currently wiring a given external tool into SwarmGo,
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

  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        SwarmGo ile birlikte kullanılabilecek isteğe bağlı CLI araçlarının (token optimizasyonu, geliştirme, render — ör.{' '}
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
                    <div key={t.name} className="flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] px-3 py-2">
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
                                ? 'SwarmGo hook bağlantısını aç/kapat'
                                : 'Bu araç için SwarmGo hook’u oluştur ve etkinleştir'
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
                  ))}
              </div>
            ))}
        </div>
      )}
    </div>
  )
}
