import { useEffect, useState } from 'react'
import type { Agent, AgentPatch } from '../../types'
import { AgentAvatar } from './AgentAvatar'
import { ProviderModelSelect } from './ProviderModelSelect'
import { AgentSettingsForm } from './AgentSettingsForm'

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
}: Props) {
  const [internalId, setInternalId] = useState<string | null>(null)
  const [showForm, setShowForm] = useState(false)
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
    <div className="flex h-full min-h-0">
      {/* Left: roster */}
      <div className="flex w-64 shrink-0 flex-col border-r border-[var(--color-border)]">
        <div className="flex items-center justify-between px-4 pt-4 pb-1">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Ajanlar
          </span>
          <button
            onClick={() => setShowForm((v) => !v)}
            className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
            title="Yeni ajan"
          >
            +
          </button>
        </div>

        {showForm && (
          <div className="mx-3 mb-2 space-y-2 rounded-lg bg-[var(--color-surface-2)] p-3">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Ajan adı"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <textarea
              value={soul}
              onChange={(e) => setSoul(e.target.value)}
              placeholder="Karakter / sistem promptu (soul)"
              rows={3}
              className="w-full resize-none rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <ProviderModelSelect
              provider={provider}
              model={model}
              onChange={(p, m) => {
                setProvider(p)
                setModel(m)
              }}
            />
            <button
              onClick={submit}
              className="w-full rounded bg-[var(--color-accent)] py-1 text-sm font-medium text-white hover:opacity-90"
            >
              Oluştur
            </button>
          </div>
        )}

        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {agents.map((a) => (
            <div
              key={a.id}
              className={`group mb-1 flex w-full items-center rounded-lg pr-1 text-sm transition ${
                selectedId === a.id
                  ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <button
                onClick={() => select(a.id)}
                className="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2 text-left"
              >
                <AgentAvatar agent={a} size={32} active={defaultAgentId === a.id} />
                <span className="flex min-w-0 flex-col">
                  <span className="truncate font-medium">{a.name}</span>
                  <span className="truncate text-xs opacity-70">
                    {a.provider}
                    {defaultAgentId === a.id ? ' · varsayılan' : ''}
                  </span>
                </span>
              </button>
              <button
                onClick={() => onSetDefault(a.id)}
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

      {/* Right: selected agent's settings */}
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
    </div>
  )
}
