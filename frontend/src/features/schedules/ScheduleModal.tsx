import { useState } from 'react'
import { Clock, Sparkles, X } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Flow, Schedule, ScheduleSessionMode } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { toast } from '@/shared/components'
import { COLUMN_ACCENT } from './automationMeta'
import { PRESET_GROUPS } from './cronPresets'
import { FormModal } from './FormModal'
import { Field, FlowPicker, TargetModeToggle, inputCls } from './pickers'
import { localInputToUnix, unixToLocalInput } from './timeUtils'
import { FieldError } from './FieldError'
import { useFieldErrors } from './useFieldErrors'

interface Props {
  agents: Agent[]
  flows: Flow[]
  /** null = create a new schedule; otherwise edit this one. */
  editing: Schedule | null
  onClose: () => void
  onSaved: (s: Schedule, isNew: boolean) => void
  /** Edit mode only: delete this schedule (the board confirms and closes). */
  onDelete?: () => void
  onError: (msg: string) => void
}

// ScheduleModal is the create/edit popup for cron schedules (the first board
// column). It replaces the old always-visible inline form + inline edit row.
export function ScheduleModal({
  agents,
  flows,
  editing,
  onClose,
  onSaved,
  onDelete,
  onError,
}: Props) {
  const [targetMode, setTargetMode] = useState<'agent' | 'flow'>(editing?.flowId ? 'flow' : 'agent')
  const [name, setName] = useState(editing?.name ?? '')
  const [agentId, setAgentId] = useState(editing?.agentId ?? '')
  const [flowId, setFlowId] = useState(editing?.flowId ?? '')
  const [cronExpr, setCronExpr] = useState(editing?.cronExpr ?? '*/5 * * * *')
  const [prompt, setPrompt] = useState(editing?.prompt ?? '')
  // An empty stored mode means the backend default (reuse), so show that.
  const [sessionMode, setSessionMode] = useState<ScheduleSessionMode>(
    editing?.sessionMode === 'spawn' ? 'spawn' : 'reuse',
  )
  const [expiresAt, setExpiresAt] = useState(unixToLocalInput(editing?.expiresAt))
  const [generatingTitle, setGeneratingTitle] = useState(false)

  // A flow input is optional; an agent needs a prompt. Every schedule needs a
  // target and a cron expression. Record order is the blocking priority.
  const { markAttempted, firstError, errorFor } = useFieldErrors({
    cron: !cronExpr.trim() ? 'Cron ifadesi zorunlu' : '',
    target:
      targetMode === 'flow' ? (!flowId ? 'Akış seçilmeli' : '') : !agentId ? 'Ajan zorunlu' : '',
    prompt: targetMode === 'agent' && !prompt.trim() ? 'Prompt zorunlu' : '',
  })

  const submit = async () => {
    markAttempted()
    if (firstError) {
      onError(firstError)
      return
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    const target = targetMode === 'flow' ? { flowId, agentId: '' } : { agentId, flowId: '' }
    try {
      if (editing) {
        const updated = await api.updateSchedule(editing.id, {
          // Sent even when empty: `|| undefined` would drop the key and make
          // clearing the name a silent no-op on a full-object PUT.
          name: name.trim(),
          ...target,
          cronExpr: cronExpr.trim(),
          prompt: prompt.trim(),
          sessionMode,
          expiresAt: expUnix,
        })
        onSaved(updated, false)
      } else {
        const created = await api.createSchedule({
          name: name.trim() || undefined,
          ...(targetMode === 'flow' ? { flowId } : { agentId }),
          cronExpr: cronExpr.trim(),
          prompt: prompt.trim(),
          sessionMode,
          enabled: true,
          expiresAt: expUnix,
        })
        onSaved(created, true)
      }
      toast.success(editing ? 'Zamanlama güncellendi' : 'Zamanlama oluşturuldu')
      onClose()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  return (
    <FormModal
      title={editing ? 'Zamanlamayı düzenle' : 'Yeni zamanlama'}
      icon={Clock}
      accent={COLUMN_ACCENT.schedules}
      submitLabel={editing ? 'Kaydet' : '+ Zamanlama'}
      onSubmit={submit}
      onClose={onClose}
      onDelete={editing ? onDelete : undefined}
      deleteTestId="schedule-delete"
      testId={editing ? 'schedule-edit-modal' : 'schedule-create-modal'}
    >
      <Field label="Ad (opsiyonel)" hint="Zamanlamaya bir isim ver — kartta ve listede gösterilir.">
        <div className="flex items-center gap-2">
          <input
            data-testid="schedule-create-name-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="örn: günlük rapor"
            className={`w-full ${inputCls}`}
          />
          {editing && (
            <button
              type="button"
              disabled={generatingTitle}
              onClick={async () => {
                setGeneratingTitle(true)
                try {
                  // Suggestion only — it lands in the form and is persisted by Save,
                  // so Cancel still discards it.
                  const { title } = await api.generateScheduleTitle(editing.id)
                  setName(title)
                  toast.success('Başlık önerildi — kaydetmeyi unutma')
                } catch (e) {
                  onError((e as Error).message)
                } finally {
                  setGeneratingTitle(false)
                }
              }}
              className="flex shrink-0 items-center gap-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              title="AI ile başlık oluştur"
            >
              <Sparkles size={14} className={generatingTitle ? 'animate-pulse' : ''} />
              {generatingTitle ? '...' : 'Oluştur'}
            </button>
          )}
        </div>
      </Field>

      <Field label="Hedef" hint="Zamanlama bir ajana prompt gönderir ya da bir akış çalıştırır.">
        <div className="flex flex-wrap items-center gap-2">
          <TargetModeToggle mode={targetMode} onChange={setTargetMode} />
          {targetMode === 'flow' ? (
            <div data-testid="schedule-create-flow-wrap">
              <FlowPicker flows={flows} value={flowId} onChange={setFlowId} />
            </div>
          ) : (
            <div data-testid="schedule-create-agent-wrap">
              <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />
            </div>
          )}
        </div>
        <FieldError message={errorFor('target')} />
      </Field>

      {/* Session mode applies only to an agent target: a flow-backed schedule
          always records into its own per-run transcript. */}
      {targetMode === 'agent' && (
        <Field
          label="Oturum"
          hint="Her çalışma aynı zamanlama sohbetine mi eklensin, yoksa kendi oturumunu mu açsın?"
        >
          <select
            data-testid="schedule-create-session-mode-select"
            value={sessionMode}
            onChange={(e) => setSessionMode(e.target.value as ScheduleSessionMode)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
          >
            <option value="reuse">Aynı oturumu sürdür (varsayılan)</option>
            <option value="spawn">Her çalışmada yeni oturum aç</option>
          </select>
        </Field>
      )}

      <Field label="Cron ifadesi" hint="Hazır ifadeyi seç ya da elle yaz.">
        <div className="flex flex-wrap items-center gap-2">
          <select
            data-testid="schedule-create-cron-preset-select"
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
          >
            <option value={cronExpr}>Hazır ifade seç…</option>
            {PRESET_GROUPS.map((g) => (
              <optgroup key={g.group} label={g.group}>
                {g.items.map((p) => (
                  <option key={p.expr} value={p.expr}>
                    {p.label} ({p.expr})
                  </option>
                ))}
              </optgroup>
            ))}
          </select>
          <input
            data-testid={editing ? 'schedule-edit-cron-input' : 'schedule-create-cron-input'}
            data-schedule-id={editing?.id}
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            placeholder="cron: dk sa gün ay haftagünü"
            className={`w-52 rounded border bg-[var(--color-bg)] px-2 py-1.5 font-mono text-sm outline-none focus:border-[var(--color-accent)] ${errorFor('cron') ? 'border-[var(--color-danger)]' : 'border-[var(--color-border)]'}`}
          />
        </div>
        <FieldError message={errorFor('cron')} />
      </Field>

      <Field label={targetMode === 'flow' ? 'Akış girdisi (opsiyonel)' : 'Prompt (zorunlu)'}>
        <textarea
          data-testid={editing ? 'schedule-edit-prompt-input' : 'schedule-create-prompt-input'}
          data-schedule-id={editing?.id}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          rows={4}
          placeholder={
            targetMode === 'flow' ? 'Akış girdisi (opsiyonel)' : 'Ajana gönderilecek talimat'
          }
          className={`${inputCls} resize-y ${errorFor('prompt') ? 'border-[var(--color-danger)]' : ''}`}
        />
        <FieldError message={errorFor('prompt')} />
      </Field>

      <Field label="Son tarih (opsiyonel)" hint="Bu tarihten sonra zamanlama çalışmaz.">
        <div className="flex items-center gap-2">
          <input
            data-testid="schedule-create-expires-input"
            type="datetime-local"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          {expiresAt && (
            <button
              type="button"
              onClick={() => setExpiresAt('')}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              title="Son tarihi temizle"
            >
              <X size={14} />
            </button>
          )}
        </div>
      </Field>
    </FormModal>
  )
}
