import { useTranslation } from 'react-i18next'
import { SystemRoleAssignments } from '@/shared/components/agents/SystemRolePickers'
import { SYSTEM_ROLE_GROUPS, type SystemRoleGroup } from '@/shared/components/agents/systemRoles'

const GROUPS: SystemRoleGroup[] = ['workers', 'helpers', 'analysis', 'optimizers']

// WorkspaceAgentsPanel is the workspace's full role roster: which agent runs each
// worker profile and each system helper here. Feature screens (Insight, Flows)
// show the same pickers for just the roles they own.
export function WorkspaceAgentsPanel() {
  const { t } = useTranslation('workspace')
  const { t: tr } = useTranslation('sharedUi')
  return (
    <>
      <p className="text-sm text-[var(--color-text-dim)]">{t('agents.intro')}</p>
      <SystemRoleAssignments
        sections={GROUPS.map((g) => ({
          key: g,
          title: tr(`systemRoles.groups.${g}`),
          hint: t(`agents.groupHints.${g}`),
          roles: SYSTEM_ROLE_GROUPS[g],
        }))}
      />
    </>
  )
}
