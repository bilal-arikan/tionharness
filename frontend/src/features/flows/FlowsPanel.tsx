import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, GitBranch, PanelRightOpen, Save, Sparkles, X } from 'lucide-react'
import type { Connection } from '@xyflow/react'
import type {
  Agent,
  AgentPromptVersion,
  Flow,
  FlowEdge,
  FlowGraph,
  FlowNode,
  FlowNodeType,
  FlowProposal,
  FlowRun,
  FlowVersion,
  Automation,
} from '@/types'
import { api } from '@/api'
import {
  Badge,
  Button,
  EmptyState,
  InfoPopover,
  ListPane,
  PaneHeader,
  ModalOverlay,
} from '@/shared/components'
import { SidebarHeader, RefreshButton, SELECTED_ITEM_CLS } from '@/shared/components/SidebarChrome'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { useSessionState } from '@/shared/hooks/useSessionState'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { useVisiblePoll } from '@/shared/hooks/useVisiblePoll'
import { useViewport } from '@/shared/hooks/useViewport'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { subscribeFlowNode } from '@/shared/lib/flowNodeBus'
import { SIGNAL_FLOWS } from '@/app/eventToRefreshSignals'
import { compareText } from '@/shared/lib/intl'
import { FlowCanvas } from './FlowCanvas'
import { NodeInspector } from './NodeInspector'
import { RunsTab } from './RunsTab'
import { EvolutionTab } from './EvolutionTab'
import { TestTab } from './TestTab'
import {
  applyPositions,
  autoLayout,
  canonicalKey,
  defaultNode,
  freshEdgeId,
  freshNodeId,
  layoutKey,
  lint,
  needsLayout,
  parseGraph,
  toReactFlow,
  type FlowRFNode,
  type NodeStatus,
} from './flowGraph'

type Tab = 'canvas' | 'runs' | 'evolution' | 'test'
const TABS: Tab[] = ['canvas', 'runs', 'evolution', 'test']
const RUNS_POLL_MS = 5000

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
  tab?: string | null
  onTabChange?: (tab: string | null) => void
  onOpenSession?: (sessionId: string) => void
}

function tabOf(raw: string | null | undefined): Tab {
  return raw === 'runs' || raw === 'evolution' || raw === 'test' ? raw : 'canvas'
}

// FlowsPanel is the evolving-flow screen: one flow per agent on the left, and
// for the selected flow a node canvas with an inspector, its runs, its
// evolution (policy, proposals, versions, prompt versions) and a test runner.
// Layout follows the viewport: docked columns on wide landscape screens, a
// drawer list + stacked panes + bottom-sheet inspector on narrow, square and
// portrait screens.
export function FlowsPanel({ agents, onError, tab: tabProp, onTabChange, onOpenSession }: Props) {
  const { t } = useTranslation('flows')
  const tick = useRefreshTrigger(SIGNAL_FLOWS)
  const { tier, aspect } = useViewport()
  const stacked = tier === 'narrow' || tier === 'square' || aspect === 'portrait'
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.flowsListOpen')

  const [flows, setFlows] = useState<Flow[]>([])
  const [loadingFlows, setLoadingFlows] = useState(true)
  const [selectedId, setSelectedId] = useSessionState<string | null>('flows.selectedId', null)
  const [q, setQ] = useState('')
  const tab = tabOf(tabProp)
  const setTab = useCallback(
    (next: Tab) => onTabChange?.(next === 'canvas' ? null : next),
    [onTabChange],
  )

  // Draft graph of the selected flow (canvas state).
  const [draft, setDraft] = useState<FlowGraph | null>(null)
  const [savedKey, setSavedKey] = useState('')
  const [savedLayout, setSavedLayout] = useState('')
  const [draftFlowId, setDraftFlowId] = useState<string | null>(null)
  const [selNode, setSelNode] = useState<string | null>(null)
  const [selEdge, setSelEdge] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [saveOpen, setSaveOpen] = useState(false)
  const [reason, setReason] = useState('')
  const [statuses, setStatuses] = useState<Map<string, NodeStatus>>(new Map())

  // Per-flow data for the other tabs.
  const [runs, setRuns] = useState<FlowRun[]>([])
  const [runsLoading, setRunsLoading] = useState(true)
  const [versions, setVersions] = useState<FlowVersion[]>([])
  const [proposals, setProposals] = useState<FlowProposal[]>([])
  const [promptVersions, setPromptVersions] = useState<AgentPromptVersion[]>([])

  // Fall back to the first flow when nothing (or something gone) is selected —
  // derived, so no effect has to write the selection back.
  const selected = useMemo(() => {
    if (flows.length === 0) return null
    return flows.find((f) => f.id === selectedId) ?? flows[0]
  }, [flows, selectedId])
  const agentNames = useMemo(() => new Map(agents.map((a) => [a.id, a.name])), [agents])
  // Automations feed the trigger node's picker and the canvas labels; a load
  // failure only leaves the picker empty.
  const [automations, setAutomations] = useState<Automation[]>([])
  useEffect(() => {
    let cancelled = false
    api
      .listAutomations()
      .then((list) => {
        if (!cancelled) setAutomations(list)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [tick])
  const automationNames = useMemo(
    () => new Map(automations.map((a) => [a.id, a.name || a.id])),
    [automations],
  )

  const loadFlows = useCallback(() => {
    api
      .listFlows()
      .then((list) => {
        setFlows(list)
        setLoadingFlows(false)
      })
      .catch((e: Error) => {
        setLoadingFlows(false)
        onError(e.message)
      })
  }, [onError])

  useEffect(() => {
    loadFlows()
  }, [loadFlows, tick])

  // (Re)load the draft when the selected flow or its head version changes. A
  // pending unsaved edit survives an external head change (the agent or the
  // observer committed a version meanwhile): the next save lands on top of it.
  const dirty = !!draft && canonicalKey(draft) !== savedKey
  const layoutDirty = !!draft && layoutKey(draft) !== savedLayout
  useRegisterDirty('flows', dirty)
  const headKey = selected ? `${selected.id}@${selected.version}` : ''
  const resetDraft = (key: string) => {
    if (!selected || !key) {
      setDraft(null)
      return
    }
    if (dirty && draftFlowId?.split('@')[0] === selected.id) return
    let g = parseGraph(selected.graph)
    if (needsLayout(g)) g = autoLayout(g)
    setDraft(g)
    setSavedKey(canonicalKey(g))
    setSavedLayout(layoutKey(parseGraph(selected.graph)))
    setDraftFlowId(key)
    setSelNode(null)
    setSelEdge(null)
    setStatuses(new Map())
  }
  useKeyedReset(headKey, resetDraft)
  // First render has no draft yet: seed it once (useKeyedReset only fires on changes).
  if (draft === null && selected && draftFlowId !== headKey) {
    resetDraft(headKey)
  }

  // Loading flips on when the selected flow changes (render-time reset) and off
  // in the fetch's finally, so no effect sets state synchronously.
  useKeyedReset(selected?.id ?? '', () => setRunsLoading(true))
  const loadDetails = useCallback(() => {
    if (!selected) return
    api
      .listFlowRuns(selected.id, 100)
      .then(setRuns)
      .catch((e: Error) => onError(e.message))
      .finally(() => setRunsLoading(false))
    api
      .listFlowVersions(selected.id)
      .then(setVersions)
      .catch(() => undefined)
    api
      .listFlowProposals(selected.id)
      .then(setProposals)
      .catch(() => undefined)
    api
      .listPromptVersions(selected.agentId)
      .then(setPromptVersions)
      .catch(() => undefined)
  }, [selected, onError])

  useEffect(() => {
    loadDetails()
  }, [loadDetails, tick])
  // Runs keep arriving while turns run; a slow poll backs the live frames.
  useVisiblePoll(loadDetails, RUNS_POLL_MS, [loadDetails], tab === 'runs' || tab === 'test')

  // Live node status on the canvas for the selected flow.
  useEffect(() => {
    if (!selected) return
    return subscribeFlowNode(selected.id, (frame) => {
      setStatuses((prev) => {
        const next =
          frame.event.index === 1 && frame.event.phase === 'start'
            ? new Map<string, NodeStatus>()
            : new Map(prev)
        next.set(
          frame.event.nodeId,
          frame.event.phase === 'start'
            ? 'running'
            : frame.event.phase === 'done'
              ? 'done'
              : 'error',
        )
        return next
      })
      if (frame.event.type === 'output' && frame.event.phase !== 'start') {
        setTimeout(() => {
          loadFlows()
          loadDetails()
        }, 400)
      }
    })
  }, [selected, loadFlows, loadDetails])

  // ---- draft mutations ----
  const update = (fn: (g: FlowGraph) => FlowGraph) => setDraft((g) => (g ? fn(g) : g))
  const addNode = (type: FlowNodeType) => {
    update((g) => {
      const id = freshNodeId(g, type === 'llm' ? 'stage' : type)
      const n = defaultNode(type, id, t(`defaultTitle.${type}`))
      const maxY = Math.max(0, ...g.nodes.map((x) => x.y ?? 0))
      return { ...g, nodes: [...g.nodes, { ...n, x: 0, y: maxY + 140 }] }
    })
  }
  const changeNode = (id: string, patch: Partial<FlowNode>) =>
    update((g) => ({ ...g, nodes: g.nodes.map((n) => (n.id === id ? { ...n, ...patch } : n)) }))
  const changeEdge = (id: string, patch: Partial<FlowEdge>) =>
    update((g) => ({ ...g, edges: g.edges.map((e) => (e.id === id ? { ...e, ...patch } : e)) }))
  const deleteNodes = (ids: string[]) => {
    update((g) => {
      const keep = new Set(
        g.nodes
          .filter((n) => !ids.includes(n.id) || n.type === 'input' || n.type === 'output')
          .map((n) => n.id),
      )
      return {
        ...g,
        nodes: g.nodes.filter((n) => keep.has(n.id)),
        edges: g.edges.filter((e) => keep.has(e.from) && keep.has(e.to)),
      }
    })
    setSelNode(null)
  }
  const deleteEdges = (ids: string[]) => {
    update((g) => ({ ...g, edges: g.edges.filter((e) => !ids.includes(e.id)) }))
    setSelEdge(null)
  }
  const connect = (c: Connection) => {
    if (!c.source || !c.target || c.source === c.target) return
    update((g) => {
      if (g.edges.some((e) => e.from === c.source && e.to === c.target)) return g
      const from = g.nodes.find((n) => n.id === c.source)
      // Linear nodes keep one outgoing edge: a new connection replaces it.
      const edges =
        from && from.type !== 'route' ? g.edges.filter((e) => e.from !== c.source) : g.edges
      return {
        ...g,
        edges: [
          ...edges,
          { id: freshEdgeId(g, c.source!, c.target!), from: c.source!, to: c.target! },
        ],
      }
    })
  }
  const nodesChanged = (nodes: FlowRFNode[]) => update((g) => applyPositions(g, nodes))
  const relayout = () => update((g) => autoLayout(g))

  const save = (why: string) => {
    if (!selected || !draft) return
    setSaving(true)
    api
      .saveFlow(selected.id, draft, why)
      .then((res) => {
        setSavedKey(canonicalKey(draft))
        setSavedLayout(layoutKey(draft))
        setFlows((list) => list.map((f) => (f.id === res.flow.id ? res.flow : f)))
        setDraftFlowId(`${res.flow.id}@${res.flow.version}`)
        setSaveOpen(false)
        setReason('')
        loadDetails()
      })
      .catch((e: Error) => onError(e.message))
      .finally(() => setSaving(false))
  }
  const discard = () => {
    if (!selected) return
    let g = parseGraph(selected.graph)
    if (needsLayout(g)) g = autoLayout(g)
    setDraft(g)
    setSavedKey(canonicalKey(g))
    setSavedLayout(layoutKey(parseGraph(selected.graph)))
    setSelNode(null)
    setSelEdge(null)
  }

  const rf = useMemo(
    () =>
      draft ? toReactFlow(draft, statuses, agentNames, automationNames) : { nodes: [], edges: [] },
    [draft, statuses, agentNames, automationNames],
  )
  const problems = useMemo(() => (draft ? lint(draft) : []), [draft])
  const filtered = useMemo(
    () =>
      flows
        .filter(
          (f) =>
            !q.trim() ||
            `${f.name} ${f.agentName} ${f.id}`.toLowerCase().includes(q.trim().toLowerCase()),
        )
        .sort((a, b) => compareText(a.agentName, b.agentName)),
    [flows, q],
  )
  const inspectorOpen = !!selNode || !!selEdge
  // The docked inspector can be folded to a thin rail while nothing is selected;
  // picking a node or edge always shows it again.
  const [inspectorCollapsed, setInspectorCollapsed] = useSessionState(
    'flows.inspectorCollapsed',
    false,
  )

  const tabs = (
    <div
      className="flex overflow-hidden rounded-md border border-[var(--color-border)] text-xs"
      role="tablist"
    >
      {TABS.map((k) => (
        <button
          key={k}
          role="tab"
          aria-selected={tab === k}
          onClick={() => setTab(k)}
          data-testid={`flows-tab-${k}`}
          className={`px-2.5 py-1 ${tab === k ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]' : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'}`}
        >
          {t(`tabs.${k}`)}
          {k === 'evolution' && selected && selected.pendingProposals > 0 && (
            <span className="ml-1 rounded-full bg-[var(--color-warning)] px-1 text-[9px] text-[var(--color-on-warning)]">
              {selected.pendingProposals}
            </span>
          )}
        </button>
      ))}
    </div>
  )

  const title = selected ? (
    <div className="flex min-w-0 items-center gap-2">
      <AgentAvatar
        agent={{
          id: selected.agentId,
          name: selected.agentName,
          avatar: selected.agentAvatar,
          color: selected.agentColor,
        }}
        size={24}
      />
      <span className="truncate text-sm font-semibold">{selected.name}</span>
      <Badge tone="muted">{t('version', { v: selected.version })}</Badge>
      {dirty && <Badge tone="warning">{t('header.unsaved')}</Badge>}
      {selected.trivial && !dirty && <Badge tone="muted">{t('header.trivial')}</Badge>}
    </div>
  ) : (
    <span className="text-sm font-semibold">{t('title')}</span>
  )

  const right = selected && tab === 'canvas' && (
    <>
      {dirty && (
        <Button size="sm" variant="secondary" onClick={discard} disabled={saving}>
          <X size={12} /> {t('header.discard')}
        </Button>
      )}
      <Button
        size="sm"
        onClick={() => (dirty ? setSaveOpen(true) : layoutDirty ? save('layout') : undefined)}
        disabled={saving || (!dirty && !layoutDirty) || problems.length > 0}
        data-testid="flow-save"
        title={problems.length ? t('header.fixProblems') : undefined}
      >
        <Save size={12} /> {dirty ? t('header.save') : t('header.saveLayout')}
      </Button>
    </>
  )

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="flows-panel">
      <PaneHeader
        listOpen={listOpen}
        onToggleList={toggleList}
        titleSlot={title}
        secondary={tabs}
        right={right || undefined}
      />
      <div className="flex min-h-0 flex-1">
        <ListPane
          open={listOpen}
          onToggle={toggleList}
          widthKey="tionharness.flowsListWidth"
          label={t('list.title')}
          testId="flows-list"
        >
          <SidebarHeader title={t('list.title')} onCollapse={toggleList}>
            <RefreshButton onClick={loadFlows} />
          </SidebarHeader>
          <div className="px-3 pb-2">
            <input
              value={q}
              onChange={(e) => setQ(e.target.value)}
              placeholder={t('list.search')}
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs focus:border-[var(--color-accent)] focus:outline-none"
            />
          </div>
          <ul className="min-h-0 flex-1 overflow-y-auto px-2 pb-2">
            {!loadingFlows && filtered.length === 0 && (
              <EmptyState icon={GitBranch} title={t('list.empty')} />
            )}
            {filtered.map((f) => (
              <li key={f.id}>
                <button
                  type="button"
                  onClick={() => {
                    setSelectedId(f.id)
                    if (stacked) toggleList()
                  }}
                  data-testid={`flow-row-${f.id}`}
                  className={`flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-xs hover:bg-[var(--color-surface-2)] ${selectedId === f.id ? SELECTED_ITEM_CLS : ''}`}
                >
                  <AgentAvatar
                    agent={{
                      id: f.agentId,
                      name: f.agentName,
                      avatar: f.agentAvatar,
                      color: f.agentColor,
                    }}
                    size={26}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium">{f.name}</span>
                    <span className="block truncate text-[10px] text-[var(--color-text-dim)]">
                      {t('list.meta', { v: f.version, nodes: f.nodeCount, runs: f.stats.runs })}
                    </span>
                  </span>
                  {f.pendingProposals > 0 && (
                    <span
                      className="shrink-0 rounded-full bg-[var(--color-warning)] px-1.5 text-[9px] text-[var(--color-on-warning)]"
                      title={t('list.pending', { count: f.pendingProposals })}
                    >
                      {f.pendingProposals}
                    </span>
                  )}
                  {!f.trivial && (
                    <Sparkles size={12} className="shrink-0 text-[var(--color-accent)]" />
                  )}
                </button>
              </li>
            ))}
          </ul>
        </ListPane>

        <main className="relative flex min-w-0 flex-1 flex-col">
          {!selected ? (
            <EmptyState
              icon={GitBranch}
              title={loadingFlows ? t('list.loading') : t('list.noneSelected')}
              className="flex-1 justify-center"
            />
          ) : tab === 'canvas' && draft ? (
            <div className="flex min-h-0 flex-1">
              <div className="relative min-w-0 flex-1">
                <FlowCanvas
                  nodes={rf.nodes}
                  edges={rf.edges}
                  showMinimap={!stacked}
                  onNodesChange={nodesChanged}
                  onConnect={connect}
                  onEdgesDelete={deleteEdges}
                  onNodesDelete={deleteNodes}
                  onSelect={({ nodeId, edgeId }) => {
                    setSelNode(nodeId)
                    setSelEdge(edgeId)
                  }}
                  onAdd={addNode}
                  onAutoLayout={relayout}
                />
                {problems.length > 0 && (
                  <div className="pointer-events-none absolute inset-x-2 bottom-2 z-10 rounded-md border border-[var(--color-warning)]/50 bg-[var(--color-surface)] px-3 py-1.5 text-[11px] text-[var(--color-warning)] shadow-[var(--shadow-md)]">
                    {problems.slice(0, 3).map((p, i) => (
                      <div key={i}>{t(p.key, p.params)}</div>
                    ))}
                  </div>
                )}
                {/* Bottom-sheet inspector on stacked layouts. */}
                {stacked && inspectorOpen && (
                  <div
                    className="absolute inset-x-0 bottom-0 z-20 max-h-[58%] overflow-y-auto rounded-t-xl border-t border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
                    data-testid="flow-inspector-sheet"
                  >
                    <div className="sticky top-0 flex justify-center bg-[var(--color-surface)] pt-1">
                      <button
                        type="button"
                        onClick={() => {
                          setSelNode(null)
                          setSelEdge(null)
                        }}
                        className="rounded p-1 text-[var(--color-text-dim)]"
                        aria-label={t('inspector.close')}
                      >
                        <ChevronDown size={16} />
                      </button>
                    </div>
                    <NodeInspector
                      graph={draft}
                      nodeId={selNode}
                      edgeId={selEdge}
                      agents={agents}
                      automations={automations}
                      ownerAgentId={selected.agentId}
                      readOnly={false}
                      onChangeNode={changeNode}
                      onChangeEdge={changeEdge}
                      onDeleteNode={(id) => deleteNodes([id])}
                      onDeleteEdge={(id) => deleteEdges([id])}
                      onClose={() => {
                        setSelNode(null)
                        setSelEdge(null)
                      }}
                    />
                  </div>
                )}
              </div>
              {!stacked && inspectorCollapsed && !inspectorOpen && (
                <aside
                  className="flex w-9 shrink-0 justify-center border-l border-[var(--color-border)] bg-[var(--color-surface)] pt-3"
                  data-testid="flow-inspector-rail"
                >
                  <button
                    type="button"
                    onClick={() => setInspectorCollapsed(false)}
                    title={t('inspector.expand')}
                    aria-label={t('inspector.expand')}
                    className="h-fit rounded p-1 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                  >
                    <PanelRightOpen size={14} />
                  </button>
                </aside>
              )}
              {!stacked && (!inspectorCollapsed || inspectorOpen) && (
                <aside
                  className="w-[22rem] shrink-0 overflow-y-auto border-l border-[var(--color-border)] bg-[var(--color-surface)]"
                  data-testid="flow-inspector"
                >
                  <NodeInspector
                    graph={draft}
                    nodeId={selNode}
                    edgeId={selEdge}
                    agents={agents}
                    automations={automations}
                    ownerAgentId={selected.agentId}
                    readOnly={false}
                    onChangeNode={changeNode}
                    onChangeEdge={changeEdge}
                    onDeleteNode={(id) => deleteNodes([id])}
                    onDeleteEdge={(id) => deleteEdges([id])}
                    onClose={() => {
                      setSelNode(null)
                      setSelEdge(null)
                    }}
                    onCollapse={() => setInspectorCollapsed(true)}
                  />
                  {selected.note && (
                    <p className="border-t border-[var(--color-border)] px-4 py-3 text-[11px] text-[var(--color-text-dim)]">
                      {selected.note}
                    </p>
                  )}
                </aside>
              )}
            </div>
          ) : tab === 'runs' ? (
            <RunsTab
              flow={selected}
              runs={runs}
              loading={runsLoading}
              stacked={stacked}
              onReload={loadDetails}
              onOpenSession={onOpenSession}
              onError={onError}
            />
          ) : tab === 'evolution' ? (
            <EvolutionTab
              flow={selected}
              versions={versions}
              proposals={proposals}
              promptVersions={promptVersions}
              busy={saving}
              onChanged={() => {
                loadFlows()
                loadDetails()
              }}
              onError={onError}
              onOpenSession={onOpenSession}
            />
          ) : tab === 'test' ? (
            <TestTab flow={selected} onOpenSession={onOpenSession} onError={onError} />
          ) : null}
        </main>
      </div>

      {saveOpen && selected && (
        <ModalOverlay onClose={() => setSaveOpen(false)}>
          <div className="w-full max-w-md rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-2xl">
            <h2 className="mb-3 flex items-center gap-1 text-sm font-semibold">
              {t('save.title', { v: selected.version + 1 })}
              <InfoPopover text={t('save.hint')} />
            </h2>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              autoFocus
              placeholder={t('save.placeholder')}
              data-testid="flow-save-reason"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm focus:border-[var(--color-accent)] focus:outline-none"
            />
            <div className="mt-3 flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={() => setSaveOpen(false)}>
                {t('save.cancel')}
              </Button>
              <Button
                size="sm"
                onClick={() => save(reason.trim() || t('save.defaultReason'))}
                disabled={saving}
                data-testid="flow-save-confirm"
              >
                {saving ? t('save.saving') : t('save.confirm')}
              </Button>
            </div>
          </div>
        </ModalOverlay>
      )}
    </div>
  )
}
