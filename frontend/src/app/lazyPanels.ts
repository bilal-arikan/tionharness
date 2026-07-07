// Lazy-loaded panel chunks, kept in their own module (components only) so the
// react-refresh only-export-components rule stays happy in viewRegistry.
import { lazy } from 'react'

// Code-split: the Flows panel pulls in React Flow (~300KB), loaded only when
// the user opens the Akışlar view.
export const FlowsPanel = lazy(() =>
  import('@/features/flows/FlowsPanel').then((m) => ({ default: m.FlowsPanel })),
)
// The collaboration network panel also pulls in React Flow — load it on demand.
export const NetworkPanel = lazy(() =>
  import('@/features/network/NetworkPanel').then((m) => ({ default: m.NetworkPanel })),
)
