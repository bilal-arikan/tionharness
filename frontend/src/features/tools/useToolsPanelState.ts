import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSessionState } from '@/shared/hooks/useSessionState'
import { api } from '@/api'
import type {
  MCPServer,
  MCPTransport,
  MCPPoolStats,
  ToolVisibility,
  WorkspaceTool,
  ImportableMCPServer,
} from '@/types'
import {
  toolSource,
  toolServer,
  toolLabel,
  toolCategory,
  CATEGORY_LABELS,
  CATEGORY_ORDER,
  extractParams,
  parseArgs,
} from './toolMeta'
import { toast } from '@/shared/components'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { useVisiblePoll } from '@/shared/hooks/useVisiblePoll'

// The MCP pool snapshot has no SSE signal, so this interval IS the update path —
// but it only drives an indicator, so it stays coarse and visibility-gated.
const POOL_STATS_POLL_MS = 10000

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
  // Connection scope: "shared" = one workspace-wide connection (default);
  // "scoped" = a separate idle-evicted connection per (session, agent).
  const [scope, setScope] = useState<'shared' | 'scoped'>('shared')
  // When set, the add form is editing this server (PATCH) instead of creating.
  const [editingId, setEditingId] = useState<string | null>(null)

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
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.toolsListOpen')
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
      return new Set<string>(JSON.parse(localStorage.getItem('tionharness.toolsCollapsed') || '[]'))
    } catch {
      return new Set<string>()
    }
  })
  const toggleGroup = (label: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(label)) next.delete(label)
      else next.add(label)
      localStorage.setItem('tionharness.toolsCollapsed', JSON.stringify([...next]))
      return next
    })
  }

  const loadServers = useCallback(() => {
    api
      .listMCPServers()
      .then(setServers)
      .catch((e) => onError(e.message))
  }, [onError])

  // Live pool snapshot for the reaper / live-connection indicator. Polled on a
  // light interval (best-effort; failures are swallowed so the panel stays calm).
  const [poolStats, setPoolStats] = useState<MCPPoolStats | null>(null)
  const loadPoolStats = useCallback(() => {
    api
      .mcpPoolStats()
      .then(setPoolStats)
      .catch(() => {})
  }, [])
  useEffect(() => {
    loadPoolStats()
  }, [loadPoolStats])
  useVisiblePoll(loadPoolStats, POOL_STATS_POLL_MS, [loadPoolStats])

  // Servers configured in OTHER workspaces, offered for one-click copy here.
  const [importable, setImportable] = useState<ImportableMCPServer[]>([])
  const [addingImportable, setAddingImportable] = useState<string | null>(null)
  const loadImportable = useCallback(() => {
    api
      .importableMCPServers()
      .then(setImportable)
      .catch((e) => onError(e.message))
  }, [onError])

  // Copy one server from another workspace into this one, then refresh both the
  // local list (its tools appear next turn) and the importable list (it drops out).
  const addImportable = async (item: ImportableMCPServer) => {
    setAddingImportable(item.server.id)
    try {
      await api.addImportableMCPServer(item.workspaceId, item.server.id)
      loadServers()
      loadImportable()
      toast.success('MCP sunucusu eklendi')
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setAddingImportable(null)
    }
  }

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
    const next = t.enabled
      ? [...new Set([...disabled, t.name])]
      : disabled.filter((n) => n !== t.name)
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
      const payload = {
        name: name.trim(),
        transport,
        command: command.trim(),
        args,
        url: url.trim(),
        headers: Object.keys(headers).length ? headers : undefined,
        scope: (scope === 'scoped' ? 'scoped' : undefined) as 'scoped' | undefined,
      }
      // Edit mode (editingId set) PATCHes the existing row; else create.
      const wasEditing = !!editingId
      if (editingId) await api.updateMCPServer(editingId, payload)
      else await api.createMCPServer(payload)
      resetForm()
      loadServers()
      loadPoolStats()
      toast.success(wasEditing ? 'MCP sunucusu güncellendi' : 'MCP sunucusu eklendi')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // resetForm clears the add/edit form and leaves edit mode.
  const resetForm = () => {
    setEditingId(null)
    setName('')
    setCommand('')
    setArgsText('')
    setUrl('')
    setHeadersText('')
    setScope('shared')
    setTransport('stdio')
  }

  // startEdit loads a server's fields into the form and switches it to edit mode.
  const startEdit = (s: MCPServer) => {
    setEditingId(s.id)
    setName(s.name)
    setTransport(s.transport)
    setCommand(s.command)
    setArgsText(parseArgs(s.args).join(' '))
    setUrl(s.url)
    // Render the stored headers JSON back into "Key: Value" lines for the textarea.
    let headerLines = ''
    try {
      const h = JSON.parse(s.headersConfig || '{}') as Record<string, string>
      headerLines = Object.entries(h)
        .map(([k, v]) => `${k}: ${v}`)
        .join('\n')
    } catch {
      headerLines = ''
    }
    setHeadersText(headerLines)
    setScope(s.scope === 'scoped' ? 'scoped' : 'shared')
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
      toast.success('MCP sunucusu silindi')
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
    scope,
    setScope,
    editingId,
    startEdit,
    cancelEdit: resetForm,
    poolStats,
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
    importable,
    addingImportable,
    loadImportable,
    addImportable,
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
