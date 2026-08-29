import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Agent, AgentPatch } from '@/types'
import { ProviderInstanceModelSelect } from '@/shared/components/agents/ProviderInstanceModelSelect'
import { Button } from '@/shared/components'
import { updateAgentProviderModels } from './agentBulkEdit'

interface Props {
  agents: Agent[]
  onUpdateAgent: (id: string, patch: AgentPatch) => Promise<unknown>
  onApplied: () => void
  onCancel: () => void
  onError?: (message: string) => void
}

export function AgentBulkEditPanel({ agents, onUpdateAgent, onApplied, onCancel, onError }: Props) {
  const { t } = useTranslation('common')
  const first = agents.find((agent) => !agent.system)
  const [providerInstanceId, setProviderInstanceId] = useState(
    first?.providerInstanceId ?? first?.provider ?? '',
  )
  const [model, setModel] = useState(first?.model ?? '')
  const [saving, setSaving] = useState(false)

  const apply = async () => {
    setSaving(true)
    try {
      await updateAgentProviderModels(agents, providerInstanceId, model, onUpdateAgent)
      onApplied()
    } catch (error) {
      const failure = error instanceof Error ? error : new Error(t('agents.bulkEdit.error'))
      if (!onError) throw failure
      onError(failure.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      data-testid="agent-bulk-edit-panel"
      className="mx-2 mb-2 space-y-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-3"
    >
      <div>
        <h3 className="text-sm font-semibold text-[var(--color-text)]">
          {t('agents.bulkEdit.title')}
        </h3>
        <p className="text-xs text-[var(--color-text-dim)]">
          {t('agents.bulkEdit.description', {
            count: agents.filter((agent) => !agent.system).length,
          })}
        </p>
      </div>
      <ProviderInstanceModelSelect
        providerInstanceId={providerInstanceId}
        model={model}
        onChange={(_kindId, instanceId, nextModel) => {
          setProviderInstanceId(instanceId)
          setModel(nextModel)
        }}
      />
      <div className="flex justify-end gap-2">
        <Button variant="secondary" onClick={onCancel} disabled={saving}>
          {t('agents.bulkEdit.cancel')}
        </Button>
        <Button
          data-testid="agent-bulk-edit-apply"
          onClick={apply}
          disabled={saving || !providerInstanceId}
        >
          {saving ? t('agents.bulkEdit.saving') : t('agents.bulkEdit.apply')}
        </Button>
      </div>
    </div>
  )
}
