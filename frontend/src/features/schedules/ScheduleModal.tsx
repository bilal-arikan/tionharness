import { useState } from 'react'
import { Clock, X } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Flow, Schedule } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { COLUMN_ACCENT } from './automationMeta'
import { PRESET_GROUPS } from './cronPresets'
import { FormModal } from './FormModal'
import { Field, FlowPicker, TargetModeToggle, inputCls } from './pickers'
import { localInputToUnix, unixToLocalInput } from './timeUtils'

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
export function ScheduleModal({ agents, flows, editing, onClose, onSaved, onDelete, onError }: Props) {
  const [targetMode, setTargetMode] = useState<'agent' | 'flow'>(editing?.flowId ? 'flow' : 'agent')
  const [agentId, setAgentId] = useState(editing?.agentId ?? '')
  const [flowId, setFlowId] = useState(editing?.flowId ?? '')
  const [cronExpr, setCronExpr] = useState(editing?.cronExpr ?? '*/5 * * * *')
  const [prompt, setPrompt] = useState(editing?.prompt ?? '')
  const [expiresAt, setExpiresAt] = useState(unixToLocalInput(editing?.expiresAt))

  const submit = async () => {
    if (!cronExpr.trim()) {
      onError('Cron ifadesi zorunlu')
      return
    }
    if (targetMode === 'flow') {
      if (!flowId) {
        onError('Akış seçilmeli')
        return
      }
    } else {
      if (!agentId) {
        onError('Ajan zorunlu')
        return
      }
      if (!prompt.trim()) {
        onError('Prompt zorunlu')
        return
      }
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
          ...target,
          cronExpr: cronExpr.trim(),
          prompt: prompt.trim(),
          expiresAt: expUnix,
        })
        onSaved(updated, false)
      } else {
        const created = await api.createSchedule({
          ...(targetMode === 'flow' ? { flowId } : { agentId }),
          cronExpr: cronExpr.trim(),
          prompt: prompt.trim(),
          enabled: true,
          expiresAt: expUnix,
        })
        onSaved(created, true)
      }
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
      </Field>

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
            className="w-52 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </div>
      </Field>

      <Field label={targetMode === 'flow' ? 'Akış girdisi (opsiyonel)' : 'Prompt (zorunlu)'}>
        <textarea
          data-testid={editing ? 'schedule-edit-prompt-input' : 'schedule-create-prompt-input'}
          data-schedule-id={editing?.id}
          value={prompt}
          onChange={(e) => setPrompt(e.target.value)}
          rows={4}
          placeholder={targetMode === 'flow' ? 'Akış girdisi (opsiyonel)' : 'Ajana gönderilecek talimat'}
          className={`${inputCls} resize-y`}
        />
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
