import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from 'react'
import { useNodesState, useEdgesState, type Edge } from '@xyflow/react'
import { api } from '@/api'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import type { FlowNodeEvent } from '@/api/flows'
import { TemplatePreview } from './TemplatePreview'
import { RunTreeView } from './RunTreeView'
import { FLOW_TEMPLATES } from './flowTemplates'
import {
  graphToReactFlow,
  reactFlowToGraph,
  canonicalGraphKey,
  type FlowRFNode,
} from './flowGraph'
import type { EdgeStyle } from './FlowCanvas'
import { safeParse, type FlowsTab } from './flowsPanelShared'
import { createFlowGraphOps } from './flowGraphOps'
import { createFlowActions } from './flowActions'
import { FlowsListPane } from './FlowsListPane'
import { FlowsHeader } from './FlowsHeader'
import { FlowEditorView } from './FlowEditorView'
import type { Agent, Flow, FlowRun, FlowState } from '@/types'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { useSessionState } from '@/shared/hooks/useSessionState'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
  // Deep-link: when set, open this flow's run history (from the Activity screen).
  openFlowId?: string | null
  // Active left-column tab, driven by the URL (#/w/{ws}/flows/{tab}); null/unknown
  // → "flows". onTabChange writes it back so the hash reflects the current tab.
  tab?: string | null
  onTabChange?: (tab: string | null) => void
}

// FlowsPanel is the visual protocol builder: pick a flow, edit it on a drag-and-
// drop node canvas (React Flow), save, run with an input, and watch per-node
// progress stream live on the canvas and in the trace below. The panel owns the
// state; the list column (FlowsListPane), top bar (FlowsHeader), editor body
// (FlowEditorView) and the action factories (flowActions / flowGraphOps) render
// and mutate it.
export function FlowsPanel({ agents, onError, openFlowId, tab: tabProp, onTabChange }: Props) {
  const [flows, setFlows] = useState<Flow[]>([])
  // Selection + active tab persist across screen switches within the session
  // (reset on app reload). The selected flow's editor state is re-loaded on mount
  // by the restore effect below.
  const [selectedId, setSelectedId] = useSessionState<string | null>('flows.selectedId', null)
  // Absolute path of the selected flow's on-disk JSON file (for copy / reveal).
  const [flowPath, setFlowPath] = useState('')
  // Left-column tab: own flows, read-only template gallery, or run history. Driven
  // by the URL (deep-linkable, #/w/{ws}/flows/{tab}); the parent owns the value so
  // the hash and the tab stay in sync. Unknown/absent → "flows".
  const tab: FlowsTab = tabProp === 'templates' || tabProp === 'runs' ? tabProp : 'flows'
  // Matches the useState setter shape consumers expect (value OR updater), but
  // routes the result to the URL-owning parent instead of local state.
  const setTab = useCallback<Dispatch<SetStateAction<FlowsTab>>>(
    (t) => onTabChange?.(typeof t === 'function' ? t(tab) : t),
    [onTabChange, tab],
  )
  const [templateId, setTemplateId] = useSessionState<string | null>('flows.templateId', null)
  // Run history (all flows, newest first) + the selected run for the read-only viewer.
  const [runs, setRuns] = useState<FlowRun[]>([])
  // Runs tab filter: off (default) lists only root runs, so one run of a composed
  // flow is one row instead of a burst of its subflow/spawn children. The children
  // are not lost — they are still reachable by id and through the run tree.
  const [showSubRuns, setShowSubRuns] = useSessionState('flows.showSubRuns', false)
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
  // Accumulate mode: sequential agent nodes share a growing conversation thread
  // (prompt-cache reuse). Stored per-flow in the graph; defaults ON for flows that
  // never set it (a new flow or a pre-feature one).
  const [accumulate, setAccumulate] = useState(true)
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

  // True until the first flow list lands — the list column shows a loading state
  // rather than the "no flows yet" copy.
  const [flowsLoading, setFlowsLoading] = useState(true)

  const loadFlows = useCallback(() => {
    api
      .listFlows()
      .then(setFlows)
      .catch((e) => onError(e.message))
      .finally(() => setFlowsLoading(false))
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
  // in-progress runs advance live. Polling stops when leaving the tab. Toggling
  // showSubRuns re-runs the effect, so the list switches filter immediately
  // instead of waiting out the current poll interval.
  useEffect(() => {
    if (tab !== 'runs') return
    let alive = true
    const tick = () => {
      api
        .listAllFlowRuns(!showSubRuns)
        .then((rs) => alive && setRuns(rs))
        .catch(() => {})
    }
    tick()
    const id = setInterval(tick, 3000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [tab, showSubRuns])

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
        // Default ON unless the flow explicitly stored accumulate:false. The raw
        // JSON distinguishes an absent field (undefined → default on) from an
        // explicit false (→ off), which the backend now persists verbatim.
        setAccumulate(g.accumulate === undefined ? true : !!g.accumulate)
      } catch {
        setNodes([])
        setEdges([])
        setStart('')
        setAnimated(false)
        setAccumulate(true)
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

  // Multi-select (Ctrl/Cmd+Click, Shift-range) on the "Akışlarım" tab for bulk
  // run / delete. Runs fire-and-forget with an empty input.
  const sel = useMultiSelect()
  // Left flow list collapse (slim rail / mobile drawer).
  const { open: flowsListOpen, toggle: toggleFlowsList, setOpen: setFlowsListOpen } = useCollapsibleList('tionswarm.flowsListOpen')
  // Landing on the screen with no flow selected: open the list drawer so a narrow
  // screen shows the pickable flow list instead of an empty canvas. Runs once on
  // mount; on md+ the list is always visible so this is a no-op there.
  useEffect(() => {
    if (!selectedId) setFlowsListOpen(true)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  // Node editor popup: clicking a node (not dragging) opens a modal to edit it,
  // instead of a docked side panel. Closing keeps the node selected on canvas.
  const [nodeEditorOpen, setNodeEditorOpen] = useState(false)
  const openNodeEditor = (id: string) => {
    setSelectedNodeId(id)
    setNodeEditorOpen(true)
  }

  // Canvas node operations (add / patch / start / delete / duplicate / arrange),
  // re-created each render over the live editor state — exactly like the former
  // inline definitions.
  const ops = createFlowGraphOps({
    agents,
    nodes,
    setNodes,
    edges,
    setEdges,
    start,
    setStart,
    selectedNodeId,
    setSelectedNodeId,
  })

  // Flow-level actions (create / instantiate / save / emoji / delete / bulk / run).
  const actions = createFlowActions({
    agents,
    selectedId,
    setSelectedId,
    setFlows,
    loadFlows,
    selectFlow,
    setTab,
    name,
    setEmoji,
    nodes,
    edges,
    start,
    edgeStyle,
    animated,
    accumulate,
    input,
    setRunning,
    runs,
    setRuns,
    rootOnlyRuns: !showSubRuns,
    setSelectedRunId,
    sel,
    onError,
  })

  // Tag edits persist immediately (setFlowTags) and sync the list array so the
  // flow row's tag chips refresh live.
  const handleTagsChange = (next: string[]) => {
    setTags(next)
    if (!selectedId) return
    setFlows((prev) => prev.map((x) => (x.id === selectedId ? { ...x, tags: next } : x)))
    api.setFlowTags(selectedId, next).catch((e) => onError((e as Error).message))
  }

  // rerunRun re-executes an already-finished run's flow with the SAME input
  // (Koşular tab). It streams so the run-list refreshes live, then selects the
  // freshly produced run in the viewer. Uses the CURRENT flow definition.
  const rerunRun = useCallback(
    async (r: FlowRun) => {
      setRerunning(true)
      const refresh = () => api.listAllFlowRuns(!showSubRuns).then(setRuns).catch(() => {})
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
    [onError, showSubRuns],
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
      <FlowsListPane
        flowsListOpen={flowsListOpen}
        toggleFlowsList={toggleFlowsList}
        tab={tab}
        setTab={setTab}
        q={q}
        setQ={setQ}
        flows={flows}
        flowsLoading={flowsLoading}
        runs={runs}
        showSubRuns={showSubRuns}
        setShowSubRuns={setShowSubRuns}
        templateId={templateId}
        setTemplateId={setTemplateId}
        selectedId={selectedId}
        selectedRunId={selectedRunId}
        setSelectedRunId={setSelectedRunId}
        allTags={allTags}
        tagFilter={tagFilter}
        toggleTagFilter={toggleTagFilter}
        setTagFilter={setTagFilter}
        sel={sel}
        selectFlow={selectFlow}
        createFlow={actions.createFlow}
        removeFlow={actions.removeFlow}
        bulkRun={actions.bulkRun}
        bulkDelete={actions.bulkDelete}
      />

      {/* Main column: the title bar sits ONLY here (right of the list), like chat. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <FlowsHeader
          flowsListOpen={flowsListOpen}
          toggleFlowsList={toggleFlowsList}
          tab={tab}
          flows={flows}
          selectedId={selectedId}
          selectedTemplate={selectedTemplate}
          selectedRun={selectedRun}
          emoji={emoji}
          changeEmoji={actions.changeEmoji}
          name={name}
          setName={setName}
          flowPath={flowPath}
          saveFlow={actions.saveFlow}
          instantiateTemplate={actions.instantiateTemplate}
          rerunRun={rerunRun}
          rerunning={rerunning}
          onError={onError}
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
            <RunTreeView
              run={selectedRun}
              flows={flows}
              agents={agents}
              onRerun={rerunRun}
              rerunning={rerunning}
              onResumed={() => api.listAllFlowRuns(!showSubRuns).then(setRuns).catch(() => {})}
            />
          )
        ) : !selectedId ? (
          <div className="flex-1 p-6">
            <p className="text-sm text-[var(--color-text-dim)]">
              Soldan bir akış seçin veya yeni bir akış oluşturun.
            </p>
          </div>
        ) : (
          <FlowEditorView
            agents={agents}
            flows={flows.filter((f) => f.id !== selectedId)}
            nodes={nodes}
            edges={edges}
            edgeStyle={edgeStyle}
            animated={animated}
            setAnimated={setAnimated}
            accumulate={accumulate}
            setAccumulate={setAccumulate}
            changeEdgeStyle={changeEdgeStyle}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            setEdges={setEdges}
            setSelectedNodeId={setSelectedNodeId}
            openNodeEditor={openNodeEditor}
            addNode={ops.addNode}
            addNodeAt={ops.addNodeAt}
            autoArrange={ops.autoArrange}
            paletteOpen={paletteOpen}
            togglePalette={togglePalette}
            paletteVisible={paletteVisible}
            togglePaletteVisible={togglePaletteVisible}
            tags={tags}
            onTagsChange={handleTagsChange}
            nodeEditorOpen={nodeEditorOpen}
            setNodeEditorOpen={setNodeEditorOpen}
            selectedNode={selectedNode}
            start={start}
            patchSelected={ops.patchSelected}
            duplicateSelected={ops.duplicateSelected}
            deleteSelected={ops.deleteSelected}
            runInputRef={runInputRef}
            input={input}
            setInput={setInput}
            doRun={actions.doRun}
            running={running}
            run={run}
            liveNodes={liveNodes}
            trace={trace}
          />
        )}
      </div>
    </div>
  )
}
