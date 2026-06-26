import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import type { AgentTools } from '../../types'

interface Props {
  agentId: string
  onError?: (msg: string) => void
}

// AgentToolsSection controls which workspace-ACTIVE tools an agent may use. The
// master switch toggles tool use entirely. By default an agent reaches EVERY
// active tool (including ones enabled workspace-wide later); unchecking a tool
// adds it to the agent's denylist, switching it off for this agent only. An
// empty denylist means "all tools". Changes auto-save.
export function AgentToolsSection({ agentId, onError }: Props) {
  const [data, setData] = useState<AgentTools | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api.agentTools(agentId).then(setData).catch((e) => onError?.(e.message))
  }, [agentId, onError])

  useEffect(() => load(), [load])

  // A tool is enabled unless it is on the denylist. Default (empty denylist) =>
  // every tool enabled.
  const names = data?.catalog.map((t) => t.name) ?? []
  const blocked = new Set(data?.blockedTools ?? [])
  const enabledCount = names.filter((n) => !blocked.has(n)).length

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

  const toggleTool = (name: string) => {
    const next = new Set(blocked)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    save(data?.mcpEnabled ?? true, Array.from(next))
  }

  // "Hepsi" = clear the denylist (all enabled); "Hiçbiri" = block every tool.
  const setAll = (enabled: boolean) => save(data?.mcpEnabled ?? true, enabled ? [] : names)

  if (!data) {
    return <p className="text-xs text-[var(--color-text-dim)]">Araçlar yükleniyor…</p>
  }

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
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs text-[var(--color-text-dim)]">
              Araçlar ({enabledCount}/{names.length} açık) · işareti kaldırılan araç bu ajanda engellenir
            </p>
            <div className="flex gap-2 text-xs">
              <button
                data-testid="agent-tools-enable-all"
                onClick={() => setAll(true)}
                disabled={busy}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90"
                title="Hiçbir aracı engelleme"
              >
                Hepsi
              </button>
              <button
                data-testid="agent-tools-block-all"
                onClick={() => setAll(false)}
                disabled={busy}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90"
                title="Tüm araçları engelle"
              >
                Hiçbiri
              </button>
            </div>
          </div>
          <div className="max-h-64 space-y-1 overflow-y-auto">
            {names.length === 0 && (
              <p className="text-xs text-[var(--color-text-dim)]">
                Bu workspace'te aktif araç yok. Araçlar ekranından etkinleştir.
              </p>
            )}
            {data.catalog.map((t) => (
              <label
                key={t.name}
                className="flex cursor-pointer items-start gap-2.5 rounded px-1 py-1 hover:bg-[var(--color-surface-2)]"
              >
                <input
                  data-testid="agent-tool-checkbox"
                  data-tool-name={t.name}
                  type="checkbox"
                  checked={!blocked.has(t.name)}
                  disabled={busy}
                  onChange={() => toggleTool(t.name)}
                  className="mt-1"
                />
                <span className="min-w-0">
                  <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">{t.name}</code>
                  <span className="ml-2 text-xs text-[var(--color-text-dim)]">{t.description}</span>
                </span>
              </label>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
