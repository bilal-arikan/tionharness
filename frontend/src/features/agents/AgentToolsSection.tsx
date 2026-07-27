import { useCallback, useEffect, useMemo, useState } from 'react'
import { X } from 'lucide-react'
import { api } from '@/api'
import type { AgentTools, AgentToolEntry, AgentToolTier } from '@/types'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton } from '@/shared/components'
import { AGENT_TIERS } from '@/features/tools/toolMeta'
import { AgentTierBadge, AgentTierSelector } from '@/features/tools/VisibilityControls'
import { AgentToolOverrideRow } from './AgentToolOverrideRow'

interface Props {
  agentId: string
  onError?: (msg: string) => void
}

// AgentToolsSection manages an agent's per-tool OVERRIDES. Every tool reaches
// the agent at its workspace-effective tier by default; this panel collects the
// tools pinned to a different one — including 'Yasaklı', which replaces the old
// standalone denylist (banning is now the last stop on the same scale).
//
// Only overridden tools are listed: the screen is a DIFF against the workspace
// defaults, not a second copy of the 140-row tools catalog. Changes auto-save,
// and picking a tier equal to the default deletes the override instead of
// storing a redundant one.
export function AgentToolsSection({ agentId, onError }: Props) {
  const [data, setData] = useState<AgentTools | null>(null)
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('')
  // Tier applied by a plain click in the picker (and by the bulk action). Ban is
  // the common case, so it is the default — matching the old panel's behaviour.
  const [pickTier, setPickTier] = useState<AgentToolTier>('blocked')
  // Multi-select on the picker list: modifier-click selects, then one bulk action
  // overrides every selected tool in a single save.
  const sel = useMultiSelect()

  const load = useCallback(() => {
    api.agentTools(agentId).then(setData).catch((e) => onError?.(e.message))
  }, [agentId, onError])

  useEffect(() => load(), [load])

  const catalog = useMemo(() => data?.catalog ?? [], [data])
  const byName = useMemo(() => new Map(catalog.map((t) => [t.name, t])), [catalog])
  const overrides = useMemo(() => data?.toolOverrides ?? {}, [data])

  const save = async (mcpEnabled: boolean, next: Record<string, AgentToolTier>) => {
    if (!data) return
    setBusy(true)
    setData({ ...data, mcpEnabled, toolOverrides: next }) // optimistic
    try {
      await api.setAgentTools(agentId, mcpEnabled, next)
    } catch (e) {
      onError?.((e as Error).message)
      load()
    } finally {
      setBusy(false)
    }
  }

  // setTier pins one tool to a tier — or CLEARS the override when the chosen tier
  // equals the tool's workspace default, so the diff list never accumulates
  // no-op entries. A tool missing from the catalog (a stale entry or a "prefix*"
  // pattern) has no known default, so its override is always kept.
  const setTier = (name: string, tier: AgentToolTier) => {
    const next = { ...overrides }
    if (byName.get(name)?.defaultVisibility === tier) delete next[name]
    else next[name] = tier
    save(data?.mcpEnabled ?? true, next)
  }

  const clearOverride = (name: string) => {
    const next = { ...overrides }
    delete next[name]
    save(data?.mcpEnabled ?? true, next)
  }

  const resetAll = () => save(data?.mcpEnabled ?? true, {})
  const blockAll = () =>
    save(
      data?.mcpEnabled ?? true,
      Object.fromEntries(catalog.map((t) => [t.name, 'blocked' as AgentToolTier])),
    )

  const applyToSelected = (tier: AgentToolTier) => {
    const next = { ...overrides }
    for (const name of sel.selected) {
      if (byName.get(name)?.defaultVisibility === tier) delete next[name]
      else next[name] = tier
    }
    save(data?.mcpEnabled ?? true, next)
    sel.clear()
  }

  // Overridden tools that exist in the catalog — the main diff list.
  const overridden = useMemo(
    () =>
      Object.keys(overrides)
        .filter((n) => byName.has(n))
        .sort()
        .map((n) => ({ tool: byName.get(n) as AgentToolEntry, tier: overrides[n] })),
    [overrides, byName],
  )
  // Override keys with no catalog entry: a "prefix*" pattern, or a tool not built
  // for this workspace right now (a gated shell tool, a disabled MCP server).
  // They are kept — dropping them would silently lift a ban — but listed apart
  // since there is no default to diff against.
  const orphans = useMemo(
    () => Object.keys(overrides).filter((n) => !byName.has(n)).sort(),
    [overrides, byName],
  )

  // Tools with no override yet, filtered by the search box.
  const available = useMemo(() => {
    const q = query.trim().toLowerCase()
    return catalog.filter(
      (t) =>
        !(t.name in overrides) &&
        (!q || t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)),
    )
  }, [catalog, overrides, query])

  if (!data) {
    return <p className="text-xs text-[var(--color-text-dim)]">Araçlar yükleniyor…</p>
  }

  const overrideCount = overridden.length + orphans.length

  return (
    <div className="space-y-3">
      <label className="flex items-center gap-2.5 text-sm">
        <input
          data-testid="agent-tools-enable-checkbox"
          type="checkbox"
          checked={data.mcpEnabled}
          disabled={busy}
          onChange={(e) => save(e.target.checked, overrides)}
        />
        <span className="font-medium">Bu ajan için araç kullanımını etkinleştir</span>
      </label>

      {data.mcpEnabled && (
        <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs text-[var(--color-text-dim)]">
              Araç override'ları ({overrideCount}/{catalog.length})
            </p>
            <div className="flex gap-2 text-xs">
              <button
                data-testid="agent-tools-clear-blocks"
                onClick={resetAll}
                disabled={busy || overrideCount === 0}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90 disabled:opacity-40"
                title="Tüm override'ları kaldır (her araç workspace varsayılanına döner)"
              >
                Tümünü sıfırla
              </button>
              <button
                data-testid="agent-tools-block-all"
                onClick={blockAll}
                disabled={busy || catalog.length === 0}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90 disabled:opacity-40"
                title="Tüm araçları yasakla"
              >
                Tümünü yasakla
              </button>
            </div>
          </div>

          {overrideCount === 0 ? (
            <p className="mb-3 rounded border border-dashed border-[var(--color-border)] px-3 py-3 text-center text-xs text-[var(--color-text-dim)]">
              Hiçbir override yok — her araç workspace varsayılanıyla geliyor. Aşağıdan bir aracı
              seçip görünürlüğünü değiştir veya yasakla.
            </p>
          ) : (
            <ul className="mb-3 space-y-1">
              {overridden.map(({ tool, tier }) => (
                <AgentToolOverrideRow
                  key={tool.name}
                  tool={tool}
                  tier={tier}
                  busy={busy}
                  onSelect={(t) => setTier(tool.name, t)}
                  onReset={() => clearOverride(tool.name)}
                />
              ))}
              {orphans.map((name) => (
                <li
                  key={name}
                  data-testid="agent-tool-override-orphan"
                  data-tool-name={name}
                  className="flex items-center gap-2 rounded border border-dashed border-[var(--color-border)] px-2.5 py-1.5"
                  title="Bu isim şu an katalogda yok (desen veya kapalı bir araç). Kayıt korunuyor."
                >
                  <code className="text-xs">{name}</code>
                  <AgentTierBadge tier={overrides[name]} />
                  <button
                    onClick={() => clearOverride(name)}
                    disabled={busy}
                    title="Kaydı sil"
                    className="ml-auto rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-danger)] disabled:opacity-40"
                  >
                    <X size={13} />
                  </button>
                </li>
              ))}
            </ul>
          )}

          {/* Add-an-override picker */}
          <div className="border-t border-[var(--color-border)] pt-2">
            <div className="mb-2 flex flex-wrap items-center gap-2">
              <input
                data-testid="agent-tools-search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Override eklemek için araç ara…"
                className="min-w-40 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
              />
              <span className="flex items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                Tıklayınca:
                <AgentTierSelector value={pickTier} busy={busy} onSelect={setPickTier} compact />
              </span>
            </div>
            <div className="max-h-56 space-y-1 overflow-y-auto">
              {catalog.length === 0 && (
                <p className="text-xs text-[var(--color-text-dim)]">
                  Bu workspace'te aktif araç yok. Araçlar ekranından etkinleştir.
                </p>
              )}
              {catalog.length > 0 && available.length === 0 && (
                <p className="px-1 py-1 text-xs text-[var(--color-text-dim)]">
                  {query.trim() ? 'Eşleşen araç yok.' : 'Tüm araçlarda zaten override var.'}
                </p>
              )}
              {available.map((t) => {
                const orderedIds = available.map((x) => x.name)
                return (
                  <button
                    key={t.name}
                    data-testid="agent-tool-block-add"
                    data-tool-name={t.name}
                    onClick={(e) => {
                      // Modifier-click multi-selects; plain click applies pickTier.
                      if (sel.handleClick(e, t.name, orderedIds)) return
                      setTier(t.name, pickTier)
                    }}
                    disabled={busy}
                    className={`flex w-full items-start gap-2 rounded px-1 py-1 text-left hover:bg-[var(--color-surface-2)] ${
                      sel.isSelected(t.name) ? 'bg-[var(--color-accent-soft)] ring-1 ring-[var(--color-accent)]' : ''
                    }`}
                  >
                    <AgentTierBadge tier={t.defaultVisibility} className="mt-0.5 shrink-0 opacity-70" />
                    <span className="min-w-0">
                      <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">
                        {t.name}
                      </code>
                      <span className="ml-2 text-xs text-[var(--color-text-dim)]">{t.description}</span>
                    </span>
                  </button>
                )
              })}
            </div>
          </div>

          <SelectionBar
            count={sel.count}
            onClear={sel.clear}
            onSelectAll={available.length ? () => sel.selectAll(available.map((t) => t.name)) : undefined}
          >
            {AGENT_TIERS.map((tier) => (
              <SelectionBarButton
                key={tier.value}
                onClick={() => applyToSelected(tier.value)}
                danger={tier.value === 'blocked'}
              >
                {tier.label}
              </SelectionBarButton>
            ))}
          </SelectionBar>
        </div>
      )}
    </div>
  )
}
