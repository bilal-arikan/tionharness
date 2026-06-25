import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import type { Agent, AgentPatch } from '../../types'
import { AgentIdentity } from './AgentIdentity'
import { ProviderModelSelect } from './ProviderModelSelect'
import { useCatalog, resolveModelLabel } from '../../lib/catalog'
import { AgentSettingsForm } from './AgentSettingsForm'
import { AgentActivityPanel } from './AgentActivityPanel'
import { Button } from '../common'

interface Props {
  agents: Agent[]
  defaultAgentId: string | null
  /** Controlled selection (deep-link aware); falls back to internal state. */
  selectedId?: string | null
  onSelectAgent?: (id: string) => void
  /** Set an agent as the default for new chats. */
  onSetDefault: (id: string) => void
  onCreateAgent: (name: string, soul: string, provider: string, model: string) => void
  onUpdateAgent: (id: string, patch: AgentPatch) => Promise<void>
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
  onDeleteAgent,
  onRefresh,
  onError,
  onOpenExecution,
}: Props) {
  const [internalId, setInternalId] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const catalog = useCatalog()

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
  const [provider, setProvider] = useState('claude-cli')
  const [model, setModel] = useState('')

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

  const submit = () => {
    if (!name.trim()) return
    onCreateAgent(name.trim(), soul.trim(), provider, model)
    setName('')
    setSoul('')
    setShowForm(false)
  }

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: roster */}
      <div className="flex w-64 shrink-0 flex-col border-r border-[var(--color-border)]">
        <div className="flex items-center justify-between px-4 pt-4 pb-1">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Ajanlar
          </span>
          <div className="flex items-center gap-1.5">
            {onRefresh && (
              <button
                onClick={doRefresh}
                disabled={refreshing}
                data-testid="agents-refresh"
                className="text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-50"
                title="Listeyi yenile"
              >
                <RefreshCw size={13} className={refreshing ? 'animate-spin' : ''} />
              </button>
            )}
            <button
              onClick={() => setShowForm((v) => !v)}
              data-testid="agent-create-toggle"
              className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
              title="Yeni ajan"
            >
              +
            </button>
          </div>
        </div>

        {showForm && (
          <div className="mx-3 mb-2 space-y-2 rounded-lg bg-[var(--color-surface-2)] p-3">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Ajan adı"
              data-testid="agent-create-name-input"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <textarea
              value={soul}
              onChange={(e) => setSoul(e.target.value)}
              placeholder="Karakter / sistem promptu (soul)"
              rows={3}
              data-testid="agent-create-soul-textarea"
              className="w-full resize-none rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <div data-testid="agent-create-provider-wrap" className="contents">
              <ProviderModelSelect
                provider={provider}
                model={model}
                onChange={(p, m) => {
                  setProvider(p)
                  setModel(m)
                }}
              />
            </div>
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
                selectedId === a.id
                  ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <button
                onClick={() => select(a.id)}
                data-testid="agent-roster-select"
                data-agent-id={a.id}
                className="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2 text-left"
              >
                <AgentIdentity
                  agent={a}
                  size="md"
                  active={defaultAgentId === a.id}
                  nameSuffix={
                    <span className="ml-1.5 shrink-0 font-mono text-[10px] opacity-60" title="Ajan ID (klasör adı)">
                      {a.id}
                    </span>
                  }
                  subtitle={
                    resolveModelLabel(catalog, a.provider, a.model) +
                    (defaultAgentId === a.id ? ' · varsayılan' : '')
                  }
                />
              </button>
              <button
                onClick={() => onSetDefault(a.id)}
                data-testid="agent-set-default"
                data-agent-id={a.id}
                title={defaultAgentId === a.id ? 'Varsayılan ajan' : 'Varsayılan yap'}
                className={`ml-1 shrink-0 rounded p-1 transition ${
                  defaultAgentId === a.id
                    ? 'text-[var(--color-accent)]'
                    : 'text-[var(--color-text-dim)] opacity-0 hover:text-[var(--color-accent)] group-hover:opacity-100'
                }`}
              >
                {defaultAgentId === a.id ? '★' : '☆'}
              </button>
            </div>
          ))}
          {agents.length === 0 && (
            <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
              Henüz ajan yok. + ile oluştur.
            </p>
          )}
        </div>
      </div>

      {/* Middle: selected agent's settings */}
      <div className="min-w-0 flex-1">
        {selected ? (
          <AgentSettingsForm
            key={selected.id}
            agent={selected}
            onSave={(p) => onUpdateAgent(selected.id, p)}
            onDelete={async () => {
              if (confirm(`"${selected.name}" ajanı ve sahip olduğu oturumlar kalıcı olarak silinsin mi?`)) {
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

      {/* Right: selected agent's live activity feed */}
      <AgentActivityPanel
        agentId={selected?.id ?? null}
        onError={onError ?? (() => {})}
        onOpenExecution={onOpenExecution}
      />
    </div>
  )
}
