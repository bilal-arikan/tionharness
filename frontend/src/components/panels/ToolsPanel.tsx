import { useCallback, useEffect, useMemo, useState } from 'react'
import { Search, Plug, ChevronRight, Eye, EyeOff } from 'lucide-react'
import { api } from '../../api'
import type { MCPServer, MCPTransport, WorkspaceTool } from '../../types'
import { toolSource, toolServer, toolLabel, extractParams, type ParamRow } from './toolMeta'
import { toolIcon } from '../../lib/toolIcons'

interface Props {
  onError: (msg: string) => void
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
  // Visibility override lists: hiddenOv forces a tool load-on-demand; shownOv
  // forces a default-hidden tool (e.g. self-management) back into context.
  const [hiddenOv, setHiddenOv] = useState<string[]>([])
  const [shownOv, setShownOv] = useState<string[]>([])
  const [savingTool, setSavingTool] = useState<string | null>(null)
  const [hidingTool, setHidingTool] = useState<string | null>(null)
  const [selectedName, setSelectedName] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  // Collapsed group labels (accordion). Persisted so the choice sticks.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    try {
      return new Set<string>(JSON.parse(localStorage.getItem('swarmgo.toolsCollapsed') || '[]'))
    } catch {
      return new Set<string>()
    }
  })
  const toggleGroup = (label: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(label)) next.delete(label)
      else next.add(label)
      localStorage.setItem('swarmgo.toolsCollapsed', JSON.stringify([...next]))
      return next
    })
  }

  const loadServers = useCallback(() => {
    api.listMCPServers().then(setServers).catch((e) => onError(e.message))
  }, [onError])

  const loadTools = useCallback(() => {
    api
      .workspaceTools()
      .then((res) => {
        setTools(res.tools)
        setDisabled(res.disabledTools)
        setHiddenOv(res.hiddenTools ?? [])
        setShownOv(res.shownTools ?? [])
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

  // Toggle a tool's effective "hidden" (load-on-demand) state for the whole
  // workspace. A hidden tool stays active but its schema isn't shipped every turn
  // — the agent pulls it in via tool_search / activate_tools. Works for any tool,
  // including ones hidden by default in code (the self-management suite): showing
  // such a tool writes a "shown" override that forces it back into context.
  // Mirrors a skill's "Gizli" state.
  const toggleHidden = async (t: WorkspaceTool) => {
    const add = (list: string[], n: string) => [...new Set([...list, n])]
    const rm = (list: string[], n: string) => list.filter((x) => x !== n)
    // Making it shown → add to shown override, drop any hidden override (and v.v.).
    const nextHidden = t.hidden ? rm(hiddenOv, t.name) : add(hiddenOv, t.name)
    const nextShown = t.hidden ? add(shownOv, t.name) : rm(shownOv, t.name)
    setHidingTool(t.name)
    // Optimistic update.
    setTools((ts) => ts.map((x) => (x.name === t.name ? { ...x, hidden: !t.hidden } : x)))
    setHiddenOv(nextHidden)
    setShownOv(nextShown)
    try {
      await api.setWorkspaceToolsVisibility(nextHidden, nextShown)
    } catch (e) {
      onError((e as Error).message)
      loadTools() // revert on failure
    } finally {
      setHidingTool(null)
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
        toolLabel(t).toLowerCase().includes(q) ||
        t.name.toLowerCase().includes(q) ||
        (t.description ?? '').toLowerCase().includes(q),
    )
  }, [tools, query])

  // Group filtered tools by origin: built-ins first, then one group per MCP server.
  const groups = useMemo(() => {
    const builtins = filtered.filter((t) => toolSource(t) === 'builtin')
    const byServer = new Map<string, WorkspaceTool[]>()
    for (const t of filtered) {
      if (toolSource(t) !== 'mcp') continue
      const key = toolServer(t) || 'MCP'
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
          {/* Prominent, clearly-clickable jump to MCP server management. */}
          <button
            data-testid="tools-mcp-servers"
            onClick={() => setSelectedName(null)}
            title="MCP sunucularını yönet"
            className={`mb-3 flex w-full items-center justify-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition ${
              !selected
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] bg-[var(--color-surface-2)] text-[var(--color-text)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
            }`}
          >
            <Plug size={15} /> MCP Sunucuları
          </button>
          <div className="relative">
            <Search
              size={14}
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
            />
            <input
              data-testid="tools-search-input"
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
          {groups.map((g) => {
            const isCollapsed = collapsed.has(g.label)
            return (
              <div key={g.label} className="mb-1">
                <button
                  data-testid="tools-group-toggle"
                  data-group={g.label}
                  onClick={() => toggleGroup(g.label)}
                  className="flex w-full items-center gap-1.5 px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
                >
                  <ChevronRight
                    size={12}
                    className={`flex-shrink-0 transition-transform ${isCollapsed ? '' : 'rotate-90'}`}
                  />
                  <span className="truncate">{g.label}</span>
                  <span className="ml-auto font-normal tabular-nums opacity-70">{g.tools.length}</span>
                </button>
                {!isCollapsed &&
                  g.tools.map((t) => {
                    const active = selectedName === t.name
                    const ToolIcon = toolIcon(t.name)
                    return (
                      <button
                        key={t.name}
                        data-testid="tools-list-item"
                        data-tool-name={t.name}
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
                            t.enabled ? 'bg-[var(--color-success)]' : 'bg-[var(--color-border)]'
                          }`}
                        />
                        <ToolIcon
                          size={14}
                          className={`shrink-0 ${active ? '' : 'text-[var(--color-text-dim)]'}`}
                        />
                        <span
                          className={`min-w-0 truncate ${t.enabled ? '' : 'text-[var(--color-text-dim)]'}`}
                        >
                          {toolLabel(t)}
                        </span>
                        {t.selfManaged ? (
                          <SelfMgmtBadge className="ml-auto" />
                        ) : (
                          t.hidden && <HiddenBadge className="ml-auto" />
                        )}
                      </button>
                    )
                  })}
              </div>
            )
          })}
          {groups.length === 0 && (
            <p className="px-3 py-3 text-sm text-[var(--color-text-dim)]">
              {tools.length === 0 ? 'Henüz araç yok.' : 'Eşleşen araç yok.'}
            </p>
          )}
        </div>

      </aside>

      {/* Right: selected tool detail, or MCP server management. */}
      <div className="flex-1 overflow-y-auto p-6">
        {selected ? (
          <ToolDetail
            tool={selected}
            params={params}
            saving={savingTool === selected.name}
            hiding={hidingTool === selected.name}
            onToggle={() => toggleTool(selected)}
            onToggleHidden={() => toggleHidden(selected)}
          />
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

// NameOnlyBadge marks a tool rendered as name-only (Claude Code deferred-tool
// style): its summary + schema are not shipped every turn — only the name is
// listed, and the schema is pulled in via tool_search / activate_tools. The tool
// analog of a skill's "Gizli" (auto-summary off) state.
function HiddenBadge({ className = '' }: { className?: string }) {
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-warning,#d97706)_18%,transparent)] text-[var(--color-warning,#d97706)] ${className}`}
      title="Katalogda yalnızca ismi listelenir (özet gönderilmez); şema gerektiğinde tool_search/activate_tools ile yüklenir (yine de aktif)"
    >
      NameOnly
    </span>
  )
}

// SelfMgmtBadge marks a tool in the HIDDEN tier: not even listed by name in the
// per-turn catalog — folded into the single `swarmgo-self-management` skill pointer
// and discovered via tool_search. More aggressive than NameOnly (which still shows
// the name). The bulk admin family (manage agents/flows/schedules/…) plus confined
// config edits and secret reads live here. Still callable once activated.
function SelfMgmtBadge({ className = '' }: { className?: string }) {
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-text-dim)_22%,transparent)] text-[var(--color-text-dim)] ${className}`}
      title="Gizli (self-management tier): katalogda ismi bile listelenmez — tek bir 'swarmgo-self-management' skill işaretçisine katlanır, tool_search ile keşfedilir (yine de aktif edilince çağrılabilir)"
    >
      Self-mgmt
    </span>
  )
}

// ToolDetail renders the right-hand detail view for one selected tool.
function ToolDetail({
  tool,
  params,
  saving,
  hiding,
  onToggle,
  onToggleHidden,
}: {
  tool: WorkspaceTool
  params: ParamRow[]
  saving: boolean
  hiding: boolean
  onToggle: () => void
  onToggleHidden: () => void
}) {
  const ToolIcon = toolIcon(tool.name)
  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <ToolIcon size={18} className="flex-shrink-0 text-[var(--color-accent)]" />
            <h2 className="truncate text-lg font-semibold">{toolLabel(tool)}</h2>
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-2">
            <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-xs text-[var(--color-text-dim)]">
              {toolSource(tool) === 'mcp' ? `MCP · ${toolServer(tool)}` : 'Yerleşik'}
            </span>
            <span
              className={`rounded px-1.5 py-0.5 text-xs ${
                tool.enabled
                  ? 'bg-[color-mix(in_srgb,var(--color-success)_12%,transparent)] text-[var(--color-success)]'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
              }`}
            >
              {tool.enabled ? 'Aktif' : 'Devre dışı'}
            </span>
            {tool.selfManaged ? <SelfMgmtBadge /> : tool.hidden && <HiddenBadge />}
          </div>
        </div>
        <div className="flex flex-shrink-0 items-center gap-2">
          <button
            data-testid="tool-detail-hide"
            onClick={onToggleHidden}
            disabled={hiding}
            title={
              tool.hidden
                ? 'Göster: aracın şeması her tur ajana gönderilsin'
                : 'NameOnly: katalogda yalnız ismi görünsün, şema gerektiğinde on-demand yüklensin'
            }
            className="flex items-center gap-1.5 rounded-lg bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium transition hover:opacity-90 disabled:opacity-50"
          >
            {tool.hidden ? <Eye size={14} /> : <EyeOff size={14} />}
            {tool.hidden ? 'Göster' : 'NameOnly'}
          </button>
          <button
            data-testid="tool-detail-toggle"
            onClick={onToggle}
            disabled={saving}
            className={`rounded-lg px-4 py-2 text-sm font-medium transition disabled:opacity-50 ${
              tool.enabled
                ? 'bg-[var(--color-surface-2)] hover:opacity-90'
                : 'bg-[var(--color-accent)] text-white hover:opacity-90'
            }`}
          >
            {tool.enabled ? 'Devre dışı bırak' : 'Etkinleştir'}
          </button>
        </div>
      </div>

      {toolSource(tool) === 'mcp' && (
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
                    <span className="rounded bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-danger)]">
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
                  data-testid="mcp-server-test"
                  data-server-id={s.id}
                  onClick={() => onTest(s)}
                  disabled={testing === s.id}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                >
                  Test
                </button>
                <button
                  data-testid="mcp-server-toggle"
                  data-server-id={s.id}
                  onClick={() => onToggle(s)}
                  className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                >
                  {s.enabled ? 'Kapat' : 'Aç'}
                </button>
                <button
                  data-testid="mcp-server-delete"
                  data-server-id={s.id}
                  onClick={() => onRemove(s)}
                  className="rounded px-2 py-1 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
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
            data-testid="mcp-server-name-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="İsim (ör. filesystem)"
            className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
          />
          <select
            data-testid="mcp-server-transport-select"
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
                data-testid="mcp-server-command-input"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="Komut (ör. npx)"
                className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <input
                data-testid="mcp-server-args-input"
                value={argsText}
                onChange={(e) => setArgsText(e.target.value)}
                placeholder="Argümanlar (boşlukla ayrılmış)"
                className="rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
            </>
          ) : (
            <input
              data-testid="mcp-server-url-input"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="URL"
              className="col-span-2 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
            />
          )}
        </div>
        <button
          data-testid="mcp-server-add"
          onClick={onAdd}
          className="mt-3 rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90"
        >
          Ekle
        </button>
      </div>
    </div>
  )
}
