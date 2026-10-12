import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Agent } from '@/types'
import { agentApi } from '@/api/agents'
import { workspaceApi } from '@/api/workspaces'
import { SIGNAL_AGENTS } from '@/app/eventToRefreshSignals'
import { useAsync } from '@/shared/hooks/useAsync'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { InfoPopover } from '../InfoPopover'
import { toast } from '../toastStore'
import { AgentPicker } from './AgentPicker'
import { builtinRoleAgent, roleCandidates } from './systemRoles'

// useRoleAgents loads the workspace roster; roleCandidates keeps only each
// role's own system agents.
function useRoleAgents(): Agent[] {
  const agentsTick = useRefreshTrigger(SIGNAL_AGENTS)
  const { data } = useAsync(() => agentApi.listAgents(), [agentsTick])
  return data ?? []
}

// useRoleAssignments loads this workspace's role assignments and saves each pick
// at once as a one-role merge patch, so screens never overwrite each other.
function useRoleAssignments() {
  const [assignments, setAssignments] = useState<Record<string, string> | null>(null)
  const [assignable, setAssignable] = useState<string[] | null>(null)
  useEffect(() => {
    let alive = true
    workspaceApi
      .getWorkspaceSettings()
      .then((s) => {
        if (!alive) return
        setAssignments(s.systemAgentAssignments ?? {})
        setAssignable(s.assignableRoles ?? null)
      })
      .catch((e: Error) => toast.error(e.message))
    return () => {
      alive = false
    }
  }, [])
  const change = async (role: string, agentId: string) => {
    const prev = assignments ?? {}
    setAssignments({ ...prev, [role]: agentId })
    try {
      const saved = await workspaceApi.updateWorkspaceSettings({
        systemAgentAssignments: { [role]: agentId },
      })
      setAssignments(saved.systemAgentAssignments ?? {})
    } catch (e) {
      setAssignments(prev)
      toast.error((e as Error).message)
    }
  }
  return { assignments, assignable, change }
}

interface RolePickerProps {
  role: string
  agents: Agent[]
  value: string
  onChange: (agentId: string) => void
}

// RoleAgentPicker offers only the role's own system agents (built-in and its
// customisations). With nothing assigned the trigger still shows the agent that
// runs the role — avatar, name, model — marked "built-in".
function RoleAgentPicker({ role, agents, value, onChange }: RolePickerProps) {
  const { t } = useTranslation('sharedUi')
  return (
    <div className="min-w-0 [&>div]:w-full [&_[data-testid=agent-picker-trigger]]:w-full [&_[data-testid=agent-picker-trigger]]:min-w-0">
      <AgentPicker
        agents={roleCandidates(agents, role, value)}
        value={value}
        onChange={onChange}
        placeholder={t('systemRoles.builtinUnnamed')}
        defaultAgent={builtinRoleAgent(agents, role)}
        defaultLabel={t('systemRoles.builtinBadge')}
        clearable
      />
    </div>
  )
}

// SystemRoleSelect is a single self-saving role picker without a label, for a
// screen that names the role itself (the Insight rail's analysis agent).
export function SystemRoleSelect({ role }: { role: string }) {
  const agents = useRoleAgents()
  const { assignments, change } = useRoleAssignments()
  if (!assignments) return null
  return (
    <RoleAgentPicker
      role={role}
      agents={agents}
      value={assignments[role] ?? ''}
      onChange={(id) => change(role, id)}
    />
  )
}

interface PickersProps {
  roles: readonly string[]
  assignments: Record<string, string>
  onChange: (role: string, agentId: string) => void
}

// SystemRolePickers renders one labelled "which system agent runs this role"
// row per role.
export function SystemRolePickers({ roles, assignments, onChange }: PickersProps) {
  const { t } = useTranslation('sharedUi')
  const agents = useRoleAgents()
  return (
    <div className="flex flex-col gap-2" data-testid="system-role-pickers">
      {roles.map((role) => (
        <div
          key={role}
          data-testid={`system-role-${role}`}
          className="grid grid-cols-1 items-center gap-1 sm:grid-cols-[minmax(0,1fr)_minmax(0,17rem)] sm:gap-3"
        >
          <span className="flex min-w-0 items-center gap-1 text-sm">
            <span className="truncate">{t(`systemRoles.roles.${role}.label`)}</span>
            <InfoPopover mode="icon" text={t(`systemRoles.roles.${role}.hint`)} />
          </span>
          <RoleAgentPicker
            role={role}
            agents={agents}
            value={assignments[role] ?? ''}
            onChange={(id) => onChange(role, id)}
          />
        </div>
      ))}
    </div>
  )
}

export interface SystemRoleSection {
  key: string
  title: string
  hint?: string
  roles: readonly string[]
}

interface CardProps {
  // Either one untitled list of roles (a feature screen's single section) …
  roles?: readonly string[]
  title?: string
  hint?: string
  // … or several titled sections sharing one settings load (Workspace ▸ Agents).
  sections?: SystemRoleSection[]
}

// SystemRoleAssignments is the self-saving form used inside feature screens
// (Workspace ▸ Agents, Insight settings, Flows ▸ Evolution).
export function SystemRoleAssignments({ roles, title, hint, sections }: CardProps) {
  const { t } = useTranslation('sharedUi')
  const { assignments, assignable, change } = useRoleAssignments()
  if (!assignments) return null
  const list: SystemRoleSection[] = sections ?? [
    {
      key: 'roles',
      title: title ?? t('systemRoles.title'),
      hint: hint ?? t('systemRoles.hint'),
      roles: roles ?? [],
    },
  ]
  const visible = list
    .map((sec) => ({
      ...sec,
      roles: assignable ? sec.roles.filter((r) => assignable.includes(r)) : sec.roles,
    }))
    .filter((sec) => sec.roles.length > 0)
  if (visible.length === 0) return null
  return (
    <div className="flex flex-col gap-5" data-testid="system-role-assignments">
      {visible.map((sec) => (
        <div key={sec.key} className="flex flex-col gap-2">
          <span className="flex items-center gap-1 text-sm font-medium">
            {sec.title}
            {sec.hint && <InfoPopover mode="icon" text={sec.hint} />}
          </span>
          <SystemRolePickers roles={sec.roles} assignments={assignments} onChange={change} />
        </div>
      ))}
    </div>
  )
}
