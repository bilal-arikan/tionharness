// Barrel for the app-global setting category panels. Each panel now lives in its
// own file under settings/; this re-exports them so existing imports
// (`import { ProfilePanel, … } from './settings/appPanels'`) keep working. The
// simple stateless categories are pure draft+setter forms; the stateful ones
// (providers, commands, step kinds, workspace) live in their own files too.

export { ProfilePanel } from './ProfilePanel'
export { NotificationsPanel } from './NotificationsPanel'
export { SoundPanel } from './SoundPanel'
export { AppearancePanel } from './AppearancePanel'
export { ContextPanel } from './ContextPanel'
export { AutoTitlePanel } from './AutonomyPanel'
export { ToolsPanel } from './AppToolsPanel'
export { BackupPanel } from './BackupPanel'
export { AboutPanel } from './AboutPanel'
