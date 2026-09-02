// ForkModal — "buradan çatalla" (Rota F5): spawn a worker under a coordinator
// session straight from the canvas, with the same brief spawn_worker would
// take from inside the coordinator's turn.
import { useState } from 'react'
import { GitFork, Loader2, X } from 'lucide-react'
import { api } from '@/api'
import { ModalOverlay, toast } from '@/shared/components'

interface Props {
  sessionId: string
  sessionTitle?: string
  onClose: () => void
}

const PROFILES = ['explore', 'planner', 'coder', 'reviewer', 'validator']

export function ForkModal({ sessionId, sessionTitle, onClose }: Props) {
  const [agent, setAgent] = useState('explore')
  const [task, setTask] = useState('')
  const [coordinator, setCoordinator] = useState(false)
  const [busy, setBusy] = useState(false)

  const submit = async () => {
    if (!agent.trim() || !task.trim()) return
    setBusy(true)
    try {
      const r = await api.spawnWorker(sessionId, {
        agent: agent.trim(),
        task: task.trim(),
        coordinator,
      })
      toast.success(
        r.queued
          ? `Worker kuyruğa alındı (${r.agentName}, sıra ${r.queuePosition})`
          : `Worker açıldı: ${r.agentName} (${r.sessionId})`,
      )
      onClose()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        className="flex w-[min(560px,92vw)] flex-col gap-3 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-xl"
        data-testid="fork-modal"
      >
        <div className="flex items-center gap-2">
          <GitFork size={16} className="opacity-70" />
          <span className="text-sm font-semibold">Buradan çatalla</span>
          <span className="truncate text-xs text-[var(--color-text-dim)]">
            {sessionTitle || sessionId} altında worker aç
          </span>
          <button
            type="button"
            onClick={onClose}
            className="ml-auto rounded p-1 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            aria-label="Kapat"
          >
            <X size={16} />
          </button>
        </div>
        <label className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
          Ajan / profil
          <input
            list="fork-profiles"
            value={agent}
            onChange={(e) => setAgent(e.target.value)}
            className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm text-[var(--color-text)]"
            placeholder="explore | planner | coder | reviewer | validator | ajan adı"
          />
          <datalist id="fork-profiles">
            {PROFILES.map((p) => (
              <option key={p} value={p} />
            ))}
          </datalist>
        </label>
        <label className="flex flex-col gap-1 text-xs text-[var(--color-text-dim)]">
          Görev (worker yalnız bunu görür: dosya yolları, satırlar, "bitti" ne demek)
          <textarea
            value={task}
            onChange={(e) => setTask(e.target.value)}
            rows={6}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm text-[var(--color-text)]"
            placeholder="Self-contained brief…"
          />
        </label>
        <label className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]">
          <input
            type="checkbox"
            checked={coordinator}
            onChange={(e) => setCoordinator(e.target.checked)}
          />
          Alt-koordinatör olsun (kendi worker'larını açabilir)
        </label>
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg border border-[var(--color-border)] px-3 py-1 text-xs text-[var(--color-text-dim)]"
          >
            Vazgeç
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy || !agent.trim() || !task.trim()}
            className="flex items-center gap-1 rounded-lg border border-[var(--color-accent)] px-3 py-1 text-xs text-[var(--color-accent)] disabled:opacity-40"
            data-testid="fork-submit"
          >
            {busy ? <Loader2 size={12} className="animate-spin" /> : <GitFork size={12} />}
            Worker aç
          </button>
        </div>
      </div>
    </ModalOverlay>
  )
}
