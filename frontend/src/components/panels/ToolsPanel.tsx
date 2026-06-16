import { useCallback, useEffect, useState } from 'react'
import { api } from '../../api'
import type { MCPServer, MCPTransport, WorkspaceTool } from '../../types'

interface Props {
  onError: (msg: string) => void
}

// ToolsPanel is the workspace-wide tools screen. It lists every available tool
// (built-ins + tools from enabled MCP servers) with a per-tool activate/deactivate
// toggle that applies to the whole workspace, plus MCP server management. Agents
// then pick which of the active tools they may use from the Agents screen.
export function ToolsPanel({ onError }: Props) {
  const [servers, setServers] = useState<MCPServer[]>([])
  const [testing, setTesting] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<Record<string, string>>({})

  // Add-server form.
  const [name, setName] = useState('')
  const [transport, setTransport] = useState<MCPTransport>('stdio')
  const [command, setCommand] = useState('')
  const [argsText, setArgsText] = useState('')
  const [url, setUrl] = useState('')

  // Workspace tool activation.
  const [tools, setTools] = useState<WorkspaceTool[]>([])
  const [disabled, setDisabled] = useState<string[]>([])
  const [savingTool, setSavingTool] = useState<string | null>(null)

  const loadServers = useCallback(() => {
    api.listMCPServers().then(setServers).catch((e) => onError(e.message))
  }, [onError])

  const loadTools = useCallback(() => {
    api
      .workspaceTools()
      .then((res) => {
        setTools(res.tools)
        setDisabled(res.disabledTools)
      })
      .catch((e) => onError(e.message))
  }, [onError])

  useEffect(() => loadServers(), [loadServers])
  // Reload the catalog whenever the set of MCP servers changes (enabling a
  // server adds its tools to the workspace catalog).
  useEffect(() => loadTools(), [loadTools, servers])

  const toggleTool = async (t: WorkspaceTool) => {
    const next = t.enabled ? [...new Set([...disabled, t.name])] : disabled.filter((n) => n !== t.name)
    setSavingTool(t.name)
    // Optimistic update.
    setTools((ts) => ts.map((x) => (x.name === t.name ? { ...x, enabled: !t.enabled } : x)))
    setDisabled(next)
    try {
      await api.setWorkspaceTools(next)
    } catch (e) {
      onError((e as Error).message)
      loadTools() // revert on failure
    } finally {
      setSavingTool(null)
    }
  }

  const addServer = async () => {
    if (!name.trim()) return
    try {
      // Split args on whitespace; quotes are not parsed (keep it simple).
      const args = argsText.trim() ? argsText.trim().split(/\s+/) : []
      await api.createMCPServer({ name: name.trim(), transport, command: command.trim(), args, url: url.trim() })
      setName('')
      setCommand('')
      setArgsText('')
      setUrl('')
      loadServers()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const toggleServer = async (s: MCPServer) => {
    try {
      await api.toggleMCPServer(s.id, !s.enabled)
      loadServers()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const testServer = async (s: MCPServer) => {
    setTesting(s.id)
    setTestResult((r) => ({ ...r, [s.id]: 'Test ediliyor…' }))
    try {
      const res = await api.testMCPServer(s.id)
      setTestResult((r) => ({
        ...r,
        [s.id]: res.ok
          ? `✓ ${res.toolCount} araç: ${(res.tools ?? []).map((t) => t.name).join(', ')}`
          : `✗ ${res.error}`,
      }))
    } catch (e) {
      setTestResult((r) => ({ ...r, [s.id]: `✗ ${(e as Error).message}` }))
    } finally {
      setTesting(null)
    }
  }

  const removeServer = async (s: MCPServer) => {
    if (!confirm(`"${s.name}" sunucusu silinsin mi?`)) return
    try {
      await api.deleteMCPServer(s.id)
      loadServers()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const activeCount = tools.filter((t) => t.enabled).length

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="mx-auto max-w-3xl space-y-8">
        {/* Workspace tool activation */}
        <section>
          <h2 className="mb-1 text-sm font-semibold">Workspace araçları</h2>
          <p className="mb-3 text-xs text-[var(--color-text-dim)]">
            Bu workspace'te kullanılabilecek araçları aç/kapat. Kapatılan araçlar hiçbir ajana sunulmaz.
            Ajanlar, aktif araçlardan hangilerini kullanacağını <strong>Ajanlar</strong> ekranından seçer.
            ({activeCount}/{tools.length} aktif)
          </p>
          <div className="divide-y divide-[var(--color-border)] overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
            {tools.map((t) => (
              <label
                key={t.name}
                className="flex cursor-pointer items-start gap-3 px-4 py-2.5 hover:bg-[var(--color-surface-2)]"
              >
                <input
                  type="checkbox"
                  checked={t.enabled}
                  disabled={savingTool === t.name}
                  onChange={() => toggleTool(t)}
                  className="mt-1"
                />
                <span className="min-w-0">
                  <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">{t.name}</code>
                  <span className="ml-2 text-sm text-[var(--color-text-dim)]">{t.description}</span>
                </span>
              </label>
            ))}
            {tools.length === 0 && (
              <p className="px-4 py-3 text-sm text-[var(--color-text-dim)]">Henüz araç yok.</p>
            )}
          </div>
        </section>

        {/* MCP servers */}
        <section>
          <h2 className="mb-2 text-sm font-semibold">MCP Sunucuları</h2>
          <div className="space-y-2">
            {servers.map((s) => (
              <div
                key={s.id}
                className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{s.name}</span>
                      <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
                        {s.transport}
                      </span>
                      {!s.enabled && (
                        <span className="text-xs text-[var(--color-text-dim)]">(devre dışı)</span>
                      )}
                    </div>
                    <div className="truncate text-xs text-[var(--color-text-dim)]">
                      {s.transport === 'stdio' ? `${s.command} ${JSON.parse(s.args || '[]').join(' ')}` : s.url}
                    </div>
                  </div>
                  <div className="flex flex-shrink-0 items-center gap-2">
                    <button
                      onClick={() => testServer(s)}
                      disabled={testing === s.id}
                      className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                    >
                      Test
                    </button>
                    <button
                      onClick={() => toggleServer(s)}
                      className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                    >
                      {s.enabled ? 'Kapat' : 'Aç'}
                    </button>
                    <button
                      onClick={() => removeServer(s)}
                      className="rounded px-2 py-1 text-xs text-red-400 hover:bg-red-500/10"
                    >
                      Sil
                    </button>
                  </div>
                </div>
                {testResult[s.id] && (
                  <div className="mt-2 break-words text-xs text-[var(--color-text-dim)]">
                    {testResult[s.id]}
                  </div>
                )}
              </div>
            ))}
            {servers.length === 0 && (
              <p className="text-sm text-[var(--color-text-dim)]">Henüz MCP sunucusu eklenmedi.</p>
            )}
          </div>

          {/* Add server */}
          <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
            <h3 className="mb-3 text-xs font-semibold text-[var(--color-text-dim)]">Yeni MCP sunucusu</h3>
            <div className="grid grid-cols-2 gap-2">
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="İsim (ör. filesystem)"
                className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <select
                value={transport}
                onChange={(e) => setTransport(e.target.value as MCPTransport)}
                className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              >
                <option value="stdio">stdio</option>
                <option value="sse">sse</option>
                <option value="http">http</option>
              </select>
              {transport === 'stdio' ? (
                <>
                  <input
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    placeholder="Komut (ör. npx)"
                    className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
                  />
                  <input
                    value={argsText}
                    onChange={(e) => setArgsText(e.target.value)}
                    placeholder="Argümanlar (boşlukla ayrılmış)"
                    className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
                  />
                </>
              ) : (
                <input
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  placeholder="URL"
                  className="col-span-2 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
                />
              )}
            </div>
            <button
              onClick={addServer}
              className="mt-3 rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
            >
              Ekle
            </button>
          </div>
        </section>
      </div>
    </div>
  )
}
