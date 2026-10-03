import { i18next } from '@/i18n'
import {
  Orbit,
  LayoutDashboard,
  MessageSquare,
  Users,
  Waypoints,
  LayoutGrid,
  Clock,
  GitBranch,
  FileCode,
  Sparkles,
  Plug,
  Store,
  Wallet,
  FileText,
  Lightbulb,
  Boxes,
  Settings,
} from 'lucide-react'

// One destination list for the rail, mobile strip, titles and shell layout.
// Panel components stay in lazyPanels so importing metadata never loads them.
const VIEWS = [
  {
    key: 'dashboard',
    labelKey: 'navigation.dashboard',
    icon: LayoutDashboard,
    primary: true,
    headerless: true,
  },
  {
    key: 'chat',
    labelKey: 'navigation.chat',
    icon: MessageSquare,
    primary: true,
    headerless: false,
  },
  { key: 'agents', labelKey: 'navigation.agents', icon: Users, primary: true, headerless: true },
  { key: 'rota', labelKey: 'navigation.rota', icon: Waypoints, primary: true, headerless: true },
  {
    key: 'explorer',
    labelKey: 'navigation.explorer',
    icon: Orbit,
    primary: true,
    headerless: true,
  },
  { key: 'board', labelKey: 'navigation.board', icon: LayoutGrid, primary: true, headerless: true },
  {
    key: 'schedules',
    labelKey: 'navigation.schedules',
    icon: Clock,
    primary: true,
    headerless: true,
  },
  { key: 'flows', labelKey: 'navigation.flows', icon: GitBranch, primary: true, headerless: true },
  {
    key: 'artifacts',
    labelKey: 'navigation.artifacts',
    icon: FileCode,
    primary: true,
    headerless: true,
  },
  { key: 'skills', labelKey: 'navigation.skills', icon: Sparkles, primary: true, headerless: true },
  { key: 'tools', labelKey: 'navigation.tools', icon: Plug, primary: true, headerless: true },
  // Preserve the existing command palette order as well as the rail order.
  {
    key: 'market',
    labelKey: 'navigation.market',
    icon: Store,
    primary: true,
    headerless: true,
    titleOrder: 14,
  },
  {
    key: 'budget',
    labelKey: 'navigation.budget',
    icon: Wallet,
    primary: true,
    headerless: true,
    titleOrder: 11,
  },
  {
    key: 'prompts',
    labelKey: 'navigation.promptsFiles',
    icon: FileText,
    primary: true,
    headerless: true,
    titleOrder: 12,
  },
  {
    key: 'insights',
    labelKey: 'navigation.insights',
    icon: Lightbulb,
    primary: true,
    headerless: true,
    titleOrder: 13,
  },
  {
    key: 'workspace',
    labelKey: 'navigation.workspace',
    icon: Boxes,
    primary: false,
    headerless: false,
  },
  {
    key: 'settings',
    labelKey: 'navigation.settings',
    icon: Settings,
    primary: false,
    headerless: false,
  },
] as const

export type View = (typeof VIEWS)[number]['key']

export const VIEW_METADATA = VIEWS.map((item, index) => ({
  ...item,
  titleOrder: 'titleOrder' in item ? item.titleOrder : index,
  get label() {
    return i18next.t(item.labelKey)
  },
}))

export const PRIMARY_NAV = VIEW_METADATA.filter((item) => item.primary)
export const PINNED_NAV = VIEW_METADATA.filter((item) => !item.primary)

export const VIEW_TITLE = Object.defineProperties(
  {},
  Object.fromEntries(
    [...VIEW_METADATA]
      .sort((a, b) => a.titleOrder - b.titleOrder)
      .map((item) => [item.key, { enumerable: true, get: () => item.label }]),
  ),
) as Record<View, string>

export const HEADERLESS_VIEWS = new Set<View>(
  VIEW_METADATA.filter((item) => item.headerless).map((item) => item.key),
)
