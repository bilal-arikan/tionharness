import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { Agent, Task, BoardState } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'

const BOARD_STATES: { key: BoardState; label: string }[] = [
  { key: 'todo', label: 'Yapılacak' },
  { key: 'in_progress', label: 'Devam Eden' },
  { key: 'review', label: 'İnceleme' },
  { key: 'done', label: 'Bitti' },
  { key: 'failed', label: 'Başarısız' },
]

interface Props {
  task: Task
  agents: Agent[]
  onClose: () => void
  // Called with the persisted task so the board can update its copy in place.
  onSaved: (task: Task) => void
  onError: (msg: string) => void
}

// TaskDetailPanel is the right-hand inspector/editor for a single Kanban card:
// view and edit title, prompt, description, owner agent and board column. Form
// state is seeded from the task and reseeded whenever a different card is opened.
export function TaskDetailPanel({ task, agents, onClose, onSaved, onError }: Props) {
  const [title, setTitle] = useState(task.title)
  const [prompt, setPrompt] = useState(task.prompt)
  const [description, setDescription] = useState(task.description)
  const [ownerAgentId, setOwnerAgentId] = useState(task.ownerAgentId)
  const [boardState, setBoardState] = useState<BoardState>(task.boardState)
  const [saving, setSaving] = useState(false)

  // Reseed the form when the selected card changes (panel stays mounted).
  useEffect(() => {
    setTitle(task.title)
    setPrompt(task.prompt)
    setDescription(task.description)
    setOwnerAgentId(task.ownerAgentId)
    setBoardState(task.boardState)
  }, [task])

  const dirty =
    title !== task.title ||
    prompt !== task.prompt ||
    description !== task.description ||
    ownerAgentId !== task.ownerAgentId ||
    boardState !== task.boardState

  const save = async () => {
    setSaving(true)
    try {
      const updated = await api.updateTask(task.id, {
        title: title.trim(),
        prompt: prompt.trim(),
        description: description.trim(),
        ownerAgentId,
        boardState,
      })
      onSaved(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <aside className="flex h-full w-96 shrink-0 flex-col overflow-y-auto border-l border-[var(--color-border)] bg-[var(--color-surface)]">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
        <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Görev detayı
        </span>
        <button
          onClick={onClose}
          title="Paneli kapat"
          className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          ✕
        </button>
      </div>

      <div className="flex flex-1 flex-col gap-4 px-4 py-4">
        <Field label="Başlık">
          <input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Prompt — ajana verilecek talimat">
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            rows={5}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Açıklama (opsiyonel)">
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Ajan">
          <AgentPicker
            agents={agents}
            value={ownerAgentId}
            onChange={setOwnerAgentId}
            placeholder="Ajan seç (opsiyonel)"
          />
        </Field>

        <Field label="Durum (kolon)">
          <div className="flex flex-wrap gap-1.5">
            {BOARD_STATES.map((s) => (
              <button
                key={s.key}
                onClick={() => setBoardState(s.key)}
                className={`rounded-full px-2.5 py-1 text-xs transition ${
                  boardState === s.key
                    ? 'bg-[var(--color-accent)] text-white'
                    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                }`}
              >
                {s.label}
              </button>
            ))}
          </div>
        </Field>

        <div className="mt-auto flex items-center gap-2 border-t border-[var(--color-border)] pt-3">
          <button
            onClick={save}
            disabled={!dirty || saving}
            className="rounded bg-[var(--color-accent)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-40"
          >
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </button>
          <button
            onClick={onClose}
            className="rounded border border-[var(--color-border)] px-3 py-1.5 text-sm transition hover:bg-[var(--color-bg)]"
          >
            Kapat
          </button>
          {dirty && !saving && (
            <span className="ml-auto text-[11px] text-amber-400">kaydedilmemiş değişiklik</span>
          )}
        </div>
      </div>
    </aside>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        {label}
      </span>
      {children}
    </label>
  )
}
