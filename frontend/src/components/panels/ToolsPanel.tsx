import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSessionState } from '../../hooks/useSessionState'
import { Search, Plug, ChevronRight, Check, Ban } from 'lucide-react'
import { api } from '../../api'
import type { MCPServer, MCPTransport, ToolVisibility, WorkspaceTool } from '../../types'
import {
  toolSource,
  toolServer,
  toolLabel,
  toolCategory,
  CATEGORY_LABELS,
  CATEGORY_ORDER,
  extractParams,
  VISIBILITY_TIERS,
  visibilityMeta,
  type ParamRow,
} from './toolMeta'
import { toolIcon } from '../../lib/toolIcons'
import { useMultiSelect } from '../../hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton, ListPane, PaneHeader } from '../common'
import { useCollapsibleList } from '../../hooks/useCollapsibleList'

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
  // Optional http request headers, one "Key: Value" per line (e.g. Authorization).
  const [headersText, setHeadersText] = useState('')

  // Bulk JSON import (mcpServers document).
  const [importText, setImportText] = useState('')
  const [importing, setImporting] = useState(false)
  const [importMsg, setImportMsg] = useState('')

  // Workspace tool activation + selection.
  const [tools, setTools] = useState<WorkspaceTool[]>([])
  const [disabled, setDisabled] = useState<string[]>([])
  // Per-tool visibility override map (tool name → tier). The effective tier shown
  // per tool comes from t.visibility; this map is what we persist.
  const [visOv, setVisOv] = useState<Record<string, ToolVisibility>>({})
  const [savingTool, setSavingTool] = useState<string | null>(null)
  const [visBusy, setVisBusy] = useState<string | null>(null)
  // Selection persists across screen switches within the session (resets on reload).
  const [selectedName, setSelectedName] = useSessionState<string | null>('tools.selectedName', null)
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('swarmgo.toolsListOpen')
  const [query, setQuery] = useState('')
  // List filters: a set of visibility tiers (empty = all) and an enabled/disabled
  // status filter. Independent of the search query — they narrow the same list.
  const [visFilter, setVisFilter] = useState<Set<ToolVisibility>>(new Set())
  const [statusFilter, setStatusFilter] = useState<'all' | 'enabled' | 'disabled'>('all')
  const toggleVisFilter = (tier: ToolVisibility) =>
    setVisFilter((prev) => {
      const next = new Set(prev)
      if (next.has(tier)) next.delete(tier)
      else next.add(tier)
      return next
    })
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
        setVisOv(res.toolVisibility ?? {})
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

  // Set one tool's context-visibility tier (full / summary / name-only / hidden)
  // for the whole workspace. Persists an explicit override into the visibility map
  // — pinning the tool to that tier regardless of its code default. A hidden/
  // name-only tool stays active; only how much of it rides in the per-turn context
  // changes. Mirrors a skill's visibility state.
  const setToolVisibility = async (t: WorkspaceTool, tier: ToolVisibility) => {
    if (t.visibility === tier) return
    const nextOv = { ...visOv, [t.name]: tier }
    setVisBusy(t.name)
    // Optimistic update.
    setTools((ts) => ts.map((x) => (x.name === t.name ? { ...x, visibility: tier } : x)))
    setVisOv(nextOv)
    try {
      await api.setWorkspaceToolVisibility(nextOv)
    } catch (e) {
      onError((e as Error).message)
      loadTools() // revert on failure
    } finally {
      setVisBusy(null)
    }
  }

  // Set the same visibility tier for EVERY tool of one MCP server in a single
  // action — the per-server counterpart of the per-tool tier selector. The manual
  // override for externally-added MCP servers (whose tools default to name-only).
  const setServerVisibility = async (serverName: string, tier: ToolVisibility) => {
    const names = tools
      .filter((t) => toolSource(t) === 'mcp' && (toolServer(t) || 'MCP') === serverName)
      .map((t) => t.name)
    if (names.length === 0) return
    const nameSet = new Set(names)
    const nextOv = { ...visOv }
    for (const n of names) nextOv[n] = tier
    // Optimistic update.
    setTools((ts) => ts.map((x) => (nameSet.has(x.name) ? { ...x, visibility: tier } : x)))
    setVisOv(nextOv)
    try {
      await api.setWorkspaceToolVisibility(nextOv)
    } catch (e) {
      onError((e as Error).message)
      loadTools() // revert on failure
    }
  }

  const addServer = async () => {
    if (!name.trim()) return
    try {
      // Split args on whitespace; quotes are not parsed (keep it simple).
      const args = argsText.trim() ? argsText.trim().split(/\s+/) : []
      // Parse "Key: Value" lines into a headers map (http transport only). The
      // first colon splits; later colons (e.g. in a URL value) stay in the value.
      const headers: Record<string, string> = {}
      if (transport === 'http') {
        for (const line of headersText.split('\n')) {
          const t = line.trim()
          if (!t) continue
          const i = t.indexOf(':')
          if (i <= 0) continue
          headers[t.slice(0, i).trim()] = t.slice(i + 1).trim()
        }
      }
      await api.createMCPServer({
        name: name.trim(),
        transport,
        command: command.trim(),
        args,
        url: url.trim(),
        headers: Object.keys(headers).length ? headers : undefined,
      })
      setName('')
      setCommand('')
      setArgsText('')
      setUrl('')
      setHeadersText('')
      loadServers()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const importServers = async () => {
    const text = importText.trim()
    if (!text) return
    setImporting(true)
    setImportMsg('')
    try {
      const res = await api.importMCPServers(text)
      const errCount = Object.keys(res.errors ?? {}).length
      const parts = [`${res.created.length} sunucu eklendi`]
      if (errCount > 0) {
        const detail = Object.entries(res.errors)
          .map(([n, e]) => `${n}: ${e}`)
          .join('; ')
        parts.push(`${errCount} hata — ${detail}`)
      }
      setImportMsg(parts.join(' · '))
      if (res.created.length > 0) setImportText('')
      loadServers()
    } catch (e) {
      setImportMsg(`✗ ${(e as Error).message}`)
    } finally {
      setImporting(false)
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

  // Filter by the search query (label/name/description), the visibility-tier set
  // (empty = all tiers), and the enabled/disabled status filter.
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return tools.filter((t) => {
      if (
        q &&
        !(
          toolLabel(t).toLowerCase().includes(q) ||
          t.name.toLowerCase().includes(q) ||
          (t.description ?? '').toLowerCase().includes(q)
        )
      )
        return false
      if (visFilter.size && !visFilter.has(t.visibility)) return false
      if (statusFilter === 'enabled' && !t.enabled) return false
      if (statusFilter === 'disabled' && t.enabled) return false
      return true
    })
  }, [tools, query, visFilter, statusFilter])
  const filtersActive = visFilter.size > 0 || statusFilter !== 'all'

  // Group filtered tools: built-ins by functional category (fixed order), then one
  // group per MCP server (alphabetical). Empty categories are skipped.
  const groups = useMemo(() => {
    const byCategory = new Map<string, WorkspaceTool[]>()
    const byServer = new Map<string, WorkspaceTool[]>()
    for (const t of filtered) {
      if (toolSource(t) === 'mcp') {
        const key = toolServer(t) || 'MCP'
        if (!byServer.has(key)) byServer.set(key, [])
        byServer.get(key)!.push(t)
      } else {
        const key = toolCategory(t)
        if (!byCategory.has(key)) byCategory.set(key, [])
        byCategory.get(key)!.push(t)
      }
    }
    const out: { label: string; tools: WorkspaceTool[] }[] = []
    // Built-in categories in fixed order; any unknown key appended after, by label.
    const seen = new Set<string>()
    const emit = (cat: string) => {
      const list = byCategory.get(cat)
      if (!list || !list.length || seen.has(cat)) return
      seen.add(cat)
      out.push({ label: CATEGORY_LABELS[cat] ?? cat, tools: list })
    }
    for (const cat of CATEGORY_ORDER) emit(cat)
    for (const cat of [...byCategory.keys()].sort((a, b) =>
      (CATEGORY_LABELS[a] ?? a).localeCompare(CATEGORY_LABELS[b] ?? b),
    )) {
      emit(cat)
    }
    // MCP servers after built-ins, alphabetical.
    for (const [server, list] of [...byServer.entries()].sort((a, b) => a[0].localeCompare(b[0]))) {
      out.push({ label: server, tools: list })
    }
    return out
  }, [filtered])

  // Multi-select (Ctrl/Cmd+Click, Shift-range) for bulk enable/disable + NameOnly.
  // Plain click still opens the tool's detail; modifier-click selects instead.
  const sel = useMultiSelect()
  const orderedNames = useMemo(
    () => groups.flatMap((g) => (collapsed.has(g.label) ? [] : g.tools.map((t) => t.name))),
    [groups, collapsed],
  )
  const bulkSetEnabled = async (enabled: boolean) => {
    if (sel.count === 0) return
    const ids = [...sel.selected]
    const next = enabled
      ? disabled.filter((n) => !sel.selected.has(n))
      : [...new Set([...disabled, ...ids])]
    setTools((ts) => ts.map((x) => (sel.selected.has(x.name) ? { ...x, enabled } : x)))
    setDisabled(next)
    sel.clear()
    try {
      await api.setWorkspaceTools(next)
    } catch (e) {
      onError((e as Error).message)
      loadTools()
    }
  }
  const bulkSetVisibility = async (tier: ToolVisibility) => {
    if (sel.count === 0) return
    const nextOv = { ...visOv }
    for (const n of sel.selected) nextOv[n] = tier
    setTools((ts) => ts.map((x) => (sel.selected.has(x.name) ? { ...x, visibility: tier } : x)))
    setVisOv(nextOv)
    sel.clear()
    try {
      await api.setWorkspaceToolVisibility(nextOv)
    } catch (e) {
      onError((e as Error).message)
      loadTools()
    }
  }

  const selected = tools.find((t) => t.name === selectedName) ?? null
  const params = useMemo(() => extractParams(selected?.inputSchema), [selected])

  return (
    <div className="flex h-full min-h-0 flex-1">
      {/* Left: searchable, grouped tool list — full-height sibling column (like chat). */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="swarmgo.toolsListWidth"
        defaultWidth={288}
        label="Araçlar"
        testId="tools-list-toggle"
        hideRail
      >
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
          {/* Filters: visibility tiers (multi-select) + enabled/disabled status. */}
          <div className="mt-2 flex flex-wrap items-center gap-1">
            {VISIBILITY_TIERS.map((tier) => {
              const on = visFilter.has(tier.value)
              return (
                <button
                  key={tier.value}
                  data-testid="tools-filter-vis"
                  data-tier={tier.value}
                  data-on={on}
                  onClick={() => toggleVisFilter(tier.value)}
                  title={`Görünürlük: ${tier.label}`}
                  className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide transition"
                  style={
                    on
                      ? { backgroundColor: `color-mix(in srgb, ${tier.color} 22%, transparent)`, color: tier.color }
                      : { backgroundColor: 'var(--color-surface-2)', color: 'var(--color-text-dim)' }
                  }
                >
                  {tier.label}
                </button>
              )
            })}
            <span className="mx-0.5 h-3 w-px bg-[var(--color-border)]" />
            {(['enabled', 'disabled'] as const).map((st) => {
              const on = statusFilter === st
              return (
                <button
                  key={st}
                  data-testid="tools-filter-status"
                  data-status={st}
                  data-on={on}
                  onClick={() => setStatusFilter((prev) => (prev === st ? 'all' : st))}
                  className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide transition ${
                    on
                      ? st === 'enabled'
                        ? 'bg-[color-mix(in_srgb,var(--color-success)_22%,transparent)] text-[var(--color-success)]'
                        : 'bg-[var(--color-surface-2)] text-[var(--color-text)] ring-1 ring-inset ring-[var(--color-border)]'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                  }`}
                >
                  {st === 'enabled' ? 'Aktif' : 'Devre dışı'}
                </button>
              )
            })}
            {filtersActive && (
              <button
                data-testid="tools-filter-clear"
                onClick={() => {
                  setVisFilter(new Set())
                  setStatusFilter('all')
                }}
                title="Filtreleri temizle"
                className="rounded px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
              >
                Temizle
              </button>
            )}
          </div>
          <p className="mt-2 px-0.5 text-xs text-[var(--color-text-dim)]">
            {filtersActive ? `${filtered.length} eşleşme · ` : ''}
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
                        onClick={(e) => {
                          if (sel.handleClick(e, t.name, orderedNames, selectedName)) return
                          setSelectedName(t.name)
                        }}
                        className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm transition ${
                          sel.isSelected(t.name)
                            ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)] ring-1 ring-inset ring-[var(--color-accent)]'
                            : active
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
                        <VisibilityBadge visibility={t.visibility} className="ml-auto" />
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

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={orderedNames.length ? () => sel.selectAll(orderedNames) : undefined}
        >
          <SelectionBarButton icon={<Check size={13} />} onClick={() => bulkSetEnabled(true)}>
            Etkinleştir
          </SelectionBarButton>
          <SelectionBarButton icon={<Ban size={13} />} onClick={() => bulkSetEnabled(false)}>
            Devre dışı
          </SelectionBarButton>
          {VISIBILITY_TIERS.map((tier) => (
            <SelectionBarButton key={tier.value} onClick={() => bulkSetVisibility(tier.value)}>
              {tier.label}
            </SelectionBarButton>
          ))}
        </SelectionBar>
      </ListPane>

      {/* Right column: title bar + selected tool detail / MCP server management.
          The title bar sits ONLY here, to the right of the list — like the chat header. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          title="Araçlar & MCP"
          subtitle={selected ? `· ${selected.name}` : undefined}
          listOpen={listOpen}
          onToggleList={toggleList}
        />
        <div className="flex-1 overflow-y-auto p-6">
        {selected ? (
          <ToolDetail
            tool={selected}
            params={params}
            saving={savingTool === selected.name}
            visBusy={visBusy === selected.name}
            onToggle={() => toggleTool(selected)}
            onSetVisibility={(tier) => setToolVisibility(selected, tier)}
          />
        ) : (
          <ServerManagement
            servers={servers}
            tools={tools}
            onServerVisibility={setServerVisibility}
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
            headersText={headersText}
            setHeadersText={setHeadersText}
            onAdd={addServer}
            onToggle={toggleServer}
            onTest={testServer}
            onRemove={removeServer}
            importText={importText}
            setImportText={setImportText}
            importing={importing}
            importMsg={importMsg}
            onImport={importServers}
          />
        )}
      </div>
      </div>
    </div>
  )
}

// VisibilityBadge shows a tool's current context-visibility tier (Tam / Özet /
// İsim / Gizli) with a tier-accent color. Replaces the old NameOnly/Self-mgmt
// chips — every tool now carries exactly one of the four.
function VisibilityBadge({
  visibility,
  className = '',
}: {
  visibility: ToolVisibility
  className?: string
}) {
  const m = visibilityMeta(visibility)
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${className}`}
      style={{
        backgroundColor: `color-mix(in srgb, ${m.color} 18%, transparent)`,
        color: m.color,
      }}
      title={m.hint}
    >
      {m.label}
    </span>
  )
}

// VisibilitySelector is the 4-way segmented control to pick a tool's context tier.
function VisibilitySelector({
  value,
  busy,
  onSelect,
}: {
  value: ToolVisibility
  busy: boolean
  onSelect: (tier: ToolVisibility) => void
}) {
  return (
    <div className="inline-flex overflow-hidden rounded-lg border border-[var(--color-border)]">
      {VISIBILITY_TIERS.map((tier) => {
        const active = value === tier.value
        return (
          <button
            key={tier.value}
            data-testid="tool-visibility-tier"
            data-tier={tier.value}
            onClick={() => onSelect(tier.value)}
            disabled={busy}
            title={tier.hint}
            className={`px-3 py-2 text-xs font-medium transition disabled:opacity-50 ${
              active
                ? 'text-white'
                : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
            }`}
            style={active ? { backgroundColor: tier.color } : undefined}
          >
            {tier.label}
          </button>
        )
      })}
    </div>
  )
}

// ToolDetail renders the right-hand detail view for one selected tool.
function ToolDetail({
  tool,
  params,
  saving,
  visBusy,
  onToggle,
  onSetVisibility,
}: {
  tool: WorkspaceTool
  params: ParamRow[]
  saving: boolean
  visBusy: boolean
  onToggle: () => void
  onSetVisibility: (tier: ToolVisibility) => void
}) {
  const ToolIcon = toolIcon(tool.name)
  const examples = (tool.examples ?? []) as unknown[]
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
            <VisibilityBadge visibility={tool.visibility} />
          </div>
        </div>
        <div className="flex flex-shrink-0 items-center gap-2">
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
          Bağlam görünürlüğü
        </h3>
        <VisibilitySelector value={tool.visibility} busy={visBusy} onSelect={onSetVisibility} />
        <p className="mt-2 text-xs text-[var(--color-text-dim)]">{visibilityMeta(tool.visibility).hint}</p>
      </section>

      <section>
        <h3 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Açıklama
        </h3>
        <p className="whitespace-pre-wrap text-sm leading-relaxed">
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

      {examples.length > 0 && (
        <section>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Örnek çağrılar ({examples.length})
          </h3>
          <p className="mb-2 text-xs text-[var(--color-text-dim)]">
            Şemanın ifade edemediği kullanım kalıpları (tarih/ID biçimi, birlikte gelen alanlar).
            Bunlar yalnızca <b>Tam</b> görünürlükte (tam şemayla) modele gider.
          </p>
          <div className="space-y-2">
            {examples.map((ex, i) => (
              <pre
                key={i}
                className="overflow-x-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs"
              >
                {JSON.stringify(ex, null, 2)}
              </pre>
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

// ServerManagement is the MCP server list + add form, shown when no tool is
// selected. (Extracted so the right pane stays readable.)
function ServerManagement(props: {
  servers: MCPServer[]
  tools: WorkspaceTool[]
  onServerVisibility: (server: string, tier: ToolVisibility) => void
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
  headersText: string
  setHeadersText: (v: string) => void
  onAdd: () => void
  onToggle: (s: MCPServer) => void
  onTest: (s: MCPServer) => void
  onRemove: (s: MCPServer) => void
  importText: string
  setImportText: (v: string) => void
  importing: boolean
  importMsg: string
  onImport: () => void
}) {
  const {
    servers,
    tools,
    onServerVisibility,
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
    headersText,
    setHeadersText,
    onAdd,
    onToggle,
    onTest,
    onRemove,
    importText,
    setImportText,
    importing,
    importMsg,
    onImport,
  } = props
  return (
    <div className="mx-auto max-w-2xl">
      <h2 className="mb-1 text-sm font-semibold">MCP Sunucuları</h2>
      <p className="mb-3 text-xs text-[var(--color-text-dim)]">
        Soldaki listeden bir araç seçerek detaylarını görüntüleyip aç/kapatabilirsin.
      </p>
      <div className="space-y-2">
        {servers.map((s) => {
          // This server's MCP tools — drives the per-server visibility quick action
          // (set the whole server's tools to one tier at once).
          const serverTools = tools.filter(
            (t) => toolSource(t) === 'mcp' && (toolServer(t) || 'MCP') === s.name,
          )
          return (
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
            {serverTools.length > 0 && (
              <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] pt-2">
                <span className="text-xs text-[var(--color-text-dim)]">
                  Tüm araçlar ({serverTools.length}) →
                </span>
                {VISIBILITY_TIERS.map((tier) => (
                  <button
                    key={tier.value}
                    data-testid="mcp-server-visibility-all"
                    data-server-id={s.id}
                    data-tier={tier.value}
                    onClick={() => onServerVisibility(s.name, tier.value)}
                    title={tier.hint}
                    className="rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs hover:opacity-90"
                  >
                    {tier.label}
                  </button>
                ))}
              </div>
            )}
            {testResult[s.id] && (
              <div className="mt-2 break-words text-xs text-[var(--color-text-dim)]">{testResult[s.id]}</div>
            )}
          </div>
          )
        })}
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
            <option value="http">http (Streamable HTTP)</option>
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
            <>
              <input
                data-testid="mcp-server-url-input"
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="URL (Streamable HTTP endpoint)"
                className="col-span-2 rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <textarea
                data-testid="mcp-server-headers-input"
                value={headersText}
                onChange={(e) => setHeadersText(e.target.value)}
                spellCheck={false}
                placeholder={'Opsiyonel başlıklar — her satıra "Anahtar: Değer"\nör. Authorization: Bearer TOKEN'}
                className="col-span-2 h-20 resize-y rounded bg-[var(--color-surface-2)] px-3 py-2 font-mono text-xs outline-none"
              />
            </>
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

      {/* Bulk import from a pasted mcpServers JSON document. */}
      <div className="mt-4 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
        <h3 className="mb-1 text-xs font-semibold text-[var(--color-text-dim)]">JSON ile içe aktar</h3>
        <p className="mb-2 text-xs text-[var(--color-text-dim)]">
          Standart <code className="text-[var(--color-text)]">mcpServers</code> JSON'u yapıştır (Claude Code /
          .mcp.json biçimi). Birden çok sunucu tek seferde eklenir.
        </p>
        <textarea
          data-testid="mcp-import-textarea"
          value={importText}
          onChange={(e) => setImportText(e.target.value)}
          spellCheck={false}
          placeholder={
            '{\n  "mcpServers": {\n    "playwright": {\n      "command": "bunx",\n      "args": ["@playwright/mcp", "--browser", "chrome"]\n    }\n  }\n}'
          }
          className="h-40 w-full resize-y rounded bg-[var(--color-surface-2)] px-3 py-2 font-mono text-xs outline-none"
        />
        <div className="mt-2 flex items-center gap-3">
          <button
            data-testid="mcp-import-button"
            onClick={onImport}
            disabled={importing || !importText.trim()}
            className="rounded-lg bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {importing ? 'İçe aktarılıyor…' : 'İçe aktar'}
          </button>
          {importMsg && <span className="break-words text-xs text-[var(--color-text-dim)]">{importMsg}</span>}
        </div>
      </div>
    </div>
  )
}
