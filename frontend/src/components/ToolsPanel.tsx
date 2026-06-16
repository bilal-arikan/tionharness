import { useCallback, useEffect, useState } from 'react'
import { api } from '../api'
import type { Agent, AgentTools, MCPServer, MCPTransport } from '../types'

interface Props {
  agent: Agent | null
  onError: (msg: string) => void
}

// ToolsPanel manages workspace MCP servers and, for the selected agent, its
// tool access (whether tools are enabled + the live catalog it would receive).
export function ToolsPanel({ agent, onError }: Props) {
  const [servers, setServers] = useState<MCPServer[]>([])
  const [testing, setTesting] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<Record<string, string>>({})

  // Add-server form.
  const [name, setName] = useState('')
  const [transport, setTransport] = useState<MCPTransport>('stdio')
  const [command, setCommand] = useState('')
  const [argsText, setArgsText] = useState('')
  const [url, setUrl] = useState('')

  // Agent tool settings.
  const [tools, setTools] = useState<AgentTools | null>(null)
  const [saving, setSaving] = useState(false)

  const loadServers = useCallback(() => {
    api.listMCPServers().then(setServers).catch((e) => onError(e.message))
  }, [onError])

  useEffect(() => loadServers(), [loadServers])

  useEffect(() => {
    if (!agent) {
      setTools(null)
      return
    }
    api.agentTools(agent.id).then(setTools).catch((e) => onError(e.message))
  }, [agent, onError, servers])

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

  const saveAgentTools = async (mcpEnabled: boolean) => {
    if (!agent || !tools) return
    setSaving(true)
    try {
      await api.setAgentTools(agent.id, mcpEnabled, tools.allowedTools)
      const fresh = await api.agentTools(agent.id)
      setTools(fresh)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex-1 overflow-y-auto p-6">
      <div className="mx-auto max-w-3xl space-y-8">
        {/* Agent tool access */}
        <section>
          <h2 className="mb-2 text-sm font-semibold">Ajan araç erişimi</h2>
          {!agent ? (
            <p className="text-sm text-[var(--color-text-dim)]">
              Soldan bir ajan seçin (araçlar ajan bazında açılır).
            </p>
          ) : (
            <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
              <label className="flex items-center gap-3 text-sm">
                <input
                  type="checkbox"
                  checked={tools?.mcpEnabled ?? false}
                  disabled={saving}
                  onChange={(e) => saveAgentTools(e.target.checked)}
                />
                <span>
                  <span className="font-medium">{agent.name}</span> için araç kullanımını etkinleştir
                </span>
              </label>
              {tools?.mcpEnabled && (
                <div className="mt-3 border-t border-[var(--color-border)] pt-3">
                  <p className="mb-2 text-xs text-[var(--color-text-dim)]">
                    Bu ajana sunulan araçlar ({tools.catalog.length}):
                  </p>
                  <ul className="space-y-1">
                    {tools.catalog.map((t) => (
                      <li key={t.name} className="text-sm">
                        <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">
                          {t.name}
                        </code>
                        <span className="ml-2 text-[var(--color-text-dim)]">{t.description}</span>
                      </li>
                    ))}
                    {tools.catalog.length === 0 && (
                      <li className="text-sm text-[var(--color-text-dim)]">Henüz araç yok.</li>
                    )}
                  </ul>
                </div>
              )}
            </div>
          )}
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
