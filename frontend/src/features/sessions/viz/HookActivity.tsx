import { useMemo } from 'react'
import type { Hook, SessionDebugEvent } from '@/types'

// HookActivity summarizes a session's tool/lifecycle HOOK firings from the debug
// journal (type=hook events, each carrying the firing hook's id). Its first
// purpose is to make token-optimizer activity visible: rtk / sqz are wired as
// PreToolUse hooks that rewrite commands, so "how often did rtk/sqz actually run,
// and on which tool" is the honest signal available WITHOUT a savings measurement
// (TionHarness never sees the pre-compression size — see _Docs/17). Byte/token
// savings are intentionally NOT shown; this is activity, not savings.
//
// Coverage caveat: hook debug events are emitted by the NATIVE tool loop. On
// claude-cli-delegated turns the CLI runs the hooks itself, so those firings are
// not journaled here — the panel notes this so the count is not misread as total.

type OptKind = 'rtk' | 'sqz' | 'hook'

const KIND_META: Record<OptKind, { label: string; cls: string }> = {
  // Every hue maps onto a theme token (success/info/text-dim) so the badges
  // re-theme with presets and stay legible in the light theme.
  rtk: {
    label: 'rtk',
    cls: 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
  },
  sqz: {
    label: 'sqz',
    cls: 'bg-[color-mix(in_srgb,var(--color-info)_15%,transparent)] text-[var(--color-info)]',
  },
  hook: {
    label: 'hook',
    cls: 'bg-[color-mix(in_srgb,var(--color-text-dim)_15%,transparent)] text-[var(--color-text-dim)]',
  },
}

// classifyHook maps a hook command to a token-optimizer kind via the same
// word-bounded markers the backend capability probe uses (\brtk\b / \bsqz\b), so
// the label never fires on substrings like "quirtky"/"sqzip".
function classifyHook(command: string | undefined): OptKind {
  const c = (command ?? '').toLowerCase()
  if (/\brtk\b/.test(c)) return 'rtk'
  if (/\bsqz\b/.test(c)) return 'sqz'
  return 'hook'
}

interface HookGroup {
  hookId: string
  kind: OptKind
  fired: number
  errors: number
  byTool: Record<string, number> // tool name -> firings (parsed from "tool:decision" detail)
}

function buildGroups(events: SessionDebugEvent[], hooks: Hook[]): HookGroup[] {
  const byId = new Map<string, Hook>()
  for (const h of hooks) byId.set(h.id, h)

  const groups = new Map<string, HookGroup>()
  for (const ev of events) {
    if (ev.type !== 'hook') continue
    const hookId = ev.hookId ?? '' // '' = unattributed (pre-upgrade or CLI-path events)
    let g = groups.get(hookId)
    if (!g) {
      g = { hookId, kind: classifyHook(byId.get(hookId)?.command), fired: 0, errors: 0, byTool: {} }
      groups.set(hookId, g)
    }
    g.fired++
    if (ev.err) g.errors++
    // PreToolUse/PostToolUse detail is "tool:decision"; the tool is the part
    // before the first colon. Lifecycle-hook detail carries no tool prefix.
    const detail = ev.detail ?? ''
    const colon = detail.indexOf(':')
    if (colon > 0 && (ev.name === 'PreToolUse' || ev.name === 'PostToolUse')) {
      const tool = detail.slice(0, colon)
      g.byTool[tool] = (g.byTool[tool] ?? 0) + 1
    }
  }
  // Token-optimizers first (rtk, sqz), then other hooks; most-fired within a tier.
  const order: Record<OptKind, number> = { rtk: 0, sqz: 1, hook: 2 }
  return [...groups.values()].sort((a, b) => order[a.kind] - order[b.kind] || b.fired - a.fired)
}

export function HookActivity({ events, hooks }: { events: SessionDebugEvent[]; hooks: Hook[] }) {
  const groups = useMemo(() => buildGroups(events, hooks), [events, hooks])
  const hasOptimizer = groups.some((g) => g.kind === 'rtk' || g.kind === 'sqz')

  if (groups.length === 0) {
    return (
      <p className="text-[10px] text-[var(--color-text-dim)]">
        Bu oturumda (native turlarda) hook ateşlemesi yok. Not: claude-cli turlarında hook'lar CLI
        içinde çalışır ve buraya işlenmez.
      </p>
    )
  }

  return (
    <div>
      <ul className="space-y-1">
        {groups.map((g) => {
          const meta = KIND_META[g.kind]
          const tools = Object.entries(g.byTool).sort((a, b) => b[1] - a[1])
          return (
            <li
              key={g.hookId || 'unattributed'}
              className="flex flex-wrap items-center gap-1.5 text-[10px] leading-snug"
            >
              <span className={`shrink-0 rounded px-1.5 py-px font-medium ${meta.cls}`}>
                {meta.label}
              </span>
              <span className="text-[var(--color-text-dim)]">{g.hookId || 'atıfsız'}</span>
              <span className="text-[var(--color-text)]">{g.fired} ateşleme</span>
              {tools.length > 0 && (
                <span className="text-[var(--color-text-dim)]">
                  · {tools.map(([t, n]) => `${t}×${n}`).join(' ')}
                </span>
              )}
              {g.errors > 0 && (
                <span className="text-[var(--color-danger)]">· {g.errors} hata</span>
              )}
            </li>
          )
        })}
      </ul>
      <p className="mt-1.5 text-[10px] text-[var(--color-text-dim)]">
        {hasOptimizer
          ? 'rtk/sqz komutu yeniden yazarak çıktıyı küçültür; TionHarness sıkışmamış boyutu görmediği için burada byte tasarrufu değil, yalnız aktivite gösterilir.'
          : "Token-optimizer (rtk/sqz) hook'u bu oturumda ateşlenmedi."}{' '}
        Yalnız native turlar sayılır (claude-cli turları CLI içinde çalışır).
      </p>
    </div>
  )
}
