import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { Loader2, XCircle, ChevronDown, ChevronRight, PanelLeftClose, PanelLeftOpen, type LucideIcon } from 'lucide-react'
import { api } from '../../api'
import { EmojiField } from '../common/EmojiField'
import { NODE_ICONS } from '../flow/nodeStyles'
import { normalizeAvatar } from '../../lib/avatar'
import { CopyPathButton } from '../CopyPathButton'
import { RevealButton } from '../RevealButton'
import { useRegisterDirty } from '../../lib/dirtySignals'
import type { FlowNodeEvent } from '../../api/flows'
import { Markdown } from '../markdown/Markdown'
import { FlowCanvas, FLOW_NODE_DND_MIME, type EdgeStyle } from '../flow/FlowCanvas'
import { TemplatePreview } from '../flow/TemplatePreview'
import { RunView, STATUS_LABEL, statusColor } from '../flow/RunView'
import { NodeInspector } from '../flow/NodeInspector'
import { FlowVarsButton } from '../flow/FlowVarsButton'
import { FLOW_TEMPLATES, type FlowTemplate } from '../../lib/flowTemplates'
import {
  graphToReactFlow,
  reactFlowToGraph,
  canonicalGraphKey,
  autoLayout,
  blankNode,
  nextNodeId,
  type FlowRFNode,
} from '../../lib/flowGraph'
import type { Agent, Flow, FlowNode, FlowNodeType, FlowRun, FlowState } from '../../types'
import { Button, SelectionBar, SelectionBarButton, TagEditor, ListPane, PaneHeader, ModalOverlay } from '../common'
import {
  NewItemButton, SELECTED_ITEM_CLS, SELECTED_ITEM_RING,
} from '../common/SidebarChrome'
import { useMultiSelect } from '../../hooks/useMultiSelect'
import { useCollapsibleList } from '../../hooks/useCollapsibleList'
import { useSessionState } from '../../hooks/useSessionState'
import { Play, Trash2, X, RotateCcw } from 'lucide-react'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
  // Deep-link: when set, open this flow's run history (from the Activity screen).
  openFlowId?: string | null
}

// Node palette entries. Icons are the shared monochrome (theme-colored) lucide
// glyphs from nodeStyles, so the palette tree matches the canvas node headers.
const NODE_TYPES: { value: FlowNodeType; label: string; Icon: LucideIcon }[] = [
  { value: 'agent', label: 'Ajan', Icon: NODE_ICONS.agent },
  { value: 'branch', label: 'Dallanma', Icon: NODE_ICONS.branch },
  { value: 'parallel', label: 'Paralel', Icon: NODE_ICONS.parallel },
  { value: 'delay', label: 'Bekle', Icon: NODE_ICONS.delay },
  { value: 'transform', label: 'Birleştir', Icon: NODE_ICONS.transform },
]

const EDGE_STYLES: { value: EdgeStyle; label: string }[] = [
  { value: 'default', label: 'Eğri' },
  { value: 'smoothstep', label: 'Yumuşak' },
  { value: 'step', label: 'Basamak' },
  { value: 'straight', label: 'Düz' },
]

// FlowsPanel is the visual protocol builder: pick a flow, edit it on a drag-and-
// drop node canvas (React Flow), save, run with an input, and watch per-node
// progress stream live on the canvas and in the trace below.
export function FlowsPanel({ agents, onError, openFlowId }: Props) {
  const [flows, setFlows] = useState<Flow[]>([])
  // Selection + active tab persist across screen switches within the session
  // (reset on app reload). The selected flow's editor state is re-loaded on mount
  // by the restore effect below.
  const [selectedId, setSelectedId] = useSessionState<string | null>('flows.selectedId', null)
  // Absolute path of the selected flow's on-disk JSON file (for copy / reveal).
  const [flowPath, setFlowPath] = useState('')
  // Left-column tab: own flows, read-only template gallery, or run history.
  const [tab, setTab] = useSessionState<'flows' | 'templates' | 'runs'>('flows.tab', 'flows')
  const [templateId, setTemplateId] = useSessionState<string | null>('flows.templateId', null)
  // Run history (all flows, newest first) + the selected run for the read-only viewer.
  const [runs, setRuns] = useState<FlowRun[]>([])
  const [selectedRunId, setSelectedRunId] = useSessionState<string | null>('flows.selectedRunId', null)
  // Left-list search (adapts to the active tab: flow/template name, or a run's
  // flow name).
  const [q, setQ] = useState('')
  // Flow tag filter (Akışlarım tab): selected tags a flow must carry (ANY match).
  // Empty = no tag filter.
  const [tagFilter, setTagFilter] = useState<string[]>([])
  // "Node ekle" palette section: vertically collapsible (persisted).
  const [paletteOpen, setPaletteOpen] = useState(
    () => localStorage.getItem('tionswarm.flowPaletteOpen') !== '0',
  )
  const togglePalette = () =>
    setPaletteOpen((o) => {
      const next = !o
      localStorage.setItem('tionswarm.flowPaletteOpen', next ? '1' : '0')
      return next
    })
  // Whole left palette column (Node ekle + Görünüm) show/hide, toggled from the
  // top bar. Persisted; hidden gives the canvas full width.
  const [paletteVisible, setPaletteVisible] = useState(
    () => localStorage.getItem('tionswarm.flowPaletteVisible') !== '0',
  )
  const togglePaletteVisible = () =>
    setPaletteVisible((v) => {
      const next = !v
      localStorage.setItem('tionswarm.flowPaletteVisible', next ? '1' : '0')
      return next
    })
  // Auto-grow the run input upward: the run panel is bottom-anchored (below the
  // flex-1 canvas), so growing the textarea moves its top edge up while its bottom
  // stays put. Height tracks content up to a cap; then the textarea scrolls.
  const runInputRef = useRef<HTMLTextAreaElement>(null)

  // Editor state for the selected flow.
  const [name, setName] = useState('')
  // Cosmetic emoji for the selected flow. Persisted independently (setFlowEmoji),
  // like tags — not via the Save button — so it survives graph/name saves.
  const [emoji, setEmoji] = useState('')
  // Flow tags persist independently (setFlowTags), not via the Save button.
  const [tags, setTags] = useState<string[]>([])
  const [start, setStart] = useState('')
  const [nodes, setNodes, onNodesChange] = useNodesState<FlowRFNode>([])
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  // Edge presentation (cosmetic). Stored per-flow in the graph; localStorage
  // holds the last-used edge style as the default for flows that have none.
  const [edgeStyle, setEdgeStyle] = useState<EdgeStyle>(
    () => (localStorage.getItem('tionswarm.flowEdgeStyle') as EdgeStyle) || 'default',
  )
  const [animated, setAnimated] = useState(false)
  const changeEdgeStyle = (s: EdgeStyle) => {
    setEdgeStyle(s)
    localStorage.setItem('tionswarm.flowEdgeStyle', s)
  }

  // Run state.
  const [input, setInput] = useState('')
  const [running, setRunning] = useState(false)
  const [run, setRun] = useState<FlowRun | null>(null)
  const [liveNodes, setLiveNodes] = useState<FlowNodeEvent[]>([])
  // True while a re-run kicked off from the Koşular tab (RunView) is in flight.
  const [rerunning, setRerunning] = useState(false)

  const loadFlows = useCallback(() => {
    api.listFlows().then(setFlows).catch((e) => onError(e.message))
  }, [onError])

  useEffect(() => loadFlows(), [loadFlows])

  // Resize the run input to fit its content (grows upward, capped at 160px).
  useEffect(() => {
    const el = runInputRef.current
    if (!el) return
    el.style.height = 'auto'
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`
  }, [input, selectedId])

  // While the Koşular tab is open, load all flow runs and poll every 3s so
  // in-progress runs advance live. Polling stops when leaving the tab.
  useEffect(() => {
    if (tab !== 'runs') return
    let alive = true
    const tick = () => {
      api
        .listAllFlowRuns()
        .then((rs) => alive && setRuns(rs))
        .catch(() => {})
    }
    tick()
    const id = setInterval(tick, 3000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [tab])

  // Deep-link from the Activity screen: open this flow's run history and select
  // its latest run (runs are newest-first). Consumed once per target so polling
  // doesn't keep re-selecting.
  const consumedFlowTarget = useRef<string | null>(null)
  useEffect(() => {
    if (openFlowId) setTab('runs')
  }, [openFlowId])
  useEffect(() => {
    if (!openFlowId || tab !== 'runs' || consumedFlowTarget.current === openFlowId) return
    const latest = runs.find((r) => r.flowId === openFlowId)
    if (latest) {
      setSelectedRunId(latest.id)
      consumedFlowTarget.current = openFlowId
    }
  }, [openFlowId, tab, runs])

  const selectFlow = useCallback(
    (f: Flow) => {
      setSelectedId(f.id)
      setFlowPath('')
      api.flowPath(f.id).then((r) => setFlowPath(r.path)).catch(() => setFlowPath(''))
      setName(f.name)
      setEmoji(f.emoji ?? '')
      setTags(f.tags ?? [])
      setRun(null)
      setInput('')
      setLiveNodes([])
      setSelectedNodeId(null)
      try {
        const g = f.graph ? JSON.parse(f.graph) : { start: '', nodes: [] }
        const { nodes: rn, edges: re } = graphToReactFlow({
          start: g.start ?? '',
          nodes: Array.isArray(g.nodes) ? g.nodes : [],
        })
        setNodes(rn)
        setEdges(re)
        setStart(g.start ?? '')
        setEdgeStyle(
          (g.edgeStyle as EdgeStyle) ||
            ((localStorage.getItem('tionswarm.flowEdgeStyle') as EdgeStyle) || 'default'),
        )
        setAnimated(!!g.animated)
      } catch {
        setNodes([])
        setEdges([])
        setStart('')
        setAnimated(false)
      }
    },
    [setNodes, setEdges, setSelectedId],
  )

  // Restore the session-persisted flow selection on mount: once flows load, if a
  // flow was selected earlier this session, re-load its editor state (runs once —
  // later user clicks are unaffected).
  const didRestoreRef = useRef(false)
  useEffect(() => {
    if (didRestoreRef.current || flows.length === 0 || !selectedId) return
    didRestoreRef.current = true
    const f = flows.find((x) => x.id === selectedId)
    if (f) selectFlow(f)
  }, [flows, selectedId, selectFlow])

  const createFlow = async () => {
    const n = prompt('Akış adı:')
    if (!n) return
    try {
      const f = await api.createFlow(n)
      setFlows((prev) => [f, ...prev])
      selectFlow(f)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // instantiateTemplate creates a new editable flow from a template's graph,
  // then switches to it for editing. Template agent nodes are unassigned; the
  // backend requires every agent node to have an agentId, so we seed each with
  // the first available agent as a placeholder for the user to reassign.
  const instantiateTemplate = async (t: FlowTemplate) => {
    const defaultAgent = agents[0]?.id
    if (!defaultAgent) {
      onError('Şablondan akış oluşturmak için önce en az bir ajan oluşturun.')
      return
    }
    const graph = {
      ...t.graph,
      nodes: t.graph.nodes.map((n) =>
        n.type === 'agent' && !n.agentId ? { ...n, agentId: defaultAgent } : n,
      ),
    }
    try {
      const f = await api.createFlow(t.name, graph)
      setFlows((prev) => [f, ...prev])
      setTab('flows')
      selectFlow(f)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // addNodeAt appends a blank node of the given type at a specific canvas
  // position and makes it the start node if none is set yet.
  const addNodeAt = (type: FlowNodeType, pos: { x: number; y: number }) => {
    const existing = nodes.map((n) => n.data.node)
    const id = nextNodeId(existing)
    const node = blankNode(id, type, agents[0]?.id ?? '')
    const rf: FlowRFNode = {
      id,
      type,
      position: pos,
      data: { node, isStart: !start },
    }
    setNodes((prev) => [...prev, rf])
    if (!start) setStart(id)
  }

  // addNode (click on the palette) drops the new node near the canvas origin,
  // cascaded so successive adds don't stack exactly on top of each other.
  const addNode = (type: FlowNodeType) => {
    const offset = nodes.length * 30
    addNodeAt(type, { x: 80 + offset, y: 80 + offset })
  }

  // patchSelected updates the selected node's intrinsic fields. A type change or
  // a shrunk branch list prunes now-invalid outgoing edges so the graph stays
  // consistent on save.
  const patchSelected = (patch: Partial<FlowNode>) => {
    if (!selectedNodeId) return
    let prune = false
    setNodes((prev) =>
      prev.map((rn) => {
        if (rn.id !== selectedNodeId) return rn
        const before = rn.data.node
        const merged = { ...before, ...patch }
        if (patch.type && patch.type !== before.type) prune = true
        if (
          patch.branches &&
          patch.branches.length < (before.branches?.length ?? 0)
        )
          prune = true
        return { ...rn, type: merged.type, data: { ...rn.data, node: merged } }
      }),
    )
    if (prune) {
      setEdges((eds) => eds.filter((e) => e.source !== selectedNodeId))
    }
  }

  // Node actions are id-based so both the inspector (on the selected node) and
  // each node's NodeToolbar can invoke them.
  const makeStartNode = (id: string) => {
    setStart(id)
    setNodes((prev) => prev.map((rn) => ({ ...rn, data: { ...rn.data, isStart: rn.id === id } })))
  }

  const deleteNode = (id: string) => {
    setNodes((prev) => prev.filter((rn) => rn.id !== id))
    setEdges((eds) => eds.filter((e) => e.source !== id && e.target !== id))
    if (start === id) setStart('')
    if (selectedNodeId === id) setSelectedNodeId(null)
  }

  // duplicateNode clones a node (new id, offset position, not start) without its
  // connections — the copy starts unwired.
  const duplicateNode = (id: string) => {
    setNodes((prev) => {
      const src = prev.find((rn) => rn.id === id)
      if (!src) return prev
      const newId = nextNodeId(prev.map((rn) => rn.data.node))
      const copy: FlowRFNode = {
        id: newId,
        type: src.type,
        position: { x: src.position.x + 40, y: src.position.y + 40 },
        data: { node: { ...src.data.node, id: newId }, isStart: false },
      }
      return [...prev, copy]
    })
  }

  const makeStart = () => {
    if (selectedNodeId) makeStartNode(selectedNodeId)
  }
  const deleteSelected = () => {
    if (selectedNodeId) deleteNode(selectedNodeId)
  }

  // autoArrange re-lays-out the graph with the layered grid algorithm and
  // applies the computed positions to the canvas nodes.
  const autoArrange = () => {
    const graph = reactFlowToGraph(nodes, edges, start)
    const pos = autoLayout(graph)
    setNodes((prev) =>
      prev.map((rn) => (pos[rn.id] ? { ...rn, position: { x: pos[rn.id].x, y: pos[rn.id].y } } : rn)),
    )
  }

  const saveFlow = async () => {
    if (!selectedId) return
    try {
      const graph = reactFlowToGraph(nodes, edges, start)
      graph.edgeStyle = edgeStyle
      graph.animated = animated
      const f = await api.updateFlow(selectedId, name, graph)
      setFlows((prev) => prev.map((x) => (x.id === f.id ? f : x)))
      onError('') // clear
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // changeEmoji persists the selected flow's emoji immediately (independent of
  // the Save button) and syncs the local list so the glyph updates everywhere.
  const changeEmoji = async (next: string) => {
    setEmoji(next) // optimistic
    if (!selectedId) return
    try {
      await api.setFlowEmoji(selectedId, next)
      setFlows((prev) => prev.map((x) => (x.id === selectedId ? { ...x, emoji: next } : x)))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const removeFlow = async (f: Flow) => {
    if (!confirm(`"${f.name}" akışı silinsin mi?`)) return
    try {
      await api.deleteFlow(f.id)
      if (selectedId === f.id) setSelectedId(null)
      loadFlows()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // Multi-select (Ctrl/Cmd+Click, Shift-range) on the "Akışlarım" tab for bulk
  // run / delete. Runs fire-and-forget with an empty input.
  const sel = useMultiSelect()
  // Left flow list collapse (slim rail / mobile drawer).
  const { open: flowsListOpen, toggle: toggleFlowsList } = useCollapsibleList('tionswarm.flowsListOpen')
  // Node editor popup: clicking a node (not dragging) opens a modal to edit it,
  // instead of a docked side panel. Closing keeps the node selected on canvas.
  const [nodeEditorOpen, setNodeEditorOpen] = useState(false)
  const openNodeEditor = (id: string) => {
    setSelectedNodeId(id)
    setNodeEditorOpen(true)
  }
  const duplicateSelected = () => {
    if (selectedNodeId) duplicateNode(selectedNodeId)
  }
  const bulkRun = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.runFlow(id, '')))
    } catch (e) {
      onError((e as Error).message)
    }
  }
  const bulkDelete = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (!confirm(`${ids.length} akış silinsin mi?`)) return
    if (selectedId && sel.selected.has(selectedId)) setSelectedId(null)
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.deleteFlow(id)))
      loadFlows()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // setNodeStatus paints a node's live run state (running glow / done ring /
  // error ring).
  const setNodeStatus = (nodeId: string, status: 'running' | 'done' | 'error' | undefined) => {
    setNodes((prev) =>
      prev.map((rn) => (rn.id === nodeId ? { ...rn, data: { ...rn.data, status } } : rn)),
    )
  }

  const doRun = async () => {
    if (!selectedId) return
    setRunning(true)
    setRun(null)
    setLiveNodes([])
    setNodes((prev) => prev.map((rn) => ({ ...rn, data: { ...rn.data, status: undefined } })))
    try {
      await saveFlow() // persist edits before running
      await api.runFlowStreamStandalone(selectedId, input, {
        onNode: (ev) => {
          const status = ev.phase === 'start' ? 'running' : ev.phase === 'error' ? 'error' : 'done'
          setNodeStatus(ev.nodeId, status)
          setLiveNodes((prev) => {
            if (ev.phase === 'start') return [...prev, ev]
            // done/error: replace the pending entry for this node (still running).
            const i = prev.findIndex((n) => n.nodeId === ev.nodeId && n.output === undefined && n.error === undefined)
            if (i < 0) return [...prev, ev]
            const next = [...prev]
            next[i] = ev
            return next
          })
        },
        onReply: (r) => setRun(r.run),
        onError: (e) => onError(e),
      })
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  // rerunRun re-executes an already-finished run's flow with the SAME input
  // (Koşular tab). It streams so the run-list refreshes live, then selects the
  // freshly produced run in the viewer. Uses the CURRENT flow definition.
  const rerunRun = useCallback(
    async (r: FlowRun) => {
      setRerunning(true)
      const refresh = () => api.listAllFlowRuns().then(setRuns).catch(() => {})
      try {
        await api.runFlowStreamStandalone(r.flowId, r.input, {
          // Surface the new running run in the left list as it progresses.
          onNode: () => refresh(),
          onReply: (res) => {
            // Upsert the finished run so selection is instant (no poll-gap flicker).
            setRuns((prev) => {
              const rest = prev.filter((x) => x.id !== res.run.id)
              return [res.run, ...rest]
            })
            setSelectedRunId(res.run.id)
            refresh()
          },
          onError: (e) => onError(e),
        })
      } catch (e) {
        onError((e as Error).message)
      } finally {
        setRerunning(false)
        refresh()
      }
    },
    [onError],
  )

  // Unsaved-edits (dirty) signal for the nav "Akışlar" item + workspace label:
  // compare the live editor (name + structural graph) to the stored flow.
  // Cosmetic-only fields (edgeStyle/animated) are ignored so they don't raise a
  // false amber dot. Best-effort — a parse failure reads as "not dirty".
  const flowDirty = useMemo(() => {
    const stored = flows.find((f) => f.id === selectedId)
    if (!selectedId || !stored) return false
    if (name !== stored.name) return true
    try {
      // Compare live canvas vs stored via canonicalGraphKey, which round-trips
      // both sides through the same graphToReactFlow → reactFlowToGraph pipeline.
      // This absorbs the backend's `omitempty` marshaling (dropping next:"",
      // x/y:0, empty prompt…) so a freshly opened, unedited flow isn't flagged
      // dirty — only real structural edits differ.
      const cur = canonicalGraphKey(reactFlowToGraph(nodes, edges, start))
      const raw = stored.graph ? JSON.parse(stored.graph) : { start: '', nodes: [] }
      return cur !== canonicalGraphKey(raw)
    } catch {
      return false
    }
  }, [flows, selectedId, name, nodes, edges, start])
  useRegisterDirty('flows', flowDirty)

  // Unique, sorted tags across all flows — the pool of chips for the tag filter.
  const allTags = useMemo(() => {
    const set = new Set<string>()
    flows.forEach((f) => f.tags?.forEach((t) => set.add(t)))
    return [...set].sort((a, b) => a.localeCompare(b))
  }, [flows])
  const toggleTagFilter = (t: string) =>
    setTagFilter((prev) => (prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]))

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)?.data.node ?? null
  const trace: FlowState | null = run?.state ? safeParse(run.state) : null
  const selectedTemplate = FLOW_TEMPLATES.find((t) => t.id === templateId) ?? null
  // Derived from the polled `runs` list, so the selected run refreshes live.
  const selectedRun = runs.find((r) => r.id === selectedRunId) ?? null

  return (
    <div className="flex h-full min-h-0 flex-1">
      {/* Flow list / template gallery — full-height sibling column (like chat). */}
      <ListPane
        open={flowsListOpen}
        onToggle={toggleFlowsList}
        widthKey="tionswarm.flowsListWidth"
        defaultWidth={224}
        minWidth={180}
        label="Akışlar"
        testId="flows-list-toggle"
        hideRail
      >
        <div className="min-h-0 flex-1 overflow-y-auto p-3">
        {/* Tab switch */}
        <div className="mb-3 flex gap-1 rounded-lg bg-[var(--color-surface-2)] p-1 text-xs">
          {(['flows', 'templates', 'runs'] as const).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`flex-1 rounded-md px-1.5 py-1 ${
                tab === t ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
              }`}
            >
              {t === 'flows' ? 'Akışlarım' : t === 'templates' ? 'Şablonlar' : 'Koşular'}
            </button>
          ))}
        </div>

        {/* Search (filters the active tab's list). */}
        <input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={tab === 'runs' ? 'Koşu ara (akış adı)…' : tab === 'templates' ? 'Şablon ara…' : 'Akış ara…'}
          className="mb-2 w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
        />

        {tab === 'templates' ? (
          <ul className="space-y-1">
            {FLOW_TEMPLATES.filter((t) => t.name.toLowerCase().includes(q.trim().toLowerCase())).map((t) => (
              <li key={t.id}>
                <button
                  onClick={() => setTemplateId(t.id)}
                  className={`w-full rounded-lg px-3 py-2 text-left text-sm ${
                    templateId === t.id
                      ? SELECTED_ITEM_CLS
                      : 'hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span className="block truncate">{t.name}</span>
                  <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                    {t.description}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : tab === 'runs' ? (
          <ul className="space-y-1">
            {runs
              .filter((rn) => {
                const qq = q.trim().toLowerCase()
                if (!qq) return true
                return (flows.find((f) => f.id === rn.flowId)?.name ?? '').toLowerCase().includes(qq)
              })
              .map((rn) => {
              const rflow = flows.find((f) => f.id === rn.flowId)
              const fname = rflow?.name ?? '（silinmiş akış）'
              const femoji = normalizeAvatar(rflow?.emoji)
              const badge =
                rn.status === 'success' ? '✓' : rn.status === 'failure' ? '✕' : '▶'
              const badgeColor =
                rn.status === 'success'
                  ? 'text-[var(--color-success)]'
                  : rn.status === 'failure'
                    ? 'text-[var(--color-danger)]'
                    : 'text-[var(--color-accent)]'
              return (
                <li key={rn.id}>
                  <button
                    onClick={() => setSelectedRunId(rn.id)}
                    className={`flex w-full items-start gap-2 rounded-lg px-3 py-2 text-left text-sm ${
                      selectedRunId === rn.id
                        ? SELECTED_ITEM_CLS
                        : 'hover:bg-[var(--color-surface-2)]'
                    }`}
                  >
                    <span className={`mt-0.5 text-xs ${badgeColor}`}>{badge}</span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate">
                        {femoji && <span className="mr-1 leading-none">{femoji}</span>}
                        {fname}
                      </span>
                      <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                        {new Date(rn.createdAt * 1000).toLocaleString()}
                      </span>
                    </span>
                  </button>
                </li>
              )
            })}
            {runs.length === 0 && (
              <li className="text-sm text-[var(--color-text-dim)]">Henüz koşu yok.</li>
            )}
          </ul>
        ) : (
          <>
        <NewItemButton bare onClick={createFlow} label="Yeni akış" className="mb-3" />
        {/* Tag filter chips: click to narrow the list to flows carrying any of the
            selected tags. Only shown when at least one flow has a tag. */}
        {allTags.length > 0 && (
          <div className="mb-2 flex flex-wrap items-center gap-1">
            {allTags.map((t) => {
              const on = tagFilter.includes(t)
              return (
                <button
                  key={t}
                  onClick={() => toggleTagFilter(t)}
                  className={`rounded-full px-1.5 py-0.5 text-[10px] transition ${
                    on
                      ? 'bg-[var(--color-accent)] text-white'
                      : 'bg-[var(--color-accent-soft)] text-[var(--color-accent)] hover:opacity-80'
                  }`}
                  title={on ? 'Filtreyi kaldır' : 'Bu etikete göre filtrele'}
                >
                  #{t}
                </button>
              )
            })}
            {tagFilter.length > 0 && (
              <button
                onClick={() => setTagFilter([])}
                className="text-[10px] text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                title="Etiket filtresini temizle"
              >
                temizle
              </button>
            )}
          </div>
        )}
        {(() => {
          const visible = flows.filter(
            (f) =>
              f.name.toLowerCase().includes(q.trim().toLowerCase()) &&
              (tagFilter.length === 0 || tagFilter.some((t) => f.tags?.includes(t))),
          )
          const orderedIds = visible.map((f) => f.id)
          return (
        <ul className="space-y-1">
          {visible.map((f) => (
            <li key={f.id}>
              <button
                onClick={(e) => {
                  if (sel.handleClick(e, f.id, orderedIds, selectedId)) return
                  selectFlow(f)
                }}
                className={`flex w-full items-start justify-between rounded-lg px-3 py-2 text-left text-sm ${
                  sel.isSelected(f.id)
                    ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                    : selectedId === f.id
                      ? SELECTED_ITEM_CLS
                      : 'hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5">
                    {normalizeAvatar(f.emoji) && (
                      <span className="shrink-0 leading-none">{normalizeAvatar(f.emoji)}</span>
                    )}
                    <span className="truncate">{f.name}</span>
                  </span>
                  <span className="mt-0.5 flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                    <span className="truncate font-mono text-[11px]">{f.id}</span>
                    <span className="flex-shrink-0">· {flowNodeCount(f.graph)} node</span>
                  </span>
                  {(f.tags?.length ?? 0) > 0 && (
                    <span className="mt-1 flex flex-wrap gap-1">
                      {f.tags!.slice(0, 4).map((t) => (
                        <span
                          key={t}
                          className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]"
                        >
                          #{t}
                        </span>
                      ))}
                      {f.tags!.length > 4 && (
                        <span className="text-[10px] text-[var(--color-text-dim)]">+{f.tags!.length - 4}</span>
                      )}
                    </span>
                  )}
                </span>
                <span
                  onClick={(e) => {
                    // Let modifier-clicks bubble up to the selection handler.
                    if (e.ctrlKey || e.metaKey || e.shiftKey) return
                    e.stopPropagation()
                    removeFlow(f)
                  }}
                  className="ml-2 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                >
                  ✕
                </span>
              </button>
            </li>
          ))}
          {visible.length === 0 && (
            <li className="text-sm text-[var(--color-text-dim)]">
              {flows.length === 0 ? 'Henüz akış yok.' : 'Eşleşen akış yok.'}
            </li>
          )}
        </ul>
          )
        })()}
        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
        >
          <SelectionBarButton icon={<Play size={13} />} onClick={bulkRun}>
            Çalıştır
          </SelectionBarButton>
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
          </>
        )}
        </div>
      </ListPane>

      {/* Main column: the title bar sits ONLY here (right of the list), like chat. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          listOpen={flowsListOpen}
          onToggleList={toggleFlowsList}
          // Detail views (flow editor / template preview / run inspector) put their
          // own toolbar directly in the top bar via titleSlot + right — no redundant
          // "Akışlar" title / subtitle. Empty/list states keep the "Akışlar" title.
          title={
            (tab === 'flows' && selectedId) ||
            (tab === 'templates' && selectedTemplate) ||
            (tab === 'runs' && selectedRun)
              ? undefined
              : 'Akışlar'
          }
          titleSlot={
            tab === 'flows' && selectedId ? (
              <>
                <EmojiField value={emoji} onChange={changeEmoji} clearLabel="🔀" />
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Akış adı"
                  className="min-w-0 flex-1 rounded bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium outline-none"
                />
                <span
                  className="flex-shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
                  title="Akış ID (dosya adı)"
                >
                  {selectedId}
                </span>
              </>
            ) : tab === 'templates' && selectedTemplate ? (
              <span className="flex min-w-0 flex-col leading-tight">
                <span className="truncate text-sm font-medium">{selectedTemplate.name}</span>
                {selectedTemplate.description && (
                  <span className="truncate text-xs text-[var(--color-text-dim)]">
                    {selectedTemplate.description}
                  </span>
                )}
              </span>
            ) : tab === 'runs' && selectedRun ? (
              <span className="flex min-w-0 items-center gap-2">
                <span className="truncate text-sm font-medium">
                  {(() => {
                    const rf = flows.find((f) => f.id === selectedRun.flowId)
                    const e = normalizeAvatar(rf?.emoji)
                    return `${e ? e + ' ' : ''}${rf?.name ?? '（silinmiş akış）'}`
                  })()}
                </span>
                <span className={`flex-shrink-0 text-xs ${statusColor(selectedRun.status)}`}>
                  {STATUS_LABEL[selectedRun.status] ?? selectedRun.status}
                </span>
                <span className="flex-shrink-0 text-xs text-[var(--color-text-dim)]">
                  {new Date(selectedRun.createdAt * 1000).toLocaleString()}
                </span>
              </span>
            ) : undefined
          }
          right={
            tab === 'flows' && selectedId ? (
              <>
                <CopyPathButton path={flowPath} label="Yolu kopyala" labelClassName="hidden" title="Akış yolunu kopyala" />
                <RevealButton
                  onReveal={() => {
                    if (selectedId) api.revealFlow(selectedId).catch((e) => onError((e as Error).message))
                  }}
                  disabled={!selectedId}
                  label="Aç"
                  labelClassName="hidden sm:inline"
                  title="Akış klasörünü aç"
                />
                <Button onClick={saveFlow} size="lg" className="flex-shrink-0">
                  Kaydet
                </Button>
              </>
            ) : tab === 'templates' && selectedTemplate ? (
              <>
                <span className="hidden text-xs text-[var(--color-text-dim)] sm:inline">
                  salt-okunur önizleme
                </span>
                <Button onClick={() => instantiateTemplate(selectedTemplate)} size="lg" className="flex-shrink-0">
                  + Bu şablondan akış oluştur
                </Button>
              </>
            ) : tab === 'runs' && selectedRun ? (
              <button
                type="button"
                onClick={() => rerunRun(selectedRun)}
                disabled={rerunning || selectedRun.status === 'running' || !flows.some((f) => f.id === selectedRun.flowId)}
                title={
                  !flows.some((f) => f.id === selectedRun.flowId)
                    ? 'Akış silinmiş — tekrar çalıştırılamaz'
                    : 'Bu koşuyu aynı girdiyle tekrar çalıştır'
                }
                className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-50"
              >
                <RotateCcw size={13} className={rerunning ? 'animate-spin' : ''} />
                {rerunning ? 'Çalışıyor…' : 'Tekrar çalıştır'}
              </button>
            ) : undefined
          }
        />
        {/* Main: template preview or flow editor */}
        {tab === 'templates' ? (
        <div className="flex min-w-0 flex-1 flex-col">
          {!selectedTemplate ? (
            <div className="flex-1 p-6">
              <p className="text-sm text-[var(--color-text-dim)]">
                Soldan bir şablon seçin — yapısını önizleyin, sonra "Bu şablondan akış oluştur" deyin.
              </p>
            </div>
          ) : (
            // Template name/description + actions now live in the top PaneHeader.
            <div className="min-h-0 flex-1">
              <TemplatePreview graph={selectedTemplate.graph} agents={agents} />
            </div>
          )}
        </div>
      ) : tab === 'runs' ? (
        !selectedRun ? (
          <div className="flex-1 p-6">
            <p className="text-sm text-[var(--color-text-dim)]">
              Soldan bir koşu seçin — akışın hangi aşamada olduğunu, node çıktılarını ve
              hataları salt-okunur görün.
            </p>
          </div>
        ) : (
          <RunView
            run={selectedRun}
            flow={flows.find((f) => f.id === selectedRun.flowId)}
            agents={agents}
            onRerun={rerunRun}
            rerunning={rerunning}
            hideSummary
          />
        )
      ) : !selectedId ? (
        <div className="flex-1 p-6">
          <p className="text-sm text-[var(--color-text-dim)]">
            Soldan bir akış seçin veya yeni bir akış oluşturun.
          </p>
        </div>
      ) : (
        <div className="flex min-w-0 flex-1 flex-col">
          {/* Node palette (add nodes + flow-level presentation) + canvas. The flow
              name/id + file actions + save now live in the top PaneHeader above. */}
          <div className="flex min-h-0 flex-1">
            {paletteVisible && (
            <div className="w-40 flex-shrink-0 space-y-2 overflow-y-auto border-r border-[var(--color-border)] p-2 max-md:w-32">
              <button
                type="button"
                onClick={togglePalette}
                className="flex w-full items-center gap-1 px-1 text-xs font-semibold text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                title={paletteOpen ? 'Node ekle bölümünü daralt' : 'Node ekle bölümünü genişlet'}
                aria-expanded={paletteOpen}
              >
                {paletteOpen ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
                Node ekle
              </button>
              {paletteOpen && NODE_TYPES.map((t) => (
                <button
                  key={t.value}
                  draggable
                  onDragStart={(e) => {
                    e.dataTransfer.setData(FLOW_NODE_DND_MIME, t.value)
                    e.dataTransfer.effectAllowed = 'move'
                  }}
                  onClick={() => addNode(t.value)}
                  className="flex w-full cursor-grab items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-2 text-left text-xs hover:border-[var(--color-accent)] active:cursor-grabbing"
                >
                  <t.Icon size={15} className="shrink-0 text-[var(--color-text-dim)]" />
                  <span>{t.label}</span>
                </button>
              ))}

              {/* Flow-level presentation moved here from the meta toolbar. */}
              <div className="space-y-2 border-t border-[var(--color-border)] pt-2">
                <div className="px-1 text-xs font-semibold text-[var(--color-text-dim)]">
                  Görünüm
                </div>
                <div className="px-0.5">
                  <span className="mb-1 block px-0.5 text-[11px] text-[var(--color-text-dim)]">Etiket</span>
                  <TagEditor
                    tags={tags}
                    onChange={(next) => {
                      setTags(next)
                      if (!selectedId) return
                      // Sync the list array too so the flow row's tag chips refresh live.
                      setFlows((prev) => prev.map((x) => (x.id === selectedId ? { ...x, tags: next } : x)))
                      api.setFlowTags(selectedId, next).catch((e) => onError((e as Error).message))
                    }}
                    placeholder="Etiket…"
                    className="py-1"
                  />
                </div>
                <label className="block px-0.5">
                  <span className="mb-1 block px-0.5 text-[11px] text-[var(--color-text-dim)]">Kablo</span>
                  <select
                    value={edgeStyle}
                    onChange={(e) => changeEdgeStyle(e.target.value as EdgeStyle)}
                    className="w-full rounded bg-[var(--color-surface-2)] px-2 py-1.5 text-xs outline-none"
                  >
                    {EDGE_STYLES.map((s) => (
                      <option key={s.value} value={s.value}>
                        {s.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="flex cursor-pointer items-center gap-1.5 px-0.5 text-xs text-[var(--color-text-dim)]">
                  <input
                    type="checkbox"
                    checked={animated}
                    onChange={(e) => setAnimated(e.target.checked)}
                  />
                  Animasyon
                </label>
              </div>
            </div>
            )}
            <div className="relative min-w-0 flex-1">
              {/* Floating top-left toggle to hide/show the whole left palette,
                  overlaid on the canvas (React Flow's own toolbar is top-right,
                  Controls bottom-left, so top-left is free). */}
              <button
                type="button"
                onClick={togglePaletteVisible}
                aria-pressed={paletteVisible}
                title={paletteVisible ? 'Sol paneli gizle' : 'Sol paneli göster'}
                className="absolute left-2 top-2 z-10 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1.5 text-[var(--color-text-dim)] shadow-lg transition hover:text-[var(--color-accent)]"
              >
                {paletteVisible ? <PanelLeftClose size={16} /> : <PanelLeftOpen size={16} />}
              </button>
              <FlowCanvas
                agents={agents}
                nodes={nodes}
                edges={edges}
                edgeStyle={edgeStyle}
                animated={animated}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                setEdges={setEdges}
                onSelect={setSelectedNodeId}
                onNodeClick={openNodeEditor}
                onDropNode={addNodeAt}
                onAutoLayout={autoArrange}
              />
            </div>
          </div>

          {/* Node editor popup — opens on node click (not drag). Holds the node
              fields plus its make-start / duplicate / delete actions. */}
          {nodeEditorOpen && selectedNode && (
            <ModalOverlay onClose={() => setNodeEditorOpen(false)}>
              <div className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl">
                <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-2.5">
                  <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                    Node
                  </span>
                  <button
                    onClick={() => setNodeEditorOpen(false)}
                    title="Kapat"
                    aria-label="Kapat"
                    className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                  >
                    <X size={16} />
                  </button>
                </div>
                <div className="min-h-0 flex-1 overflow-y-auto p-4">
                  <NodeInspector
                    node={selectedNode}
                    agents={agents}
                    isStart={start === selectedNode.id}
                    allNodes={nodes.map((n) => n.data.node)}
                    onPatch={patchSelected}
                    onMakeStart={makeStart}
                    onDuplicate={duplicateSelected}
                    onDelete={deleteSelected}
                  />
                </div>
              </div>
            </ModalOverlay>
          )}

          {/* Run panel. MobileNavBar clearance is handled globally by <main>'s
              max-md bottom padding, so no extra padding is needed here (an earlier
              pb-24 created dead space + scroll). Tight top padding so the input row
              hugs the canvas above it. */}
          <div className="max-h-[40%] overflow-y-auto border-t border-[var(--color-border)] px-4 pb-4 pt-2">
            {/* Variable helper (ℹ️) sits to the LEFT of the run input; its content
                shows in a floating balloon (portal), so the row stays a simple
                centered [ℹ️][input][Çalıştır] line. The run panel is bottom-anchored,
                so the input (and the area around it) grows upward as it gets taller. */}
            <div className="flex items-center gap-2">
              <FlowVarsButton
                context="seed"
                nodeRefs={[]}
                onInsert={(t) => setInput((v) => v + t)}
              />
              <textarea
                ref={runInputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="Girdi (akışa {{input}} olarak geçer)"
                rows={1}
                className="max-h-40 min-w-0 flex-1 resize-none overflow-y-auto rounded bg-[var(--color-surface-2)] px-3 py-2 text-sm outline-none"
              />
              <Button onClick={doRun} disabled={running} size="lg">
                {running ? 'Çalışıyor…' : '▶ Çalıştır'}
              </Button>
            </div>

            {/* Live node progress while running (before the final run lands). */}
            {!run && liveNodes.length > 0 && (
              <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                <div className="mb-2 text-xs text-[var(--color-text-dim)]">Canlı ilerleme</div>
                <ol className="space-y-2">
                  {liveNodes.map((n, i) => (
                    <li key={`${n.nodeId}-${i}`} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                      <div className="mb-1 flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                        {n.error !== undefined ? (
                          <XCircle size={12} className="text-[var(--color-danger)]" />
                        ) : n.output === undefined ? (
                          <Loader2 size={12} className="animate-spin text-[var(--color-accent)]" />
                        ) : null}
                        {i + 1}. [{n.type}] {n.title}
                      </div>
                      {n.error !== undefined ? (
                        <span className="text-xs whitespace-pre-wrap text-[var(--color-danger)]">⚠️ {n.error}</span>
                      ) : n.output === undefined ? (
                        <span className="text-xs italic text-[var(--color-text-dim)]">çalışıyor…</span>
                      ) : n.type === 'branch' ? (
                        <div className="whitespace-pre-wrap">{n.output}</div>
                      ) : (
                        <Markdown>{n.output}</Markdown>
                      )}
                    </li>
                  ))}
                </ol>
              </div>
            )}

            {run && (
              <div className="mt-4 border-t border-[var(--color-border)] pt-3">
                <div className="mb-2 text-xs">
                  Durum:{' '}
                  <span className={run.status === 'success' ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}>
                    {run.status}
                  </span>
                  {run.error && <span className="ml-2 text-[var(--color-danger)]">· {run.error}</span>}
                </div>
                <ol className="space-y-2">
                  {(trace?.trace ?? []).map((t, i) => (
                    <li key={i} className="rounded bg-[var(--color-surface-2)] p-2 text-sm">
                      <div className="mb-1 text-xs text-[var(--color-text-dim)]">
                        {i + 1}. [{t.type}] {t.title}
                      </div>
                      {t.type === 'branch' ? (
                        <div className="whitespace-pre-wrap">{t.output}</div>
                      ) : (
                        <Markdown>{t.output}</Markdown>
                      )}
                    </li>
                  ))}
                </ol>
              </div>
            )}
          </div>
        </div>
      )}
      </div>
    </div>
  )
}

function safeParse(s: string): FlowState | null {
  try {
    return JSON.parse(s) as FlowState
  } catch {
    return null
  }
}

// flowNodeCount reads how many nodes a flow's stored graph holds, for the list
// meta line. Best-effort — an unparseable graph reads as 0.
function flowNodeCount(graph: string): number {
  try {
    const g = JSON.parse(graph || '{}')
    return Array.isArray(g.nodes) ? g.nodes.length : 0
  } catch {
    return 0
  }
}
