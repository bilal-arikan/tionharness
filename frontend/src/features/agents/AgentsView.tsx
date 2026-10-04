import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { useTranslation } from 'react-i18next'
import { RefreshCw, Trash2, Activity, Pencil, Archive, ArchiveRestore, Library } from 'lucide-react'
import type { Agent, AgentPatch } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { ProviderInstanceModelSelect } from '@/shared/components/agents/ProviderInstanceModelSelect'
import { useCatalog, resolveModelLabel } from '@/shared/lib/catalog'
import { eligibleParents, indexAgents, lineageOf } from '@/shared/lib/agentLineage'
import { AgentSettingsForm } from './AgentSettingsForm'
import { AgentActivityPanel } from './AgentActivityPanel'
import { SystemAgentStatusBadge } from './SystemAgentStatusBadge'
import { AgentBulkEditPanel } from './AgentBulkEditPanel'
import { AgentLineageStripes } from './AgentLineageStripes'
import { groupSystemAgents, isBuiltinSystemAgent } from './agentRoster'
import { api, getActiveWorkspace } from '@/api'
import { useReferencedAgents } from '@/shared/hooks/useReferencedAgents'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { CoordinatorWorkflowPicker } from '@/shared/components/CoordinatorWorkflowPicker'
import {
  Button,
  PromptEditor,
  SelectionBar,
  SelectionBarButton,
  ListPane,
  PaneHeader,
  ArchiveViewToggle,
  ArchiveViewBanner,
} from '@/shared/components'
import { archiveSide } from '@/shared/lib/archive'
import {
  SidebarHeader,
  NewItemButton,
  SELECTED_ITEM_CLS,
  SELECTED_ITEM_RING,
} from '@/shared/components/SidebarChrome'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'

interface Props {
  agents: Agent[]
  defaultAgentId: string | null
  defaultAgentSaveState: 'idle' | 'saving' | 'saved'
  /** Controlled selection (deep-link aware); falls back to internal state. */
  selectedId?: string | null
  onSelectAgent?: (id: string) => void
  /** Set an agent as the default for new chats. */
  onSetDefault: (id: string) => void
  onCreateAgent: (
    name: string,
    soul: string,
    provider: string,
    model: string,
    /** Coordinator defaults for the sessions the new agent opens. Optional so
     * callers that never expose the toggle keep the plain four-arg shape. */
    coordinator?: { mode: boolean; workflow: string; prompt: string },
    /** Create as a CHILD of this agent (inherits provider/model/… from it). */
    parentId?: string,
  ) => void
  onUpdateAgent: (id: string, patch: AgentPatch) => Promise<{ agent: Agent; warning?: string }>
  /** Clone the agent (full profile + tool config) into a new "(kopya)". */
  onDuplicateAgent: (id: string) => Promise<string | undefined>
  /** Derive a child that inherits every field; bindRole takes over the parent's
   * system role (how a locked built-in is customised). */
  onDeriveAgent?: (
    id: string,
    opts: { name?: string; bindRole?: boolean },
  ) => Promise<string | undefined>
  onDeleteAgent: (id: string) => Promise<void>
  /** Re-fetch the agent roster from the server. */
  onRefresh?: () => void | Promise<void>
  /** Surface errors (e.g. activity feed load failures) to the app banner. */
  onError?: (msg: string) => void
  /** Open a run on the Activity screen with it pre-selected. */
  onOpenExecution?: (sessionId: string) => void
  onOpenAgentLibrary?: () => void
}

const rosterSectionHeading = (label: string, hint: string) => (
  <h3
    className="mb-1 mt-3 px-2.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]"
    title={hint}
  >
    {label}
  </h3>
)

// AgentsView is the two-pane "Ajanlar" screen: a roster on the left, and the
// selected agent's editable settings on the right (replacing the modal).
export function AgentsView({
  agents,
  defaultAgentId,
  defaultAgentSaveState,
  selectedId: controlledId,
  onSelectAgent,
  onSetDefault,
  onCreateAgent,
  onUpdateAgent,
  onDuplicateAgent,
  onDeriveAgent,
  onDeleteAgent,
  onRefresh,
  onError,
  onOpenExecution,
  onOpenAgentLibrary,
}: Props) {
  const { t } = useTranslation('agents')
  const { t: tc } = useTranslation('common')
  const referencedAgents = useReferencedAgents(agents, [controlledId], getActiveWorkspace())
  const [internalId, setInternalId] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [systemActionPending, setSystemActionPending] = useState(false)
  const [bulkEditOpen, setBulkEditOpen] = useState(false)
  // Archive view: the roster shows only archived agents (with a restore action)
  // instead of the live ones — the kanban board's archive pattern. System agents
  // cannot be archived, so they only ever appear in the live view.
  const [showArchived, setShowArchived] = useState(false)
  const [archivedAgents, setArchivedAgents] = useState<Agent[]>([])
  const archiveRequest = useRef(0)
  const reloadArchived = useCallback(() => {
    if (!showArchived) return Promise.resolve()
    const request = ++archiveRequest.current
    return api
      .listAgents(true)
      .then((rows) => {
        if (request === archiveRequest.current)
          setArchivedAgents(rows.filter((agent) => !agent.deleted))
      })
      .catch((error) => {
        if (request === archiveRequest.current) onError?.((error as Error).message)
      })
  }, [showArchived, onError])
  useKeyedReset(showArchived, () => setArchivedAgents([]))
  useEffect(() => {
    void reloadArchived()
    return () => {
      archiveRequest.current++
    }
  }, [reloadArchived, agents])
  const catalog = useCatalog()

  // Left roster collapse (standard list pane) — toggled from the PaneHeader.
  const { open: rosterOpen, toggle: toggleRoster } = useCollapsibleList(
    'tionharness.agentsListOpen',
  )

  // Right-hand activity panel visibility (persisted) — mirrors the chat
  // SessionDetailPanel open/close affordance so the middle settings area can use
  // the full width when the feed isn't needed.
  // Default CLOSED on the Agents screen (the settings area gets the full width);
  // only reopen if the user explicitly left it open before ('1').
  const [activityOpen, setActivityOpen] = useState(
    () => localStorage.getItem('tionharness.agentActivityOpen') === '1',
  )
  const toggleActivity = () =>
    setActivityOpen((v) => {
      const next = !v
      localStorage.setItem('tionharness.agentActivityOpen', next ? '1' : '0')
      return next
    })

  const doRefresh = async () => {
    if (!onRefresh || refreshing) return
    setRefreshing(true)
    try {
      await onRefresh()
    } finally {
      setRefreshing(false)
    }
  }
  const [name, setName] = useState('')
  const [soul, setSoul] = useState('')
  // provider holds the provider INSTANCE id (default "claude-cli" — the
  // built-in default instance's id equals its kind id, _Docs/71 §3).
  const [provider, setProvider] = useState('claude-cli')
  // null = the user has not picked a model yet (submit stays blocked). '' is a
  // valid pick: the catalog's "… oturum modeli" entry, which leaves the model
  // decision to the CLI session.
  const [model, setModel] = useState<string | null>(null)
  // Coordinator defaults on the create form, mirroring AgentSettingsForm: the
  // recipe and the prompt only appear once the mode is on, and are sent empty
  // when it is off so a non-coordinator carries no orchestration leftovers.
  const [coordinatorMode, setCoordinatorMode] = useState(false)
  const [coordinatorWorkflow, setCoordinatorWorkflow] = useState('')
  const [coordinatorPrompt, setCoordinatorPrompt] = useState('')
  // "" = start from scratch; otherwise the new agent inherits from this one and
  // the provider/model/coordinator inputs are hidden (they come from the parent).
  const [createParentId, setCreateParentId] = useState('')

  // Selection is controlled by the parent (deep-link aware) when provided,
  // otherwise tracked internally.
  const selectedId = controlledId !== undefined ? controlledId : internalId
  const select = (id: string) => {
    if (onSelectAgent) onSelectAgent(id)
    else setInternalId(id)
  }

  // The roster side currently shown (live or archived).
  const visibleAgents = useMemo(
    () => archiveSide(showArchived ? archivedAgents : agents, showArchived),
    [agents, archivedAgents, showArchived],
  )

  // The rows the roster lists: locked built-in system agents stay out of the
  // workspace roster (only the customisations inheriting from them are shown).
  // They remain selectable through deep links and as inheritance parents.
  const rosterAgents = useMemo(
    () => visibleAgents.filter((a) => !isBuiltinSystemAgent(a)),
    [visibleAgents],
  )

  // Keep a valid selection within the shown side: prefer the current one, else
  // the default, else the first listed row.
  useEffect(() => {
    // A deep link may target an archived author outside either loaded roster.
    if (
      controlledId &&
      !agents.some((a) => a.id === controlledId) &&
      !archivedAgents.some((a) => a.id === controlledId)
    )
      return
    if (selectedId && visibleAgents.some((a) => a.id === selectedId)) return
    const defaultVisible = !!defaultAgentId && rosterAgents.some((a) => a.id === defaultAgentId)
    const fallback = (defaultVisible ? defaultAgentId : null) ?? rosterAgents[0]?.id ?? null
    if (fallback) select(fallback)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    visibleAgents,
    rosterAgents,
    defaultAgentId,
    selectedId,
    controlledId,
    agents,
    archivedAgents,
  ])

  const selected = [...archivedAgents, ...referencedAgents].find((a) => a.id === selectedId) ?? null

  // Multi-select for bulk roster actions (Ctrl/Cmd+Click, Shift-range).
  const sel = useMultiSelect()
  const byId = useMemo(() => indexAgents([...agents, ...archivedAgents]), [agents, archivedAgents])
  const regularAgents = useMemo(() => visibleAgents.filter((a) => !a.system), [visibleAgents])
  // System section: only the workspace customisations that inherit from a
  // built-in, split into services vs worker profiles and kept in registry order
  // so customisations of the same role stay adjacent.
  const systemGroups = useMemo(() => groupSystemAgents(visibleAgents), [visibleAgents])
  const orderedIds = useMemo(() => regularAgents.map((a) => a.id), [regularAgents])
  const bulkDelete = async () => {
    const ids = [...sel.selected].filter((id) => regularAgents.some((a) => a.id === id))
    if (ids.length === 0) return
    if (!confirm(t('view.deleteManyConfirm', { count: ids.length }))) return
    for (const id of ids) await onDeleteAgent(id)
    setBulkEditOpen(false)
    sel.clear()
  }

  const bulkAgents = regularAgents.filter((agent) => sel.selected.has(agent.id))

  // Archive or restore agents. Either way they leave the CURRENT roster side,
  // so the roster is re-fetched afterwards. Failures (e.g. a system agent, 409)
  // surface through onError instead of being swallowed.
  const setArchived = async (ids: string[], archived: boolean) => {
    try {
      for (const id of ids) await api.setArchived('agents', id, archived)
    } catch (e) {
      onError?.((e as Error).message)
    } finally {
      await onRefresh?.()
    }
  }
  const bulkArchive = async (archived: boolean) => {
    const ids = [...sel.selected].filter((id) => regularAgents.some((a) => a.id === id))
    if (ids.length === 0) return
    await setArchived(ids, archived)
    setBulkEditOpen(false)
    sel.clear()
  }

  const canSubmit = name.trim() !== '' && (createParentId !== '' || model !== null)

  const submit = () => {
    if (!canSubmit) return
    if (createParentId) {
      onCreateAgent(name.trim(), soul.trim(), '', '', undefined, createParentId)
    } else {
      if (model === null) return
      onCreateAgent(name.trim(), soul.trim(), provider, model, {
        mode: coordinatorMode,
        workflow: coordinatorMode ? coordinatorWorkflow : '',
        prompt: coordinatorMode ? coordinatorPrompt : '',
      })
    }
    setName('')
    setSoul('')
    setModel(null)
    setCoordinatorMode(false)
    setCoordinatorWorkflow('')
    setCoordinatorPrompt('')
    setCreateParentId('')
    setShowForm(false)
  }

  const restoreDefault = async (agent: Agent) => {
    if (!confirm(t('view.restoreDefaultsConfirm', { name: agent.name }))) return
    setSystemActionPending(true)
    try {
      await api.restoreDefaultAgent(agent.id)
      await onRefresh?.()
    } catch (e) {
      onError?.((e as Error).message)
    } finally {
      setSystemActionPending(false)
    }
  }

  const rosterItem = (a: Agent) => {
    const lineage = lineageOf(a, byId)
    return (
      <div
        key={a.id}
        data-testid="agent-roster-item"
        data-agent-id={a.id}
        data-lineage-depth={lineage.length}
        className={`group mb-0.5 flex w-full items-stretch rounded-lg pr-1 text-sm transition ${
          a.disabled ? 'opacity-50' : ''
        } ${
          sel.isSelected(a.id)
            ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
            : selectedId === a.id
              ? SELECTED_ITEM_CLS
              : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
        }`}
      >
        <button
          onClick={(e) => {
            if (!a.system && sel.handleClick(e, a.id, orderedIds, selectedId)) return
            select(a.id)
          }}
          data-testid="agent-roster-select"
          data-agent-id={a.id}
          className="flex min-w-0 flex-1 items-stretch gap-2 py-1 pl-2 pr-1 text-left"
        >
          {/* Inheritance marker: one colour bar per ancestor, root outermost. */}
          <AgentLineageStripes lineage={lineage} className="my-0.5" />
          <span className="flex min-w-0 flex-1 items-center py-0.5">
            <AgentIdentity
              agent={a}
              size="sm"
              active={defaultAgentId === a.id}
              nameSuffix={
                <>
                  {a.disabled && !a.system && (
                    <span className="ml-1.5 shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px]">
                      {t('view.disabled')}
                    </span>
                  )}
                  <SystemAgentStatusBadge agent={a} />
                  {a.coordinatorMode && (
                    <span
                      data-testid="agent-coordinator-badge"
                      className="ml-1.5 shrink-0 text-[11px]"
                      title={t('view.coordinatorTitle')}
                    >
                      🕸
                    </span>
                  )}
                  <span
                    className="ml-1.5 shrink-0 font-mono text-[10px] opacity-60"
                    title={t('view.idTitle')}
                  >
                    {a.id}
                  </span>
                </>
              }
              subtitle={
                resolveModelLabel(catalog, a.provider, a.model) +
                (defaultAgentId === a.id ? ` · ${t('view.defaultSuffix')}` : '') +
                (lineage.length > 0 ? ` · ← ${lineage[lineage.length - 1].name}` : '')
              }
            />
          </span>
        </button>
        {defaultAgentId === a.id && (
          <span
            data-testid="agent-default-indicator"
            data-agent-id={a.id}
            title={t('view.defaultTitle')}
            className="ml-1 shrink-0 self-center p-1 text-[var(--color-accent)]"
          >
            {t('view.defaultIndicator')}
          </span>
        )}
      </div>
    )
  }

  // Inheritance context for the selected agent's form.
  const selectedLineage = selected ? lineageOf(selected, byId) : []
  const selectedParent = selected?.parentId ? (byId.get(selected.parentId) ?? null) : null
  const selectedParentOptions = selected ? eligibleParents(selected, agents) : []

  return (
    <div className="flex h-full min-h-0 flex-1">
      {/* Left: roster — full-height sibling column (like chat). */}
      <ListPane
        open={rosterOpen}
        onToggle={toggleRoster}
        widthKey="tionharness.agentsListWidth"
        defaultWidth={256}
        label={t('view.title')}
        testId="agents-list-toggle"
      >
        <SidebarHeader title={t('view.title')} onCollapse={toggleRoster}>
          <ArchiveViewToggle
            testId="agents-archived-toggle"
            active={showArchived}
            onToggle={() => {
              sel.clear()
              if (showArchived) {
                const next = agents.find((agent) => agent.id === defaultAgentId) ?? agents[0]
                if (next) select(next.id)
              }
              setShowArchived((v) => !v)
            }}
            backLabel={t('view.active')}
            backTitle={t('view.activeTitle')}
          />
          {onRefresh && (
            <button
              onClick={doRefresh}
              disabled={refreshing}
              data-testid="agents-refresh"
              title={t('view.refresh')}
              className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-50"
            >
              <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
            </button>
          )}
        </SidebarHeader>

        {/* Prominent new-agent button (shared chrome). */}
        {!showArchived && (
          <NewItemButton
            onClick={() => setShowForm((v) => !v)}
            label={t('view.new')}
            title={t('view.newTitle')}
            testId="agent-create-toggle"
          />
        )}
        {showArchived && (
          <ArchiveViewBanner
            testId="agents-archive-banner"
            count={regularAgents.length}
            noun={t('view.archiveNoun')}
            restoreHint={t('view.archiveHint')}
          />
        )}

        {!showArchived && showForm && (
          <div className="mx-3 mb-2 space-y-2 rounded-lg bg-[var(--color-surface-2)] p-3">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('view.namePlaceholder')}
              data-testid="agent-create-name-input"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <label className="block space-y-1 text-xs text-[var(--color-text-dim)]">
              <span>{t('view.parent')}</span>
              <select
                data-testid="agent-create-parent-select"
                value={createParentId}
                onChange={(e) => setCreateParentId(e.target.value)}
                className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]"
              >
                <option value="">{t('view.fromScratch')}</option>
                {agents
                  .filter((a) => !a.deleted && !a.archived)
                  .map((a) => (
                    <option key={a.id} value={a.id}>
                      {a.locked
                        ? t('view.parentOptionBuiltin', { name: a.name, id: a.id })
                        : t('view.parentOption', { name: a.name, id: a.id })}
                    </option>
                  ))}
              </select>
            </label>
            <PromptEditor
              value={soul}
              onChange={setSoul}
              placeholder={createParentId ? t('view.soulInherited') : t('view.soul')}
              rows={3}
              data-testid="agent-create-soul-textarea"
            />
            {createParentId ? (
              <p className="text-xs text-[var(--color-text-dim)]">{t('view.inheritCreateHelp')}</p>
            ) : (
              <>
                <div data-testid="agent-create-provider-wrap" className="contents">
                  <ProviderInstanceModelSelect
                    providerInstanceId={provider}
                    model={model}
                    onChange={(_kindId, instanceId, m) => {
                      setProvider(instanceId)
                      setModel(m)
                    }}
                  />
                </div>
                <label className="flex cursor-pointer items-start gap-2 text-xs text-[var(--color-text)]">
                  <input
                    type="checkbox"
                    data-testid="agent-create-coordinator-mode"
                    checked={coordinatorMode}
                    onChange={(e) => setCoordinatorMode(e.target.checked)}
                    className="mt-0.5 accent-[var(--color-accent)]"
                  />
                  <span>{t('view.coordinatorCreate')}</span>
                </label>
                {coordinatorMode && (
                  <>
                    <CoordinatorWorkflowPicker
                      value={coordinatorWorkflow}
                      onChange={setCoordinatorWorkflow}
                      groupName="agent-create-recipe"
                    />
                    <PromptEditor
                      data-testid="agent-create-coordinator-prompt-textarea"
                      value={coordinatorPrompt}
                      onChange={setCoordinatorPrompt}
                      placeholder={t('view.coordinatorPrompt')}
                      rows={3}
                    />
                    <p className="text-xs text-[var(--color-text-dim)]">
                      {t('view.coordinatorPromptHelp')}
                    </p>
                  </>
                )}
              </>
            )}
            <Button
              onClick={submit}
              disabled={!canSubmit}
              title={!createParentId && model === null ? t('view.selectModel') : undefined}
              data-testid="agent-create-submit"
              className="w-full"
            >
              {t('view.create')}
            </Button>
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {regularAgents.map(rosterItem)}
          {systemGroups.services.length > 0 && (
            <>
              {rosterSectionHeading(t('view.systemAgents'), t('view.systemAgentsHint'))}
              {systemGroups.services.map(rosterItem)}
            </>
          )}
          {systemGroups.workers.length > 0 && (
            <>
              {rosterSectionHeading(t('view.systemWorkers'), t('view.systemWorkersHint'))}
              {systemGroups.workers.map(rosterItem)}
            </>
          )}
          {!showArchived && rosterAgents.length === 0 && (
            <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">{t('view.empty')}</p>
          )}
        </div>

        {bulkEditOpen && bulkAgents.length > 0 && (
          <AgentBulkEditPanel
            agents={bulkAgents}
            onUpdateAgent={onUpdateAgent}
            onApplied={() => {
              setBulkEditOpen(false)
              sel.clear()
            }}
            onCancel={() => setBulkEditOpen(false)}
            onError={onError}
          />
        )}
        <SelectionBar
          count={sel.count}
          onClear={() => {
            setBulkEditOpen(false)
            sel.clear()
          }}
          onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
        >
          <SelectionBarButton icon={<Pencil size={13} />} onClick={() => setBulkEditOpen(true)}>
            {tc('agents.bulkEdit.button')}
          </SelectionBarButton>
          {showArchived ? (
            <SelectionBarButton
              icon={<ArchiveRestore size={13} />}
              onClick={() => bulkArchive(false)}
            >
              {t('view.restore')}
            </SelectionBarButton>
          ) : (
            <SelectionBarButton icon={<Archive size={13} />} onClick={() => bulkArchive(true)}>
              {t('view.archive')}
            </SelectionBarButton>
          )}
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            {t('view.delete')}
          </SelectionBarButton>
        </SelectionBar>
      </ListPane>

      {/* Content column: the title bar sits ONLY here (right of the roster), like chat. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          title={t('view.title')}
          listOpen={rosterOpen}
          onToggleList={toggleRoster}
          subtitle={selected ? `· ${selected.name}` : t('view.noneSelected')}
          right={
            <>
              {onOpenAgentLibrary && (
                <button
                  type="button"
                  onClick={onOpenAgentLibrary}
                  title={t('view.libraryOpen')}
                  aria-label={t('view.libraryOpen')}
                  className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                >
                  <Library size={15} className="shrink-0" />
                  <span className="hidden sm:inline">{t('view.library')}</span>
                </button>
              )}
              {selected && (
                <>
                  <CopyPathButton
                    testId="agent-copy-path"
                    getPath={async () => (await api.agentPath(selected.id)).path}
                    title={t('view.copyPath')}
                    onError={onError}
                  />
                  {!selected.system && (
                    <button
                      type="button"
                      data-testid="agent-archive-action"
                      onClick={() => setArchived([selected.id], !selected.archived)}
                      title={selected.archived ? t('view.restoreTitle') : t('view.archiveTitle')}
                      className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                    >
                      {selected.archived ? <ArchiveRestore size={15} /> : <Archive size={15} />}
                      <span className="hidden sm:inline">
                        {selected.archived ? t('view.restore') : t('view.archive')}
                      </span>
                    </button>
                  )}
                </>
              )}
              {/* Toggle the right-hand activity feed from the top bar (like chat's Detay). */}
              <button
                onClick={toggleActivity}
                data-testid="agent-activity-toggle"
                aria-pressed={activityOpen}
                title={activityOpen ? t('view.activityHide') : t('view.activityShow')}
                className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs transition ${
                  activityOpen
                    ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
                }`}
              >
                <Activity size={15} className="shrink-0" />
                <span className="hidden sm:inline">{t('view.activity')}</span>
              </button>
            </>
          }
        />
        <div className="flex min-h-0 flex-1">
          {/* Middle: selected agent's settings */}
          <div className="flex min-w-0 flex-1 flex-col">
            {selected?.archived && (
              <div
                data-testid="agent-archived-notice"
                className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-xs text-[var(--color-text-dim)]"
              >
                <Archive size={13} className="flex-shrink-0" />
                {t('view.archivedNotice')}
              </div>
            )}
            <div className="min-h-0 flex-1">
              {selected ? (
                <AgentSettingsForm
                  key={selected.id}
                  agent={selected}
                  dirtyView="agents"
                  isDefault={defaultAgentId === selected.id}
                  defaultSaveState={defaultAgentSaveState}
                  onSetDefault={() => onSetDefault(selected.id)}
                  onSave={(p) => onUpdateAgent(selected.id, p)}
                  onDuplicate={selected.system ? undefined : () => onDuplicateAgent(selected.id)}
                  onDerive={
                    onDeriveAgent && !selected.system
                      ? (opts) => onDeriveAgent(selected.id, opts)
                      : undefined
                  }
                  readOnly={selected.system}
                  readOnlyNote={<>{t('view.readOnlyNote')}</>}
                  lineage={selectedLineage}
                  parent={selectedParent}
                  parentOptions={selectedParentOptions}
                  onSelectAgent={select}
                  onDelete={
                    selected.system
                      ? undefined
                      : async () => {
                          if (confirm(t('view.deleteOneConfirm', { name: selected.name }))) {
                            await onDeleteAgent(selected.id)
                            if (!onSelectAgent) setInternalId(null)
                          }
                        }
                  }
                  onRestoreDefault={
                    selected.parentId && !selected.system
                      ? () => restoreDefault(selected)
                      : undefined
                  }
                  systemActionPending={systemActionPending}
                />
              ) : (
                <div className="flex h-full items-center justify-center text-sm text-[var(--color-text-dim)]">
                  {t('view.selectToEdit')}
                </div>
              )}
            </div>
          </div>

          {/* Right: selected agent's live activity feed — opened from the top bar's
          "Aktivite" button. Desktop: a right-hand column. Mobile: a right slide-in
          drawer with a dim backdrop — same as the chat session-detail panel. */}
          {activityOpen && (
            <>
              <div
                className="fixed inset-0 z-30 bg-[var(--color-overlay)]/50 md:hidden"
                onClick={toggleActivity}
              />
              <div className="flex shrink-0 md:static max-md:fixed max-md:inset-y-0 max-md:right-0 max-md:z-40 max-md:w-[85vw] max-md:max-w-sm max-md:shadow-xl">
                <AgentActivityPanel
                  agentId={selected?.id ?? null}
                  onError={onError ?? (() => {})}
                  onOpenExecution={onOpenExecution}
                  onClose={toggleActivity}
                />
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
