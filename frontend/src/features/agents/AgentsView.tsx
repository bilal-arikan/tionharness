import { useEffect, useMemo, useState } from 'react'
import { RefreshCw, Trash2, Activity } from 'lucide-react'
import type { Agent, AgentPatch } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { ProviderInstanceModelSelect } from '@/shared/components/agents/ProviderInstanceModelSelect'
import { useCatalog, resolveModelLabel } from '@/shared/lib/catalog'
import { AgentSettingsForm } from './AgentSettingsForm'
import { AgentActivityPanel } from './AgentActivityPanel'
import { api } from '@/api'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { CoordinatorWorkflowPicker } from '@/shared/components/CoordinatorWorkflowPicker'
import {
  Button,
  PromptEditor,
  SelectionBar,
  SelectionBarButton,
  ListPane,
  PaneHeader,
} from '@/shared/components'
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
  ) => void
  onUpdateAgent: (id: string, patch: AgentPatch) => Promise<{ agent: Agent; warning?: string }>
  /** Clone the agent (full profile + tool config) into a new "(kopya)". */
  onDuplicateAgent: (id: string) => Promise<string | undefined>
  onDeleteAgent: (id: string) => Promise<void>
  /** Re-fetch the agent roster from the server. */
  onRefresh?: () => void | Promise<void>
  /** Surface errors (e.g. activity feed load failures) to the app banner. */
  onError?: (msg: string) => void
  /** Open a run on the Activity screen with it pre-selected. */
  onOpenExecution?: (sessionId: string) => void
}

// AgentsView is the two-pane "Ajanlar" screen: a roster on the left, and the
// selected agent's editable settings on the right (replacing the modal).
export function AgentsView({
  agents,
  defaultAgentId,
  selectedId: controlledId,
  onSelectAgent,
  onSetDefault,
  onCreateAgent,
  onUpdateAgent,
  onDuplicateAgent,
  onDeleteAgent,
  onRefresh,
  onError,
  onOpenExecution,
}: Props) {
  const [internalId, setInternalId] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
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
  const [model, setModel] = useState('')
  // Coordinator defaults on the create form, mirroring AgentSettingsForm: the
  // recipe and the prompt only appear once the mode is on, and are sent empty
  // when it is off so a non-coordinator carries no orchestration leftovers.
  const [coordinatorMode, setCoordinatorMode] = useState(false)
  const [coordinatorWorkflow, setCoordinatorWorkflow] = useState('')
  const [coordinatorPrompt, setCoordinatorPrompt] = useState('')

  // Selection is controlled by the parent (deep-link aware) when provided,
  // otherwise tracked internally.
  const selectedId = controlledId !== undefined ? controlledId : internalId
  const select = (id: string) => {
    if (onSelectAgent) onSelectAgent(id)
    else setInternalId(id)
  }

  // Keep a valid selection: prefer the current one, else the default, else first.
  useEffect(() => {
    if (selectedId && agents.some((a) => a.id === selectedId)) return
    const fallback = defaultAgentId ?? agents[0]?.id ?? null
    if (fallback) select(fallback)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agents, defaultAgentId, selectedId])

  const selected = agents.find((a) => a.id === selectedId) ?? null

  // Multi-select for bulk roster actions (Ctrl/Cmd+Click, Shift-range).
  const sel = useMultiSelect()
  const orderedIds = useMemo(() => agents.map((a) => a.id), [agents])
  const bulkDelete = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (
      !confirm(
        `${ids.length} ajan silinsin mi?\n\nSohbet geçmişleri KORUNUR — ajan orada "silinmiş" olarak görünür. Zamanlamaları ve sahip oldukları görevler kalıcı olarak silinir. Çalışan bir ajan silinemez.`,
      )
    )
      return
    for (const id of ids) await onDeleteAgent(id)
    sel.clear()
  }

  const submit = () => {
    if (!name.trim()) return
    onCreateAgent(name.trim(), soul.trim(), provider, model, {
      mode: coordinatorMode,
      workflow: coordinatorMode ? coordinatorWorkflow : '',
      prompt: coordinatorMode ? coordinatorPrompt : '',
    })
    setName('')
    setSoul('')
    setCoordinatorMode(false)
    setCoordinatorWorkflow('')
    setCoordinatorPrompt('')
    setShowForm(false)
  }

  return (
    <div className="flex h-full min-h-0 flex-1">
      {/* Left: roster — full-height sibling column (like chat). */}
      <ListPane
        open={rosterOpen}
        onToggle={toggleRoster}
        widthKey="tionharness.agentsListWidth"
        defaultWidth={256}
        label="Ajanlar"
        testId="agents-list-toggle"
        hideRail
      >
        <SidebarHeader title="Ajanlar">
          {onRefresh && (
            <button
              onClick={doRefresh}
              disabled={refreshing}
              data-testid="agents-refresh"
              title="Yenile"
              className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-50"
            >
              <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
            </button>
          )}
        </SidebarHeader>

        {/* Prominent new-agent button (shared chrome). */}
        <NewItemButton
          onClick={() => setShowForm((v) => !v)}
          label="Yeni Ajan"
          title="Yeni ajan"
          testId="agent-create-toggle"
        />

        {showForm && (
          <div className="mx-3 mb-2 space-y-2 rounded-lg bg-[var(--color-surface-2)] p-3">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Ajan adı"
              data-testid="agent-create-name-input"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <PromptEditor
              value={soul}
              onChange={setSoul}
              placeholder="Karakter / sistem promptu (soul)"
              rows={3}
              data-testid="agent-create-soul-textarea"
            />
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
              <span>
                Bu ajanın açtığı <strong>yeni</strong> oturumlar koordinatör olarak başlasın
              </span>
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
                  placeholder="Koordinatör promptu"
                  rows={3}
                />
                <p className="text-xs text-[var(--color-text-dim)]">
                  Yalnızca oturum <strong>koordinatör modundayken</strong>, ortak koordinatör el
                  kitabının hemen ardından sistem bağlamına eklenir. Bu ajana özel delegasyon
                  yönergesi (hangi worker'lar açılsın, iş nasıl bölünsün) buraya yazılır — soul'a
                  değil: mod kapalıyken hiç enjekte edilmez, dolayısıyla{' '}
                  <strong>sıfır token</strong> maliyeti olur.
                </p>
              </>
            )}
            <Button onClick={submit} data-testid="agent-create-submit" className="w-full">
              Oluştur
            </Button>
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {agents.map((a) => (
            <div
              key={a.id}
              data-testid="agent-roster-item"
              data-agent-id={a.id}
              className={`group mb-1 flex w-full items-center rounded-lg pr-1 text-sm transition ${
                sel.isSelected(a.id)
                  ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                  : selectedId === a.id
                    ? SELECTED_ITEM_CLS
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <button
                onClick={(e) => {
                  if (sel.handleClick(e, a.id, orderedIds, selectedId)) return
                  select(a.id)
                }}
                data-testid="agent-roster-select"
                data-agent-id={a.id}
                className="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2 text-left"
              >
                <AgentIdentity
                  agent={a}
                  size="md"
                  active={defaultAgentId === a.id}
                  nameSuffix={
                    <>
                      {/* Which agents orchestrate is otherwise invisible until you open
                          each one — and in a delegation workspace that is the first
                          thing you need to see. */}
                      {a.coordinatorMode && (
                        <span
                          data-testid="agent-coordinator-badge"
                          className="ml-1.5 shrink-0 text-[11px]"
                          title="Koordinatör: açtığı yeni oturumlar worker yönetebilir"
                        >
                          🕸
                        </span>
                      )}
                      <span
                        className="ml-1.5 shrink-0 font-mono text-[10px] opacity-60"
                        title="Ajan ID (klasör adı)"
                      >
                        {a.id}
                      </span>
                    </>
                  }
                  subtitle={
                    resolveModelLabel(catalog, a.provider, a.model) +
                    (defaultAgentId === a.id ? ' · varsayılan' : '')
                  }
                />
              </button>
              {/* Indicator only: the default agent is changed from the agent's own
                  settings, not by clicking around in the roster. */}
              {defaultAgentId === a.id && (
                <span
                  data-testid="agent-default-indicator"
                  data-agent-id={a.id}
                  title="Varsayılan ajan (ajan ayarlarından değiştirilir)"
                  className="ml-1 shrink-0 p-1 text-[var(--color-accent)]"
                >
                  ★
                </span>
              )}
            </div>
          ))}
          {agents.length === 0 && (
            <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
              Henüz ajan yok. + ile oluştur.
            </p>
          )}
        </div>

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
        >
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
      </ListPane>

      {/* Content column: the title bar sits ONLY here (right of the roster), like chat. */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          title="Ajanlar"
          listOpen={rosterOpen}
          onToggleList={toggleRoster}
          subtitle={selected ? `· ${selected.name}` : '· Ajan seçilmedi'}
          right={
            <>
              {selected && (
                <>
                  <CopyPathButton
                    testId="agent-copy-path"
                    getPath={async () => (await api.agentPath(selected.id)).path}
                    title="Ajanın disk üzerindeki JSON dosya yolunu kopyala"
                    onError={onError}
                  />
                </>
              )}
              {/* Toggle the right-hand activity feed from the top bar (like chat's Detay). */}
              <button
                onClick={toggleActivity}
                data-testid="agent-activity-toggle"
                aria-pressed={activityOpen}
                title={activityOpen ? 'Aktivite panelini gizle' : 'Aktivite panelini göster'}
                className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs transition ${
                  activityOpen
                    ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
                }`}
              >
                <Activity size={15} className="shrink-0" />
                <span className="hidden sm:inline">Aktivite</span>
              </button>
            </>
          }
        />
        <div className="flex min-h-0 flex-1">
          {/* Middle: selected agent's settings */}
          <div className="min-w-0 flex-1">
            {selected ? (
              <AgentSettingsForm
                key={selected.id}
                agent={selected}
                dirtyView="agents"
                isDefault={defaultAgentId === selected.id}
                onSetDefault={() => onSetDefault(selected.id)}
                onSave={(p) => onUpdateAgent(selected.id, p)}
                onDuplicate={() => onDuplicateAgent(selected.id)}
                onDelete={async () => {
                  if (
                    confirm(
                      `"${selected.name}" ajanı silinsin mi?\n\nSohbet geçmişi KORUNUR — ajan orada "silinmiş" olarak görünür. Zamanlamaları ve sahip olduğu görevler kalıcı olarak silinir. Çalışan bir ajan silinemez.`,
                    )
                  ) {
                    await onDeleteAgent(selected.id)
                    if (!onSelectAgent) setInternalId(null)
                  }
                }}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-sm text-[var(--color-text-dim)]">
                Düzenlemek için soldan bir ajan seç.
              </div>
            )}
          </div>

          {/* Right: selected agent's live activity feed — opened from the top bar's
          "Aktivite" button. Desktop: a right-hand column. Mobile: a right slide-in
          drawer with a dim backdrop — same as the chat session-detail panel. */}
          {activityOpen && (
            <>
              <div className="fixed inset-0 z-30 bg-black/50 md:hidden" onClick={toggleActivity} />
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
