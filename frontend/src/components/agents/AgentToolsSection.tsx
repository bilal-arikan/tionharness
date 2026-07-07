import { useCallback, useEffect, useMemo, useState } from 'react'
import { Ban, Plus, X } from 'lucide-react'
import { api } from '../../api'
import type { AgentTools } from '../../types'
import { useMultiSelect } from '../../hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton } from '../common'

interface Props {
  agentId: string
  onError?: (msg: string) => void
}

// AgentToolsSection manages an agent's tool DENYLIST. An agent reaches every
// workspace-ACTIVE tool by default (including ones enabled workspace-wide
// later); this panel only collects the tools to switch OFF for this agent.
// The master switch toggles tool use entirely. Empty denylist = all tools.
// Changes auto-save.
export function AgentToolsSection({ agentId, onError }: Props) {
  const [data, setData] = useState<AgentTools | null>(null)
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('')
  // Multi-select on the "available" tool list: modifier-click selects, then one
  // bulk action blocks every selected tool in a single save. Plain click still
  // blocks a single tool immediately (the original behaviour).
  const sel = useMultiSelect()

  const load = useCallback(() => {
    api.agentTools(agentId).then(setData).catch((e) => onError?.(e.message))
  }, [agentId, onError])

  useEffect(() => load(), [load])

  const names = data?.catalog.map((t) => t.name) ?? []
  const blocked = useMemo(() => new Set(data?.blockedTools ?? []), [data])

  const save = async (mcpEnabled: boolean, blockedTools: string[]) => {
    if (!data) return
    setBusy(true)
    // Drop denylist entries for tools that no longer exist in the catalog.
    const next = blockedTools.filter((n) => names.includes(n))
    setData({ ...data, mcpEnabled, blockedTools: next }) // optimistic
    try {
      await api.setAgentTools(agentId, mcpEnabled, next)
    } catch (e) {
      onError?.((e as Error).message)
      load()
    } finally {
      setBusy(false)
    }
  }

  const block = (name: string) => save(data?.mcpEnabled ?? true, [...blocked, name])
  const unblock = (name: string) =>
    save(data?.mcpEnabled ?? true, Array.from(blocked).filter((n) => n !== name))
  const blockAll = () => save(data?.mcpEnabled ?? true, names)
  const clearAll = () => save(data?.mcpEnabled ?? true, [])
  const blockSelected = () => {
    save(data?.mcpEnabled ?? true, [...new Set([...blocked, ...sel.selected])])
    sel.clear()
  }

  // Tools still available to ban (not blocked yet), filtered by the search box.
  const available = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (data?.catalog ?? []).filter(
      (t) =>
        !blocked.has(t.name) &&
        (!q || t.name.toLowerCase().includes(q) || t.description.toLowerCase().includes(q)),
    )
  }, [data, blocked, query])

  if (!data) {
    return <p className="text-xs text-[var(--color-text-dim)]">Araçlar yükleniyor…</p>
  }

  const blockedList = data.catalog.filter((t) => blocked.has(t.name))

  return (
    <div className="space-y-3">
      <label className="flex items-center gap-2.5 text-sm">
        <input
          data-testid="agent-tools-enable-checkbox"
          type="checkbox"
          checked={data.mcpEnabled}
          disabled={busy}
          onChange={(e) => save(e.target.checked, data.blockedTools)}
        />
        <span className="font-medium">Bu ajan için araç kullanımını etkinleştir</span>
      </label>

      {data.mcpEnabled && (
        <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
          {/* Blocked tools (the denylist) */}
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs text-[var(--color-text-dim)]">
              Yasaklı araçlar ({blockedList.length}/{names.length})
            </p>
            <div className="flex gap-2 text-xs">
              <button
                data-testid="agent-tools-clear-blocks"
                onClick={clearAll}
                disabled={busy || blockedList.length === 0}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90 disabled:opacity-40"
                title="Tüm yasakları kaldır (ajan her aracı kullanabilir)"
              >
                Yasakları temizle
              </button>
              <button
                data-testid="agent-tools-block-all"
                onClick={blockAll}
                disabled={busy || names.length === 0 || blockedList.length === names.length}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90 disabled:opacity-40"
                title="Tüm araçları yasakla"
              >
                Tümünü yasakla
              </button>
            </div>
          </div>

          {blockedList.length === 0 ? (
            <p className="mb-3 rounded border border-dashed border-[var(--color-border)] px-3 py-3 text-center text-xs text-[var(--color-text-dim)]">
              Hiçbir araç yasaklı değil — bu ajan tüm araçları kullanabilir. Aşağıdan yasaklamak
              istediklerini ekle.
            </p>
          ) : (
            <ul className="mb-3 flex flex-wrap gap-1.5">
              {blockedList.map((t) => (
                <li key={t.name}>
                  {/* The whole chip is the un-block control: click it to lift the
                      ban (no separate X). */}
                  <button
                    data-testid="agent-tool-blocked"
                    data-tool-name={t.name}
                    onClick={() => unblock(t.name)}
                    disabled={busy}
                    title={`${t.description}\n\nYasağı kaldırmak için tıkla`}
                    className="group flex items-center gap-1.5 rounded-full border border-[var(--color-danger)]/40 bg-[var(--color-surface-2)] px-2.5 py-1 text-xs transition hover:border-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_12%,var(--color-surface-2))] disabled:opacity-50"
                  >
                    <Ban size={12} className="shrink-0 text-[var(--color-danger)] group-hover:hidden" />
                    <X size={12} className="hidden shrink-0 text-[var(--color-danger)] group-hover:block" />
                    <code className="text-xs">{t.name}</code>
                  </button>
                </li>
              ))}
            </ul>
          )}

          {/* Add-to-denylist picker */}
          <div className="border-t border-[var(--color-border)] pt-2">
            <input
              data-testid="agent-tools-search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Yasaklamak için araç ara…"
              className="mb-2 w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
            />
            <div className="max-h-56 space-y-1 overflow-y-auto">
              {names.length === 0 && (
                <p className="text-xs text-[var(--color-text-dim)]">
                  Bu workspace'te aktif araç yok. Araçlar ekranından etkinleştir.
                </p>
              )}
              {names.length > 0 && available.length === 0 && (
                <p className="px-1 py-1 text-xs text-[var(--color-text-dim)]">
                  {query.trim() ? 'Eşleşen araç yok.' : 'Tüm araçlar zaten yasaklı.'}
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
                    // Modifier-click multi-selects; plain click blocks immediately.
                    if (sel.handleClick(e, t.name, orderedIds)) return
                    block(t.name)
                  }}
                  disabled={busy}
                  className={`flex w-full items-start gap-2.5 rounded px-1 py-1 text-left hover:bg-[var(--color-surface-2)] ${
                    sel.isSelected(t.name) ? 'bg-[var(--color-accent-soft)] ring-1 ring-[var(--color-accent)]' : ''
                  }`}
                >
                  <Plus size={13} className="mt-1 shrink-0 text-[var(--color-text-dim)]" />
                  <span className="min-w-0">
                    <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">{t.name}</code>
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
            <SelectionBarButton icon={<Ban size={13} />} onClick={blockSelected} danger>
              Seçilenleri yasakla
            </SelectionBarButton>
          </SelectionBar>
        </div>
      )}
    </div>
  )
}
