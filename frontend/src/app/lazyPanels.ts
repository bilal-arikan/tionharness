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
