import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import type { AgentTools } from '../../types'

interface Props {
  agentId: string
  onError?: (msg: string) => void
}

// AgentToolsSection lets you pick which of the workspace-ACTIVE tools a given
// agent may use. The master switch toggles tool use entirely; when on, each
// active tool can be allowed individually. An empty selection means "all active
// tools" (the default), so tools later activated workspace-wide extend to the
// agent automatically. Changes auto-save.
export function AgentToolsSection({ agentId, onError }: Props) {
  const [data, setData] = useState<AgentTools | null>(null)
  const [busy, setBusy] = useState(false)

  const load = useCallback(() => {
    api.agentTools(agentId).then(setData).catch((e) => onError?.(e.message))
  }, [agentId, onError])

  useEffect(() => load(), [load])

  // Build the selected set: empty allowlist => all active tools selected.
  const names = data?.catalog.map((t) => t.name) ?? []
  const allowed = data?.allowedTools ?? []
  const selected = new Set(allowed.length === 0 ? names : allowed)
  const selectedCount = names.filter((n) => selected.has(n)).length

  const save = async (mcpEnabled: boolean, allowedTools: string[]) => {
    if (!data) return
    setBusy(true)
    // If every active tool is selected, store [] (= all) so new tools extend automatically.
    const next = allowedTools.length === names.length ? [] : allowedTools
    setData({ ...data, mcpEnabled, allowedTools: next }) // optimistic
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
    const next = new Set(selected)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    save(data?.mcpEnabled ?? true, Array.from(next))
  }

  const setAll = (on: boolean) => save(data?.mcpEnabled ?? true, on ? names : [])

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
          onChange={(e) => save(e.target.checked, data.allowedTools)}
        />
        <span className="font-medium">Bu ajan için araç kullanımını etkinleştir</span>
      </label>

      {data.mcpEnabled && (
        <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3">
          <div className="mb-2 flex items-center justify-between">
            <p className="text-xs text-[var(--color-text-dim)]">
              Kullanılabilir araçlar ({selectedCount}/{names.length} seçili)
            </p>
            <div className="flex gap-2 text-xs">
              <button
                data-testid="agent-tools-select-all"
                onClick={() => setAll(true)}
                disabled={busy}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90"
              >
                Hepsi
              </button>
              <button
                data-testid="agent-tools-select-none"
                onClick={() => save(true, ['__none__'])}
                disabled={busy}
                className="rounded bg-[var(--color-surface-2)] px-2 py-0.5 hover:opacity-90"
                title="Hiçbir aracı kullanma"
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
                  checked={selected.has(t.name)}
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
