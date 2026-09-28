import { SettingsDisclosure } from './SettingsDisclosure'
import type { PanelProps } from './settingsPanelShared'
import { DelegationLimits } from './DelegationLimits'
import { BackgroundLimits } from './BackgroundLimits'
import { ToolExecutionLimits } from './ToolExecutionLimits'
import { CoordinatorLimits } from './CoordinatorLimits'
import { CoordinatorRecovery } from './CoordinatorRecovery'
import { AutonomousWork } from './AutonomousWork'

export function ExecutionPanel(props: PanelProps) {
  return (
    <>
      <SettingsDisclosure title="Delegation">
        <DelegationLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Background runs and idle limits">
        <BackgroundLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Tool execution limits">
        <ToolExecutionLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Coordinator limits">
        <CoordinatorLimits {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Coordinator recovery">
        <CoordinatorRecovery {...props} />
      </SettingsDisclosure>
      <SettingsDisclosure title="Autonomous work">
        <AutonomousWork {...props} />
      </SettingsDisclosure>
    </>
  )
}
