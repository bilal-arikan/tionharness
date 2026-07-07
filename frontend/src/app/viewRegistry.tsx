// View registry: per-view metadata (titles, header layout) and small shared
// view predicates. Keeping these here (not in App.tsx) lets the shell stay a
// thin composition layer while every view-list concern lives in one place.
// The lazy panel chunks live in lazyPanels.ts (components-only module).
import type { View } from './NavRail'

// Minimum time the first-run splash stays on screen (ms), so an instant workspace
// load doesn't flash the logo for a single frame. Only applies on a fresh install.
export const SPLASH_MIN_MS = 1100

export const VIEW_TITLE: Record<View, string> = {
  chat: 'Sohbet',
  executions: 'Aktivite',
  agents: 'Ajanlar',
  network: 'Ağ',
  board: 'Görevler',
  schedules: 'Otomasyon',
  flows: 'Akışlar',
  artifacts: 'Artifactlar',
  skills: 'Skills',
  tools: 'Araçlar & MCP',
  budget: 'Bütçe',
  logs: 'Loglar',
  market: 'Market',
  workspace: 'Workspace',
  settings: 'Ayarlar',
}

// Views that render their own left list-sidebar INSIDE the main area. For these we
// skip the app-level top header entirely so the sidebar (and the panel's own
// in-pane headers) reach the very top — matching the chat layout where the
// sidebar is a sibling of <main>. Errors for these still surface via ErrorToast.
export const HEADERLESS_VIEWS = new Set<View>([
  'agents', 'executions', 'artifacts', 'skills', 'tools', 'flows', 'market', 'schedules', 'logs', 'budget', 'board', 'network',
])

// isChatKind reports whether a session is continuable from the chat sidebar.
// Manual chats (chat/empty) plus "spawned" sessions qualify: spawned covers
// both the spawn tool and handoff (context-reset) children, which are
// single-agent linear transcripts explicitly meant for a human to take over and
// keep talking to. Task/flow/schedule transcripts are aggregate/multi-run logs
// and stay read-only in the Activity (executions) view instead.
export function isChatKind(kind: string): boolean {
  return kind === '' || kind === 'chat' || kind === 'spawned'
}
