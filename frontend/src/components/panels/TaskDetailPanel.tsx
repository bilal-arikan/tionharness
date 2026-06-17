import { useEffect, useState } from 'react'
import { RefreshCw, Loader2 } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Task, Run, BoardState, Flow, TurnStep } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { TurnSteps } from '../chat/TurnSteps'

// Transient live-stream step kinds not worth rendering in the compact task panel
// (incremental text/tool chunks, retractions, interactive prompts). The full
// transcript — including the streamed answer — lands in the Activity view.
const TRANSIENT_STEPS = new Set(['delta', 'tool_delta', 'tombstone', 'ask', 'steer'])

const BOARD_STATES: { key: BoardState; label: string }[] = [
  { key: 'todo', label: 'Yapılacak' },
  { key: 'in_progress', label: 'Devam Eden' },
  { key: 'review', label: 'İnceleme' },
  { key: 'done', label: 'Bitti' },
  { key: 'failed', label: 'Başarısız' },
]

const STATUS_COLOR: Record<string, string> = {
  success: 'text-[var(--color-success)]',
  failure: 'text-[var(--color-danger)]',
  running: 'text-[var(--color-warning)]',
  pending: 'text-[var(--color-text-dim)]',
}

// Cron presets for the in-panel schedule binding (mirror Schedules.tsx).
const CRON_PRESETS: { label: string; expr: string }[] = [
  { label: 'Her 5 dakika', expr: '*/5 * * * *' },
  { label: 'Saat başı', expr: '0 * * * *' },
  { label: 'Her gün 09:00', expr: '0 9 * * *' },
  { label: 'Pazartesi 08:00', expr: '0 8 * * 1' },
]

interface Props {
  task: Task
  agents: Agent[]
  flows: Flow[]
  onClose: () => void
  // Called with the persisted task so the board can update its copy in place.
  onSaved: (task: Task) => void
  // Called after a successful delete so the board can drop the card.
  onDeleted: (id: string) => void
  onError: (msg: string) => void
}

// TaskDetailPanel is the right-hand inspector/editor for a single Kanban card.
// It both edits the task (title/prompt/description/owner/column) and holds the
// full action set the card no longer carries: run now, bind to a cron schedule,
// view run history, regenerate title and delete.
export function TaskDetailPanel({ task, agents, flows, onClose, onSaved, onDeleted, onError }: Props) {
  const [title, setTitle] = useState(task.title)
  const [prompt, setPrompt] = useState(task.prompt)
  const [description, setDescription] = useState(task.description)
  const [ownerAgentId, setOwnerAgentId] = useState(task.ownerAgentId)
  const [flowId, setFlowId] = useState(task.flowId)
  const [boardState, setBoardState] = useState<BoardState>(task.boardState)
  const [saving, setSaving] = useState(false)

  const [running, setRunning] = useState(false)
  // Live activity steps streamed while a run is in flight (cleared on each run).
  const [liveSteps, setLiveSteps] = useState<TurnStep[]>([])
  const [retitling, setRetitling] = useState(false)
  const [runs, setRuns] = useState<Run[]>([])
  const [runsLoading, setRunsLoading] = useState(false)

  const [schedCron, setSchedCron] = useState('*/5 * * * *')
  const [schedBusy, setSchedBusy] = useState(false)
  const [schedDone, setSchedDone] = useState(false)

  // Reseed the form + reload run history when the selected card changes.
  useEffect(() => {
    setTitle(task.title)
    setPrompt(task.prompt)
    setDescription(task.description)
    setOwnerAgentId(task.ownerAgentId)
    setFlowId(task.flowId)
    setBoardState(task.boardState)
    setSchedDone(false)
    let alive = true
    setRunsLoading(true)
    api
      .listTaskRuns(task.id)
      .then((r) => alive && setRuns(r))
      .catch((e) => alive && onError((e as Error).message))
      .finally(() => alive && setRunsLoading(false))
    return () => {
      alive = false
    }
  }, [task, onError])

  const dirty =
    title !== task.title ||
    prompt !== task.prompt ||
    description !== task.description ||
    ownerAgentId !== task.ownerAgentId ||
    flowId !== task.flowId ||
    boardState !== task.boardState

  const save = async () => {
    setSaving(true)
    try {
      const updated = await api.updateTask(task.id, {
        title: title.trim(),
        prompt: prompt.trim(),
        description: description.trim(),
        ownerAgentId,
        flowId,
        boardState,
      })
      onSaved(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // Run the task now over SSE: stream live activity steps, then reflect the
  // finished run on the board column + last status and prepend it to history.
  const run = async () => {
    setRunning(true)
    setLiveSteps([])
    try {
      await api.runTaskStream(task.id, {
        onStep: (st) => {
          if (TRANSIENT_STEPS.has(st.kind)) return
          setLiveSteps((prev) => [...prev, st])
        },
        onReply: ({ run: r }) => {
          onSaved({
            ...task,
            boardState: r.status === 'success' ? 'done' : 'failed',
            lastRunStatus: r.status,
          })
          setRuns((prev) => [r, ...prev])
        },
        onError: (e) => onError(e),
      })
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunning(false)
    }
  }

  const retitle = async () => {
    setRetitling(true)
    try {
      const updated = await api.generateTaskTitle(task.id)
      onSaved(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRetitling(false)
    }
  }

  // Create a cron schedule bound to this task (taskId set → scheduler RunTask).
  const bindSchedule = async () => {
    if (!ownerAgentId) {
      onError('Zamanlamak için önce göreve bir ajan atayın')
      return
    }
    if (!schedCron.trim()) return
    setSchedBusy(true)
    try {
      await api.createSchedule({
        agentId: ownerAgentId,
        taskId: task.id,
        cronExpr: schedCron.trim(),
        enabled: true,
      })
      setSchedDone(true)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSchedBusy(false)
    }
  }

  const remove = async () => {
    if (!confirm(`"${task.title}" silinsin mi?`)) return
    try {
      await api.deleteTask(task.id)
      onDeleted(task.id)
    } catch (e) {
      onError((e as Error).message)
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
          <div className="flex items-center gap-1.5">
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <button
              onClick={retitle}
              disabled={retitling}
              title="AI ile başlığı yeniden oluştur"
              className="shrink-0 rounded border border-[var(--color-border)] px-2 py-1.5 text-sm text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-30"
            >
              {retitling ? '…' : <RefreshCw size={14} />}
            </button>
          </div>
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

        <Field label="Akış (opsiyonel) — seçilirse çalıştırınca akış koşar">
          <select
            value={flowId}
            onChange={(e) => setFlowId(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          >
            <option value="">🔀 Akış yok (prompt ajana gider)</option>
            {flows.map((f) => (
              <option key={f.id} value={f.id}>
                🔀 {f.name}
              </option>
            ))}
          </select>
          {flowId && (
            <p className="text-[11px] text-[var(--color-text-dim)]">
              Bu görev çalıştırılınca prompt, akışa girdi olarak verilir.
            </p>
          )}
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

        <div className="flex items-center gap-2 border-t border-[var(--color-border)] pt-3">
          <button
            onClick={save}
            disabled={!dirty || saving}
            className="rounded bg-[var(--color-accent)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-40"
          >
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </button>
          {dirty && !saving && (
            <span className="text-[11px] text-[var(--color-warning)]">kaydedilmemiş değişiklik</span>
          )}
        </div>

        {/* Run now — a flow-backed task runs its flow; otherwise the owner agent.
            Availability/label key off the persisted task, not the unsaved form. */}
        <Section title="Çalıştırma">
          <button
            onClick={run}
            disabled={(!task.ownerAgentId && !task.flowId) || running}
            className="w-full rounded bg-[var(--color-accent-soft)] px-3 py-1.5 text-sm text-[var(--color-text)] transition hover:opacity-90 disabled:opacity-30"
            title={task.flowId ? 'Akışı şimdi çalıştır' : task.ownerAgentId ? 'Görevi şimdi çalıştır' : 'Önce ajan veya akış ata'}
          >
            {running ? 'Çalışıyor…' : task.flowId ? '▶ Akışı çalıştır' : '▶ Şimdi çalıştır'}
          </button>
          {!task.ownerAgentId && !task.flowId && (
            <p className="mt-1 text-[11px] text-[var(--color-text-dim)]">
              Çalıştırmak için bir ajan ya da akış atayıp kaydedin.
            </p>
          )}
          {dirty && (
            <p className="mt-1 text-[11px] text-[var(--color-warning)]">
              Çalıştırma kayıtlı görevi kullanır — önce Kaydet.
            </p>
          )}

          {/* Live activity while the run streams. The full transcript (incl. the
              streamed answer) is viewable in the Activity view. */}
          {(running || liveSteps.length > 0) && (
            <div className="mt-3 rounded border border-[var(--color-border)] bg-[var(--color-bg)] p-2">
              <div className="mb-1.5 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                {running && <Loader2 size={11} className="animate-spin text-[var(--color-accent)]" />}
                Canlı etkinlik
              </div>
              {liveSteps.length > 0 ? (
                <TurnSteps steps={liveSteps} />
              ) : (
                <p className="text-[11px] text-[var(--color-text-dim)]">Başlatılıyor…</p>
              )}
            </div>
          )}
        </Section>

        {/* Schedule binding */}
        <Section title="Zamanlama (cron)">
          <div className="flex flex-wrap items-center gap-1.5">
            <select
              value={CRON_PRESETS.some((p) => p.expr === schedCron) ? schedCron : ''}
              onChange={(e) => e.target.value && setSchedCron(e.target.value)}
              className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-1 text-xs outline-none"
            >
              <option value="">Özel…</option>
              {CRON_PRESETS.map((p) => (
                <option key={p.expr} value={p.expr}>
                  {p.label}
                </option>
              ))}
            </select>
            <input
              value={schedCron}
              onChange={(e) => setSchedCron(e.target.value)}
              placeholder="dk sa gün ay haftagünü"
              className="w-32 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-1 font-mono text-xs outline-none focus:border-[var(--color-accent)]"
            />
            <button
              onClick={bindSchedule}
              disabled={schedBusy || !ownerAgentId}
              className="rounded bg-[var(--color-accent)] px-2.5 py-1 text-xs font-medium text-white transition hover:opacity-90 disabled:opacity-40"
              title={ownerAgentId ? 'Cron zamanlamasına bağla' : 'Önce ajan ata'}
            >
              {schedBusy ? '…' : 'Bağla'}
            </button>
          </div>
          {schedDone && (
            <p className="mt-1 text-[11px] text-[var(--color-success)]">
              ✓ Zamanlama oluşturuldu — Zamanlamalar ekranından yönetebilirsin.
            </p>
          )}
        </Section>

        {/* Run history */}
        <Section title="Geçmiş">
          {runsLoading ? (
            <p className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</p>
          ) : runs.length === 0 ? (
            <p className="text-xs text-[var(--color-text-dim)]">Henüz çalıştırma yok.</p>
          ) : (
            <div className="space-y-1">
              {runs.map((r) => (
                <div key={r.id} className="rounded bg-[var(--color-bg)] p-1.5 text-xs">
                  <span className={STATUS_COLOR[r.status] ?? ''}>{r.status}</span>
                  <span className="ml-1 text-[var(--color-text-dim)]">({r.trigger})</span>
                  <div className="mt-0.5 line-clamp-3 text-[var(--color-text)]">
                    {r.error || r.output}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Section>

        {/* Danger zone */}
        <div className="mt-auto border-t border-[var(--color-border)] pt-3">
          <button
            onClick={remove}
            className="w-full rounded px-3 py-1.5 text-sm text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
          >
            🗑 Görevi sil
          </button>
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

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="border-t border-[var(--color-border)] pt-3">
      <div className="mb-2 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        {title}
      </div>
      {children}
    </section>
  )
}
