// Lazy-loaded panel chunks, kept in their own module (components only) so the
// react-refresh only-export-components rule stays happy in viewRegistry.
import { lazy } from 'react'

// Code-split: the Flows panel pulls in React Flow (~300KB), loaded only when
// the user opens the Akışlar view.
export const FlowsPanel = lazy(() =>
  import('@/features/flows/FlowsPanel').then((m) => ({ default: m.FlowsPanel })),
)
// The collaboration network panel also pulls in React Flow — load it on demand.
// The Rota (trajectory) view is its own chunk so the lane store + panels only
// load when the user opens it (_Docs/77 R10).
export const RotaPanel = lazy(() =>
  import('@/features/rota/RotaPanel').then((m) => ({ default: m.RotaPanel })),
)
// The Explorer (Harita) drill-down map is React Flow too — load on demand.
export const ExplorerView = lazy(() =>
  import('@/features/explorer/ExplorerView').then((m) => ({ default: m.ExplorerView })),
)

export const AgentsView = lazy(() =>
  import('@/features/agents/AgentsView').then((m) => ({ default: m.AgentsView })),
)

export const TaskBoard = lazy(() =>
  import('@/features/tasks/TaskBoard').then((m) => ({ default: m.TaskBoard })),
)

export const AutomationBoard = lazy(() =>
  import('@/features/schedules/AutomationBoard').then((m) => ({ default: m.AutomationBoard })),
)

export const ArtifactsPanel = lazy(() =>
  import('@/features/artifacts/ArtifactsPanel').then((m) => ({ default: m.ArtifactsPanel })),
)

export const SkillsPanel = lazy(() =>
  import('@/features/skills/SkillsPanel').then((m) => ({ default: m.SkillsPanel })),
)

export const ToolCatalogPanel = lazy(() =>
  import('@/features/tools/ToolsPanel').then((m) => ({ default: m.ToolsPanel })),
)

export const MarketPanel = lazy(() =>
  import('@/features/market/MarketPanel').then((m) => ({ default: m.MarketPanel })),
)

export const BudgetPanel = lazy(() =>
  import('@/features/budget/BudgetPanel').then((m) => ({ default: m.BudgetPanel })),
)

export const DashboardPanel = lazy(() =>
  import('@/features/dashboard/DashboardPanel').then((m) => ({ default: m.DashboardPanel })),
)

export const InsightPanel = lazy(() =>
  import('@/features/insight/InsightPanel').then((m) => ({ default: m.InsightPanel })),
)

// Workspace memory + awareness (notes, digests, what-the-agent-saw).
export const NotesPanel = lazy(() =>
  import('@/features/notes/NotesPanel').then((m) => ({ default: m.NotesPanel })),
)

export const SettingsPanel = lazy(() =>
  import('@/features/settings/SettingsPanel').then((m) => ({ default: m.SettingsPanel })),
)

export const WorkspaceView = lazy(() =>
  import('@/features/workspace/WorkspaceView').then((m) => ({ default: m.WorkspaceView })),
)

export const PromptsView = lazy(() =>
  import('@/features/settings/PromptsView').then((m) => ({ default: m.PromptsView })),
)
