import { useState } from 'react'
import type { Agent, Session } from '../types'

interface Props {
  agents: Agent[]
  sessions: Session[]
  activeAgentId: string | null
  activeSessionId: string | null
  onSelectAgent: (id: string) => void
  onSelectSession: (id: string) => void
  onCreateAgent: (name: string, soul: string, provider: string) => void
  onNewSession: () => void
}

// Sidebar is the middle column: the agent roster and the active agent's
// sessions. Workspace switching and view navigation live in the NavRail.
export function Sidebar({
  agents,
  sessions,
  activeAgentId,
  activeSessionId,
  onSelectAgent,
  onSelectSession,
  onCreateAgent,
  onNewSession,
}: Props) {
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [soul, setSoul] = useState('')
  const [provider, setProvider] = useState('claude-cli')

  const submit = () => {
    if (!name.trim()) return
    onCreateAgent(name.trim(), soul.trim(), provider)
    setName('')
    setSoul('')
    setShowForm(false)
  }

  return (
    <aside className="flex h-full w-64 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]">
      {/* Agents */}
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
          <select
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
          >
            <option value="claude-cli">claude-cli (abonelik)</option>
            <option value="anthropic">anthropic (API key)</option>
          </select>
          <button
            onClick={submit}
            className="w-full rounded bg-[var(--color-accent)] py-1 text-sm font-medium text-white hover:opacity-90"
          >
            Oluştur
          </button>
        </div>
      )}

      <div className="max-h-56 overflow-y-auto px-2">
        {agents.map((a) => (
          <button
            key={a.id}
            onClick={() => onSelectAgent(a.id)}
            className={`mb-1 flex w-full flex-col items-start rounded-lg px-3 py-2 text-left text-sm transition ${
              activeAgentId === a.id
                ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
            }`}
          >
            <span className="font-medium">{a.name}</span>
            <span className="text-xs opacity-70">{a.provider}</span>
          </button>
        ))}
        {agents.length === 0 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            Henüz ajan yok. + ile oluştur.
          </p>
        )}
      </div>

      {/* Sessions */}
      <div className="flex items-center justify-between px-4 pt-4 pb-1">
        <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          Oturumlar
        </span>
        <button
          onClick={onNewSession}
          disabled={!activeAgentId}
          className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-30"
          title="Yeni oturum"
        >
          +
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-2">
        {sessions.map((s) => (
          <button
            key={s.id}
            onClick={() => onSelectSession(s.id)}
            className={`mb-1 flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm transition ${
              activeSessionId === s.id
                ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
            }`}
          >
            <span className="truncate">{s.title || 'Yeni sohbet'}</span>
            <span className="ml-2 text-xs opacity-60">{s.messageCount}</span>
          </button>
        ))}
        {activeAgentId && sessions.length === 0 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            Oturum yok. + ile başlat.
          </p>
        )}
      </div>
    </aside>
  )
}
