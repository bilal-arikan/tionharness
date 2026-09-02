// View registry: per-view metadata (titles, header layout) and small shared
// view predicates. Keeping these here (not in App.tsx) lets the shell stay a
// thin composition layer while every view-list concern lives in one place.
// The lazy panel chunks live in lazyPanels.ts (components-only module).
import type { View } from './NavRail'

// Minimum time the first-run splash stays on screen (ms), so an instant workspace
// load doesn't flash the logo for a single frame. Only applies on a fresh install.
export const SPLASH_MIN_MS = 1100

export const VIEW_TITLE: Record<View, string> = {
  dashboard: 'Panel',
  chat: 'Sohbet',
  agents: 'Ajanlar',
  network: 'Ağ',
  rota: 'Rota',
  explorer: 'Harita',
  board: 'Görevler',
  schedules: 'Otomasyon',
  flows: 'Akışlar',
  artifacts: 'Artifactlar',
  skills: 'Skills',
  tools: 'Araçlar & MCP',
  budget: 'Bütçe',
  prompts: 'navigation.promptsFiles',
  insights: 'İçgörü',
  market: 'Market',
  workspace: 'Workspace',
  settings: 'Ayarlar',
}

// Views that render their own left list-sidebar INSIDE the main area. For these we
// skip the app-level top header entirely so the sidebar (and the panel's own
// in-pane headers) reach the very top — matching the chat layout where the
// sidebar is a sibling of <main>. Errors for these still surface via the Toaster.
export const HEADERLESS_VIEWS = new Set<View>([
  'agents',
  'artifacts',
  'skills',
  'tools',
  'flows',
  'market',
  'schedules',
  'prompts',
  'insights',
  'budget',
  'board',
  'network',
  'rota',
  'explorer',
  'dashboard',
])

// The composer gate lives in shared/lib so the session panels can share it with
// the shell; re-exported here for the existing app-layer call sites.
export { isWritableSessionKind } from '@/shared/lib/sessionKind'
