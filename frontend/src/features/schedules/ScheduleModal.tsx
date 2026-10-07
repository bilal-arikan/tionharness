import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Clock, Sparkles, X } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Schedule, ScheduleSessionMode } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { toast } from '@/shared/components'
import { COLUMN_ACCENT } from './automationMeta'
import { PRESET_GROUPS } from './cronPresets'
import { FormModal } from './FormModal'
import { Field, inputCls } from './pickers'
import { fmtTime, localInputToUnix, unixToLocalInput } from './timeUtils'
import { FieldError } from './FieldError'
import { useFieldErrors } from './useFieldErrors'

interface Props {
  agents: Agent[]
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
export function ScheduleModal({ agents, editing, onClose, onSaved, onDelete, onError }: Props) {
  const { t } = useTranslation('schedules')
  const [name, setName] = useState(editing?.name ?? '')
  const [agentId, setAgentId] = useState(editing?.agentId ?? '')
  const [cronExpr, setCronExpr] = useState(editing?.cronExpr ?? '*/5 * * * *')
  const [prompt, setPrompt] = useState(editing?.prompt ?? '')
  // An empty stored mode means the backend default (reuse), so show that.
  const [sessionMode, setSessionMode] = useState<ScheduleSessionMode>(
    editing?.sessionMode === 'spawn' ? 'spawn' : 'reuse',
  )
  const [expiresAt, setExpiresAt] = useState(unixToLocalInput(editing?.expiresAt))
  const [generatingTitle, setGeneratingTitle] = useState(false)

  // Every schedule needs a target agent, a prompt and a cron expression. Record
  // order is the blocking priority.
  const { markAttempted, firstError, errorFor } = useFieldErrors({
    cron: !cronExpr.trim() ? t('validation.cronRequired') : '',
    target: !agentId ? t('validation.agentRequired') : '',
    prompt: !prompt.trim() ? t('validation.promptRequired') : '',
  })

  // A one-shot wake (schedule_wake) is not an editable routine: it carries no cron
  // and the scheduler owns its fire time. Such a row only reaches this modal after
  // its delivery failed, so the single supported action here is deleting it — the
  // form is replaced by a read-only summary rather than shown with dead fields.
  if (editing?.oneShot) {
    return (
      <FormModal
        title={t('scheduleModal.oneShotTitle')}
        info={t('scheduleModal.oneShotDescription')}
        icon={Clock}
        accent={COLUMN_ACCENT.schedules}
        submitLabel={t('common.close')}
        onSubmit={onClose}
        onClose={onClose}
        onDelete={onDelete}
        deleteTestId="schedule-delete"
        testId="schedule-edit-modal"
      >
        <Field label={t('scheduleModal.runTime')}>
          <div className="font-mono text-sm text-[var(--color-text)]">
            {fmtTime(editing.fireAt)}
          </div>
        </Field>
        <Field label={t('common.prompt')}>
          <div className="whitespace-pre-wrap text-sm text-[var(--color-text-dim)]">
            {editing.prompt}
          </div>
        </Field>
        {editing.lastDeliveryError ? (
          <Field label={t('common.error')}>
            <div className="whitespace-pre-wrap text-sm text-[var(--color-danger)]">
              {editing.lastDeliveryError}
            </div>
          </Field>
        ) : null}
      </FormModal>
    )
  }

  const submit = async () => {
    markAttempted()
    if (firstError) {
      onError(firstError)
      return
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError(t('validation.expiryFuture'))
      return
    }
    try {
      if (editing) {
        const updated = await api.updateSchedule(editing.id, {
          // Sent even when empty: `|| undefined` would drop the key and make
          // clearing the name a silent no-op on a full-object PUT.
          name: name.trim(),
          agentId,
          cronExpr: cronExpr.trim(),
          prompt: prompt.trim(),
          sessionMode,
          expiresAt: expUnix,
        })
        onSaved(updated, false)
      } else {
        const created = await api.createSchedule({
          name: name.trim() || undefined,
          agentId,
          cronExpr: cronExpr.trim(),
          prompt: prompt.trim(),
          sessionMode,
          enabled: true,
          expiresAt: expUnix,
        })
        onSaved(created, true)
      }
      toast.success(editing ? t('scheduleModal.updated') : t('scheduleModal.created'))
      onClose()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  return (
    <FormModal
      title={editing ? t('scheduleModal.editTitle') : t('scheduleModal.newTitle')}
      icon={Clock}
      accent={COLUMN_ACCENT.schedules}
      submitLabel={editing ? t('common.save') : t('scheduleModal.add')}
      onSubmit={submit}
      onClose={onClose}
      onDelete={editing ? onDelete : undefined}
      deleteTestId="schedule-delete"
      testId={editing ? 'schedule-edit-modal' : 'schedule-create-modal'}
    >
      <Field label={t('common.nameOptional')} hint={t('scheduleModal.nameHint')}>
        <div className="flex items-center gap-2">
          <input
            data-testid="schedule-create-name-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('scheduleModal.namePlaceholder')}
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
                  toast.success(t('common.titleSuggested'))
                } catch (e) {
                  onError((e as Error).message)
                } finally {
                  setGeneratingTitle(false)
                }
              }}
              className="flex shrink-0 items-center gap-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-xs text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              title={t('common.generateTitle')}
            >
              <Sparkles size={14} className={generatingTitle ? 'animate-pulse' : ''} />
              {generatingTitle ? '...' : t('common.generate')}
            </button>
          )}
        </div>
      </Field>

      <Field label={t('fields.target')} hint={t('scheduleModal.targetHint')}>
        <div className="flex flex-wrap items-center gap-2">
          <div data-testid="schedule-create-agent-wrap">
            <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />
          </div>
        </div>
        <FieldError message={errorFor('target')} />
      </Field>

      <Field label={t('fields.session')} hint={t('scheduleModal.sessionHint')}>
        <select
          data-testid="schedule-create-session-mode-select"
          value={sessionMode}
          onChange={(e) => setSessionMode(e.target.value as ScheduleSessionMode)}
          className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
        >
          <option value="reuse">{t('scheduleModal.sessionReuse')}</option>
          <option value="spawn">{t('scheduleModal.sessionSpawn')}</option>
        </select>
      </Field>

      <Field label={t('scheduleModal.cronExpression')} hint={t('scheduleModal.cronHint')}>
        <div className="flex flex-wrap items-center gap-2">
          <select
            data-testid="schedule-create-cron-preset-select"
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
          >
            <option value={cronExpr}>{t('scheduleModal.selectPreset')}</option>
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
            placeholder={t('scheduleModal.cronPlaceholder')}
            className={`w-52 rounded border bg-[var(--color-bg)] px-2 py-1.5 font-mono text-sm outline-none focus:border-[var(--color-accent)] ${errorFor('cron') ? 'border-[var(--color-danger)]' : 'border-[var(--color-border)]'}`}
          />
        </div>
        <FieldError message={errorFor('cron')} />
      </Field>

      <Field label={t('scheduleModal.promptRequired')}>
        <textarea
          data-testid={editing ? 'schedule-edit-prompt-input' : 'schedule-create-prompt-input'}
          data-schedule-id={editing?.id}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          rows={4}
          placeholder={t('scheduleModal.promptPlaceholder')}
          className={`${inputCls} resize-y ${errorFor('prompt') ? 'border-[var(--color-danger)]' : ''}`}
        />
        <FieldError message={errorFor('prompt')} />
      </Field>

      <Field label={t('common.expiresOptional')} hint={t('scheduleModal.expiryHint')}>
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
              title={t('common.clearExpiry')}
            >
              <X size={14} />
            </button>
          )}
        </div>
      </Field>
    </FormModal>
  )
}
