import { useTranslation } from 'react-i18next'
import { SettingsDisclosure } from './SettingsDisclosure'
import type { PanelProps } from './settingsPanelShared'
import { DelegationLimits } from './DelegationLimits'
import { BackgroundLimits } from './BackgroundLimits'
import { ToolExecutionLimits } from './ToolExecutionLimits'
import { CoordinatorLimits } from './CoordinatorLimits'
import { CoordinatorRecovery } from './CoordinatorRecovery'
import { AutonomousWork } from './AutonomousWork'

export function ExecutionPanel(props: PanelProps) {
  const { t } = useTranslation('settingsMain')
  return (
    <>
      <SettingsDisclosure title={t('execution.delegation')}>
        <DelegationLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('execution.background')}>
        <BackgroundLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('execution.toolLimits')}>
        <ToolExecutionLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('execution.coordinatorLimits')}>
        <CoordinatorLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('execution.coordinatorRecovery')}>
        <CoordinatorRecovery {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title={t('execution.autonomous')}>
        <AutonomousWork {...props} />
      </SettingsDisclosure>
    </>
  )
}
