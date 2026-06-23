import { useState } from 'react'
import { Settings } from 'lucide-react'
import type { Agent, AgentPatch } from '../../types'
import { AgentAvatar } from './AgentAvatar'
import { AgentSettingsModal } from './AgentSettingsModal'
import { ProviderModelSelect } from './ProviderModelSelect'
import { Button } from '../common'

interface Props {
  agents: Agent[]
  /** Highlighted agent = the default for NEW sessions. */
  defaultAgentId: string | null
  onSelectAgent: (id: string) => void
  onCreateAgent: (name: string, soul: string, provider: string, model: string) => void
  onUpdateAgent: (id: string, patch: AgentPatch) => Promise<void>
  /** panel = full main-area layout (Ajanlar view); otherwise a fixed sidebar. */
  panel?: boolean
}

// AgentRoster lists the workspace's agents and doubles as the "default agent for
// new chats" picker. It hosts agent creation + the per-agent settings modal.
// Used both as the sidebar for agent-scoped views (memory/tools) and as the
// standalone "Ajanlar" view (panel mode).
export function AgentRoster({
  agents,
  defaultAgentId,
  onSelectAgent,
  onCreateAgent,
  onUpdateAgent,
  panel = false,
}: Props) {
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [soul, setSoul] = useState('')
  const [provider, setProvider] = useState('claude-cli')
  const [model, setModel] = useState('')
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)

  const submit = () => {
    if (!name.trim()) return
    onCreateAgent(name.trim(), soul.trim(), provider, model)
    setName('')
    setSoul('')
    setShowForm(false)
  }

  const body = (
    <>
      <div className="flex items-center justify-between px-4 pt-4 pb-1">
        <span
          className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]"
          title="Seçili ajan = yeni sohbetlerin varsayılanı. Sohbette @ ile başka ajanları da çağırabilirsin."
        >
          Ajanlar · varsayılan
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
          <Button onClick={submit} className="w-full">
            Oluştur
          </Button>
        </div>
      )}

      <div className={`overflow-y-auto px-2 ${panel ? 'flex-1' : 'flex-1'}`}>
        {agents.map((a) => (
          <div
            key={a.id}
            className={`group mb-1 flex w-full items-center rounded-lg pr-1 text-sm transition ${
              defaultAgentId === a.id
                ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
            }`}
          >
            <button
              onClick={() => onSelectAgent(a.id)}
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
              onClick={() => setEditingAgent(a)}
              title="Ajan ayarları"
              className="ml-1 shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100"
            >
              <Settings size={16} />
            </button>
          </div>
        ))}
        {agents.length === 0 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">Henüz ajan yok. + ile oluştur.</p>
        )}
      </div>

      {editingAgent && (
        <AgentSettingsModal
          agent={editingAgent}
          onClose={() => setEditingAgent(null)}
          onSave={(patch) => onUpdateAgent(editingAgent.id, patch)}
        />
      )}
    </>
  )

  if (panel) {
    return (
      <div className="mx-auto flex h-full w-full max-w-xl flex-col">{body}</div>
    )
  }
  return (
    <aside className="flex h-full w-64 shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]">
      {body}
    </aside>
  )
}
