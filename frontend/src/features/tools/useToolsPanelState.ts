import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSessionState } from '@/shared/hooks/useSessionState'
import { api } from '@/api'
import type { MCPServer, MCPTransport, ToolVisibility, WorkspaceTool } from '@/types'
import {
  toolSource,
  toolServer,
  toolLabel,
  toolCategory,
  CATEGORY_LABELS,
  CATEGORY_ORDER,
  extractParams,
} from './toolMeta'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'

// useToolsPanelState holds all state, data loading, and mutation handlers for
// the ToolsPanel screen (extracted so the component file stays readable).
export function useToolsPanelState(onError: (msg: string) => void) {
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
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionswarm.toolsListOpen')
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
      return new Set<string>(JSON.parse(localStorage.getItem('tionswarm.toolsCollapsed') || '[]'))
    } catch {
      return new Set<string>()
    }
  })
  const toggleGroup = (label: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(label)) next.delete(label)
      else next.add(label)
      localStorage.setItem('tionswarm.toolsCollapsed', JSON.stringify([...next]))
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

  return {
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
    headersText,
    setHeadersText,
    importText,
    setImportText,
    importing,
    importMsg,
    tools,
    savingTool,
    visBusy,
    selectedName,
    setSelectedName,
    listOpen,
    toggleList,
    query,
    setQuery,
    visFilter,
    setVisFilter,
    statusFilter,
    setStatusFilter,
    toggleVisFilter,
    collapsed,
    toggleGroup,
    toggleTool,
    setToolVisibility,
    setServerVisibility,
    addServer,
    importServers,
    toggleServer,
    testServer,
    removeServer,
    activeCount,
    filtered,
    filtersActive,
    groups,
    sel,
    orderedNames,
    bulkSetEnabled,
    bulkSetVisibility,
    selected,
    params,
  }
}
