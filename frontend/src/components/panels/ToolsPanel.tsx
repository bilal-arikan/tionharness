import { useCallback, useEffect, useMemo, useState } from 'react'
import { Search, Plug, Wrench } from 'lucide-react'
import { api } from '../../api'
import type { MCPServer, MCPTransport, WorkspaceTool } from '../../types'

interface Props {
  onError: (msg: string) => void
}

// A JSON-Schema-ish parameter row extracted from a tool's inputSchema.
interface ParamRow {
  name: string
  type: string
  required: boolean
  description: string
}

// extractParams flattens a tool's JSON Schema into a simple list for display.
function extractParams(schema: unknown): ParamRow[] {
  if (!schema || typeof schema !== 'object') return []
  const s = schema as { properties?: Record<string, unknown>; required?: string[] }
  if (!s.properties) return []
  const required = new Set(s.required ?? [])
  return Object.entries(s.properties).map(([name, raw]) => {
    const p = (raw ?? {}) as { type?: string; description?: string }
    return {
      name,
      type: p.type ?? 'any',
      required: required.has(name),
      description: p.description ?? '',
    }
  })
}

// ToolsPanel is the workspace-wide tools screen. The left column lists every
// available tool (built-ins + tools from enabled MCP servers), grouped by
// origin and filterable; clicking a tool shows its details on the right, where
// it can be activated/deactivated for the whole workspace. The right column also
// hosts MCP server management when no tool is selected.
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

  // Workspace tool activation + selection.
  const [tools, setTools] = useState<WorkspaceTool[]>([])
  const [disabled, setDisabled] = useState<string[]>([])
  const [savingTool, setSavingTool] = useState<string | null>(null)
  const [selectedName, setSelectedName] = useState<string | null>(null)
  const [query, setQuery] = useState('')

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

  // Filter by the search query (matches label, full name, or description).
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return tools
    return tools.filter(
      (t) =>
        t.label.toLowerCase().includes(q) ||
        t.name.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q),
    )
  }, [tools, query])

  // Group filtered tools by origin: built-ins first, then one group per MCP server.
  const groups = useMemo(() => {
    const builtins = filtered.filter((t) => t.source === 'builtin')
    const byServer = new Map<string, WorkspaceTool[]>()
    for (const t of filtered) {
      if (t.source !== 'mcp') continue
      const key = t.server || 'MCP'
      if (!byServer.has(key)) byServer.set(key, [])
      byServer.get(key)!.push(t)
    }
    const out: { label: string; tools: WorkspaceTool[] }[] = []
    if (builtins.length) out.push({ label: 'Yerleşik', tools: builtins })
    for (const [server, list] of [...byServer.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
      out.push({ label: server, tools: list })
    }
    return out
  }, [filtered])

  const selected = tools.find((t) => t.name === selectedName) ?? null
  const params = useMemo(() => extractParams(selected?.inputSchema), [selected])

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: searchable, grouped tool list. */}
      <aside className="flex w-72 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]">
        <div className="border-b border-[var(--color-border)] p-3">
          <div className="relative">
            <Search
              size={14}
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
            />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Araç ara…"
              className="w-full rounded-lg bg-[var(--color-surface-2)] py-2 pl-8 pr-3 text-sm outline-none"
            />
          </div>
          <p className="mt-2 px-0.5 text-xs text-[var(--color-text-dim)]">
            {activeCount}/{tools.length} aktif
          </p>
        </div>

        <div className="flex-1 overflow-y-auto py-2">
          {groups.map((g) => (
            <div key={g.label} className="mb-2">
              <div className="px-3 py-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                {g.label}
              </div>
              {g.tools.map((t) => {
                const active = selectedName === t.name
                return (
                  <button
                    key={t.name}
                    onClick={() => setSelectedName(t.name)}
                    className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition ${
                      active
                        ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                        : 'hover:bg-[var(--color-surface-2)]'
                    }`}
                  >
                    <span
                      title={t.enabled ? 'Aktif' : 'Devre dışı'}
                      className={`h-1.5 w-1.5 flex-shrink-0 rounded-full ${
                        t.enabled ? 'bg-emerald-500' : 'bg-[var(--color-border)]'
                      }`}
                    />
                    <span className={`min-w-0 truncate ${t.enabled ? '' : 'text-[var(--color-text-dim)]'}`}>
                      {t.label}
                    </span>
                  </button>
                )
              })}
            </div>
          ))}
          {groups.length === 0 && (
            <p className="px-3 py-3 text-sm text-[var(--color-text-dim)]">
              {tools.length === 0 ? 'Henüz araç yok.' : 'Eşleşen araç yok.'}
            </p>
          )}
        </div>

        {/* Footer: jump to MCP server management (clears tool selection). */}
        <button
          onClick={() => setSelectedName(null)}
          className={`flex items-center gap-2 border-t border-[var(--color-border)] px-3 py-2.5 text-sm transition ${
            selected ? 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]' : 'text-[var(--color-accent)]'
          }`}
        >
          <Plug size={15} /> MCP Sunucuları
        </button>
      </aside>

      {/* Right: selected tool detail, or MCP server management. */}
      <div className="flex-1 overflow-y-auto p-6">
        {selected ? (
          <ToolDetail tool={selected} params={params} saving={savingTool === selected.name} onToggle={() => toggleTool(selected)} />
        ) : (
          <ServerManagement
            servers={servers}
            testing={testing}
            testResult={testResult}
            name={name}
            setName={setName}
            transport={transport}
            setTransport={setTransport}
            command={command}
            setCommand={setCommand}
            argsText={argsText}
            setArgsText={setArgsText}
            url={url}
            setUrl={setUrl}
            onAdd={addServer}
            onToggle={toggleServer}
            onTest={testServer}
            onRemove={removeServer}
          />
        )}
      </div>
    </div>
  )
}

// ToolDetail renders the right-hand detail view for one selected tool.
function ToolDetail({
  tool,
  params,
  saving,
  onToggle,
}: {
  tool: WorkspaceTool
  params: ParamRow[]
  saving: boolean
  onToggle: () => void
}) {
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Wrench size={18} className="flex-shrink-0 text-[var(--color-text-dim)]" />
            <h2 className="truncate text-lg font-semibold">{tool.label}</h2>
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-2">
            <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
              {tool.source === 'mcp' ? `MCP · ${tool.server}` : 'Yerleşik'}
            </span>
            <span
              className={`rounded px-1.5 py-0.5 text-xs ${
                tool.enabled
                  ? 'bg-emerald-500/10 text-emerald-500'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
              }`}
            >
              {tool.enabled ? 'Aktif' : 'Devre dışı'}
            </span>
          </div>
        </div>
        <button
          onClick={onToggle}
          disabled={saving}
          className={`flex-shrink-0 rounded-lg px-4 py-2 text-sm font-medium transition disabled:opacity-50 ${
            tool.enabled
              ? 'bg-[var(--color-surface-2)] hover:opacity-90'
              : 'bg-[var(--color-accent)] text-white hover:opacity-90'
          }`}
        >
          {tool.enabled ? 'Devre dışı bırak' : 'Etkinleştir'}
        </button>
      </div>

      {tool.source === 'mcp' && (
        <div>
          <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs">{tool.name}</code>
        </div>
      )}

      <section>
        <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Açıklama
        </h3>
        <p className="text-sm leading-relaxed">
          {tool.description || <span className="text-[var(--color-text-dim)]">Açıklama yok.</span>}
        </p>
      </section>

      <section>
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Parametreler ({params.length})
        </h3>
        {params.length === 0 ? (
          <p className="text-sm text-[var(--color-text-dim)]">Bu araç parametre almıyor.</p>
        ) : (
          <div className="divide-y divide-[var(--color-border)] overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
            {params.map((p) => (
              <div key={p.name} className="px-4 py-2.5">
                <div className="flex items-center gap-2">
                  <code className="text-sm font-medium">{p.name}</code>
                  <span className="text-xs text-[var(--color-text-dim)]">{p.type}</span>
                  {p.required && (
                    <span className="rounded bg-red-500/10 px-1.5 py-0.5 text-[10px] font-medium text-red-400">
                      zorunlu
                    </span>
                  )}
                </div>
                {p.description && (
                  <p className="mt-1 text-xs text-[var(--color-text-dim)]">{p.description}</p>
                )}
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  )
}

// ServerManagement is the MCP server list + add form, shown when no tool is
// selected. (Extracted so the right pane stays readable.)
function ServerManagement(props: {
  servers: MCPServer[]
  testing: string | null
  testResult: Record<string, string>
  name: string
  setName: (v: string) => void
  transport: MCPTransport
  setTransport: (v: MCPTransport) => void
  command: string
  setCommand: (v: string) => void
  argsText: string
  setArgsText: (v: string) => void
  url: string
  setUrl: (v: string) => void
  onAdd: () => void
  onToggle: (s: MCPServer) => void
  onTest: (s: MCPServer) => void
  onRemove: (s: MCPServer) => void
}) {
  const {
    servers,
    testing,
    testResult,
    name,
    setName,
    transport,
    setTransport,
    command,
    setCommand,
    argsText,
    setArgsText,
    url,
    setUrl,
    onAdd,
    onToggle,
    onTest,
    onRemove,
  } = props
  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="mb-1 text-sm font-semibold">MCP Sunucuları</h2>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        Soldaki listeden bir araç seçerek detaylarını görüntüleyip aç/kapatabilirsin.
      </p>
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
                  {!s.enabled && <span className="text-xs text-[var(--color-text-dim)]">(devre dışı)</span>}
                </div>
                <div className="truncate text-xs text-[var(--color-text-dim)]">
                  {s.transport === 'stdio' ? `${s.command} ${JSON.parse(s.args || '[]').join(' ')}` : s.url}
                </div>
              </div>
              <div className="flex flex-shrink-0 items-center gap-2">
                <button
                  onClick={() => onTest(s)}
                  disabled={testing === s.id}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                >
                  Test
                </button>
                <button
                  onClick={() => onToggle(s)}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                >
                  {s.enabled ? 'Kapat' : 'Aç'}
                </button>
                <button
                  onClick={() => onRemove(s)}
                  className="rounded px-2 py-1 text-xs text-red-400 hover:bg-red-500/10"
                >
                  Sil
                </button>
              </div>
            </div>
            {testResult[s.id] && (
              <div className="mt-2 break-words text-xs text-[var(--color-text-dim)]">{testResult[s.id]}</div>
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
          onClick={onAdd}
          className="mt-3 rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          Ekle
        </button>
      </div>
    </div>
  )
}
