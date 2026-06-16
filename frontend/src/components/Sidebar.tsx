import { useEffect, useRef, useState } from 'react'
import type { Agent, AgentPatch, Session } from '../types'
import { AgentAvatar } from './AgentAvatar'
import { AgentSettingsModal } from './AgentSettingsModal'
import { ProviderModelSelect } from './ProviderModelSelect'

interface Props {
  agents: Agent[]
  sessions: Session[]
  activeAgentId: string | null
  activeSessionId: string | null
  onSelectAgent: (id: string) => void
  onSelectSession: (id: string) => void
  onCreateAgent: (name: string, soul: string, provider: string, model: string) => void
  onUpdateAgent: (id: string, patch: AgentPatch) => Promise<void>
  onNewSession: () => void
  onRegenerateSessionTitle: (sessionId: string) => void
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
  onUpdateAgent,
  onNewSession,
  onRegenerateSessionTitle,
}: Props) {
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [soul, setSoul] = useState('')
  const [provider, setProvider] = useState('claude-cli')
  const [model, setModel] = useState('')
  const [retitling, setRetitling] = useState<string | null>(null)
  // The agent currently open in the settings modal (gear button), if any.
  const [editingAgent, setEditingAgent] = useState<Agent | null>(null)

  // Draggable sidebar width (persisted). Clamped to a sane range.
  const SIDEBAR_MIN = 200
  const SIDEBAR_MAX = 560
  const [width, setWidth] = useState(() => {
    const saved = Number(localStorage.getItem('swarmgo.sidebarWidth'))
    return saved >= SIDEBAR_MIN && saved <= SIDEBAR_MAX ? saved : 256
  })
  const drag = useRef<{ startX: number; startW: number } | null>(null)

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!drag.current) return
      const next = Math.min(
        SIDEBAR_MAX,
        Math.max(SIDEBAR_MIN, drag.current.startW + (e.clientX - drag.current.startX)),
      )
      setWidth(next)
    }
    const onUp = () => {
      if (!drag.current) return
      drag.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      localStorage.setItem('swarmgo.sidebarWidth', String(width))
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [width])

  const startDrag = (e: React.MouseEvent) => {
    e.preventDefault()
    drag.current = { startX: e.clientX, startW: width }
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'
  }

  const regenerate = async (sessionId: string) => {
    setRetitling(sessionId)
    try {
      await onRegenerateSessionTitle(sessionId)
    } finally {
      setRetitling(null)
    }
  }

  const submit = () => {
    if (!name.trim()) return
    onCreateAgent(name.trim(), soul.trim(), provider, model)
    setName('')
    setSoul('')
    setShowForm(false)
  }

  return (
    <aside
      style={{ width }}
      className="relative flex h-full shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]"
    >
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

      <div className="max-h-56 overflow-y-auto px-2">
        {agents.map((a) => (
          <div
            key={a.id}
            className={`group mb-1 flex w-full items-center rounded-lg pr-1 text-sm transition ${
              activeAgentId === a.id
                ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
            }`}
          >
            <button
              onClick={() => onSelectAgent(a.id)}
              className="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2 text-left"
            >
              <AgentAvatar agent={a} size={32} active={activeAgentId === a.id} />
              <span className="flex min-w-0 flex-col">
                <span className="truncate font-medium">{a.name}</span>
                <span className="truncate text-xs opacity-70">{a.provider}</span>
              </span>
            </button>
            <button
              onClick={() => setEditingAgent(a)}
              title="Ajan ayarları"
              className="ml-1 shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100"
            >
              ⚙
            </button>
          </div>
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
          <div
            key={s.id}
            className={`group mb-1 flex w-full items-center rounded-lg pr-2 text-sm transition ${
              activeSessionId === s.id
                ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
                : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
            }`}
          >
            <button
              onClick={() => onSelectSession(s.id)}
              className="flex min-w-0 flex-1 items-center justify-between px-3 py-2 text-left"
            >
              <span className="truncate">{s.title || 'Yeni sohbet'}</span>
              <span className="ml-2 text-xs opacity-60">{s.messageCount}</span>
            </button>
            <button
              onClick={() => regenerate(s.id)}
              disabled={retitling === s.id || s.messageCount === 0}
              title="Başlığı yeniden oluştur"
              className="ml-1 shrink-0 text-xs text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-accent)] group-hover:opacity-100 disabled:opacity-20"
            >
              {retitling === s.id ? '…' : '⟳'}
            </button>
          </div>
        ))}
        {activeAgentId && sessions.length === 0 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            Oturum yok. + ile başlat.
          </p>
        )}
      </div>

      {editingAgent && (
        <AgentSettingsModal
          agent={editingAgent}
          onClose={() => setEditingAgent(null)}
          onSave={(patch) => onUpdateAgent(editingAgent.id, patch)}
        />
      )}

      {/* Drag handle to resize the sidebar. */}
      <div
        onMouseDown={startDrag}
        title="Genişliği ayarla"
        className="absolute right-0 top-0 h-full w-1 cursor-col-resize bg-transparent transition hover:bg-[var(--color-accent)]"
      />
    </aside>
  )
}
