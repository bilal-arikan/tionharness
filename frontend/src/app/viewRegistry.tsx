import { i18next } from '@/i18n'
// View registry: per-view metadata (titles, header layout) and small shared
// view predicates. Keeping these here (not in App.tsx) lets the shell stay a
// thin composition layer while every view-list concern lives in one place.
// The lazy panel chunks live in lazyPanels.ts (components-only module).
import type { View } from './NavRail'

// Minimum time the first-run splash stays on screen (ms), so an instant workspace
// load doesn't flash the logo for a single frame. Only applies on a fresh install.
export const SPLASH_MIN_MS = 1100

export const VIEW_TITLE: Record<View, string> = {
  get dashboard() {
    return i18next.t('navigation.dashboard')
  },
  get chat() {
    return i18next.t('navigation.chat')
  },
  get agents() {
    return i18next.t('navigation.agents')
  },
  get rota() {
    return i18next.t('navigation.rota')
  },
  get explorer() {
    return i18next.t('navigation.explorer')
  },
  get board() {
    return i18next.t('navigation.board')
  },
  get schedules() {
    return i18next.t('navigation.schedules')
  },
  get flows() {
    return i18next.t('navigation.flows')
  },
  get artifacts() {
    return i18next.t('navigation.artifacts')
  },
  get skills() {
    return i18next.t('navigation.skills')
  },
  get tools() {
    return i18next.t('navigation.tools')
  },
  get budget() {
    return i18next.t('navigation.budget')
  },
  get prompts() {
    return i18next.t('navigation.promptsFiles')
  },
  get insights() {
    return i18next.t('navigation.insights')
  },
  get market() {
    return i18next.t('navigation.market')
  },
  get workspace() {
    return i18next.t('navigation.workspace')
  },
  get settings() {
    return i18next.t('navigation.settings')
  },
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
  'rota',
  'explorer',
  'dashboard',
])

// The composer gate lives in shared/lib so the session panels can share it with
// the shell; re-exported here for the existing app-layer call sites.
export { isWritableSessionKind } from '@/shared/lib/sessionKind'
