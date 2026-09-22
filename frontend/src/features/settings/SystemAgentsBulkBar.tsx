import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pencil } from 'lucide-react'
import { api } from '@/api'
import type { Agent } from '@/types'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton } from '@/shared/components'
import { AgentBulkEditPanel } from '@/features/agents/AgentBulkEditPanel'

interface Props {
  sel: MultiSelect
  /** System agents in roster render order. */
  agents: Agent[]
  /** Ids of `agents`, same order: the select-all and Shift-range order. */
  orderedIds: string[]
  /** Re-fetch the roster once a bulk write settles, whether it succeeded or not. */
  onSettled: () => void
  onError: (msg: string) => void
}

// SystemAgentsBulkBar is the multi-select strip under the Settings ▸ System
// agents roster: the same provider-instance + model bulk edit the Agents screen
// offers for the user's own agents, scoped to system agents. Each row is written
// through the ordinary per-agent update, so a built-in lands in the
// installation-wide override layer and a customisation pins its own override,
// exactly as a single edit from the form would.
export function SystemAgentsBulkBar({ sel, agents, orderedIds, onSettled, onError }: Props) {
  const { selected, replace } = sel

  // A row can leave the roster while selected (a customisation deleted from the
  // form); drop its id so the bar's count matches what an edit would patch.
  useEffect(() => {
    const live = [...selected].filter((id) => orderedIds.includes(id))
    if (live.length !== selected.size) replace(live)
  }, [orderedIds, selected, replace])

  // Mounted only while something is selected, so clearing the selection by any
  // route (Esc, a plain row click, the bar's X) also closes the edit panel.
  if (selected.size === 0) return null
  return (
    <BulkActions
      sel={sel}
      selectedAgents={agents.filter((agent) => selected.has(agent.id))}
      orderedIds={orderedIds}
      onSettled={onSettled}
      onError={onError}
    />
  )
}

type BulkActionsProps = Omit<Props, 'agents'> & { selectedAgents: Agent[] }

function BulkActions({ sel, selectedAgents, orderedIds, onSettled, onError }: BulkActionsProps) {
  const { t } = useTranslation('common')
  const [editOpen, setEditOpen] = useState(false)

  return (
    <>
      {editOpen && selectedAgents.length > 0 && (
        <AgentBulkEditPanel
          scope="system"
          agents={selectedAgents}
          onUpdateAgent={api.updateAgent}
          onApplied={() => {
            sel.clear()
            onSettled()
          }}
          onCancel={() => setEditOpen(false)}
          onError={(msg) => {
            // A partial failure still wrote the other rows; refresh so the
            // roster shows what did change. Selection and panel stay for a retry.
            onSettled()
            onError(msg)
          }}
        />
      )}
      <SelectionBar
        count={sel.count}
        onClear={sel.clear}
        onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
      >
        <SelectionBarButton icon={<Pencil size={13} />} onClick={() => setEditOpen(true)}>
          {t('agents.bulkEdit.button')}
        </SelectionBarButton>
      </SelectionBar>
    </>
  )
}
