import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { agentApi } from '@/api/agents'
import { workspaceApi } from '@/api/workspaces'
import { SIGNAL_AGENTS } from '@/app/eventToRefreshSignals'
import { useAsync } from '@/shared/hooks/useAsync'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { InfoPopover } from '../InfoPopover'
import { toast } from '../toastStore'
import { AgentPicker } from './AgentPicker'
import { builtinRoleAgent, isWorkerRole, roleCandidates } from './systemRoles'

interface PickersProps {
  roles: readonly string[]
  assignments: Record<string, string>
  onChange: (role: string, agentId: string) => void
}

// SystemRolePickers renders one "which agent runs this role" row per system
// role — the Insight screen's analysis-agent picker, generalised. Clearing a row
// returns the role to its built-in agent, which the placeholder names.
export function SystemRolePickers({ roles, assignments, onChange }: PickersProps) {
  const { t } = useTranslation('sharedUi')
  const agentsTick = useRefreshTrigger(SIGNAL_AGENTS)
  const { data: agents } = useAsync(() => agentApi.listAgents(), [agentsTick])
  const all = agents ?? []
  return (
    <div className="flex flex-col gap-2" data-testid="system-role-pickers">
      {roles.map((role) => {
        const value = assignments[role] ?? ''
        const builtin = builtinRoleAgent(all, role)
        return (
          <div
            key={role}
            data-testid={`system-role-${role}`}
            className="grid grid-cols-1 items-center gap-1 sm:grid-cols-[minmax(0,1fr)_minmax(0,17rem)] sm:gap-3"
          >
            <span className="flex min-w-0 items-center gap-1 text-sm">
              <span className="truncate">{t(`systemRoles.roles.${role}.label`)}</span>
              <InfoPopover
                mode="icon"
                text={`${t(`systemRoles.roles.${role}.hint`)} ${t(
                  isWorkerRole(role)
                    ? 'systemRoles.workerSemantics'
                    : 'systemRoles.utilitySemantics',
                )}`}
              />
            </span>
            <div className="min-w-0 [&>div]:w-full [&_[data-testid=agent-picker-trigger]]:w-full [&_[data-testid=agent-picker-trigger]]:min-w-0">
              <AgentPicker
                agents={roleCandidates(all, role, value)}
                value={value}
                onChange={(id) => onChange(role, id)}
                placeholder={
                  builtin
                    ? t('systemRoles.builtin', { name: builtin.name })
                    : t('systemRoles.builtinUnnamed')
                }
                clearable
              />
            </div>
          </div>
        )
      })}
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
// (Workspace ▸ Agents, Insight settings, Flows ▸ Evolution): each pick is persisted
// at once as a one-role merge patch, so screens never overwrite each other's
// roles.
export function SystemRoleAssignments({ roles, title, hint, sections }: CardProps) {
  const { t } = useTranslation('sharedUi')
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
  const change = async (role: string, agentId: string) => {
    const prev = assignments
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
