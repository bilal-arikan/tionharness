import { useEffect, useState } from 'react'
import { Sparkles, X, Loader2 } from 'lucide-react'
import type { Agent } from '../../types'
import { api } from '../../api'
import { AgentPicker } from '../agents/AgentPicker'
import { Button } from '../common'

interface Props {
  agents: Agent[]
  onClose: () => void
  // Called with the new session id once a spawn succeeds.
  onSpawned: (sessionId: string) => void
  onError: (msg: string) => void
}

// SpawnSessionModal launches a new, independent session for a chosen agent from a
// single prompt — fire-and-forget. The agent works the prompt in the background
// and the run shows up live in the activity feed. modelOverride is optional and
// swaps only the model (the agent's provider is unchanged).
export function SpawnSessionModal({ agents, onClose, onSpawned, onError }: Props) {
  const [agentId, setAgentId] = useState(agents[0]?.id ?? '')
  const [prompt, setPrompt] = useState('')
  const [modelOverride, setModelOverride] = useState('')
  const [busy, setBusy] = useState(false)

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const canSubmit = agentId !== '' && prompt.trim() !== '' && !busy

  const submit = async () => {
    if (!canSubmit) return
    setBusy(true)
    try {
      const res = await api.spawnSession(agentId, prompt.trim(), modelOverride.trim())
      onSpawned(res.sessionId)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onMouseDown={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Yeni oturum başlat"
        data-testid="spawn-session-modal"
        className="w-full max-w-lg rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-5 shadow-xl"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-center gap-2">
          <Sparkles size={18} className="text-[var(--color-accent)]" />
          <h2 className="text-base font-semibold">Yeni oturum başlat</h2>
          <button
            onClick={onClose}
            className="ml-auto text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
            title="Kapat"
          >
            <X size={18} />
          </button>
        </div>

        <p className="mb-4 text-xs text-[var(--color-text-dim)]">
          Seçtiğin ajan, bu prompt'u arka planda bağımsız bir oturumda çalışır. Beklemezsin —
          ilerleme Aktivite akışında canlı görünür.
        </p>

        <label className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Ajan</label>
        <div className="mb-3">
          <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />
        </div>

        <label className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">Prompt</label>
        <textarea
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          rows={5}
          autoFocus
          placeholder="Ajanın üzerinde çalışacağı görev…"
          className="mb-3 w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
        />

        <label className="mb-1 block text-xs font-medium text-[var(--color-text-dim)]">
          Model (opsiyonel)
        </label>
        <input
          value={modelOverride}
          onChange={(e) => setModelOverride(e.target.value)}
          placeholder="Ajanın varsayılan modeli kullanılır"
          className="mb-5 w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
        />

        <div className="flex justify-end gap-2">
          <button
            onClick={onClose}
            className="rounded-md border border-[var(--color-border)] px-3 py-1.5 text-sm text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
          >
            İptal
          </button>
          <Button onClick={submit} disabled={!canSubmit} className="flex items-center gap-1.5">
            {busy ? <Loader2 size={14} className="animate-spin" /> : <Sparkles size={14} />}
            Başlat
          </Button>
        </div>
      </div>
    </div>
  )
}
