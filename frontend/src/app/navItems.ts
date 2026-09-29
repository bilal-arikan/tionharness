import { i18next } from '@/i18n'
// The primary view list, shared by the desktop rail and the mobile bottom bar.
// Split out so NavRail.tsx exports only components (fast refresh).
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
  type LucideIcon,
} from 'lucide-react'
import type { View } from './NavRail'

export const NAV: { key: View; label: string; labelKey?: string; icon: LucideIcon }[] = [
  {
    key: 'dashboard',
    get label() {
      return i18next.t('navigation.dashboard')
    },
    labelKey: 'navigation.dashboard',
    icon: LayoutDashboard,
  },
  {
    key: 'chat',
    get label() {
      return i18next.t('navigation.chat')
    },
    labelKey: 'navigation.chat',
    icon: MessageSquare,
  },
  {
    key: 'agents',
    get label() {
      return i18next.t('navigation.agents')
    },
    labelKey: 'navigation.agents',
    icon: Users,
  },
  {
    key: 'rota',
    get label() {
      return i18next.t('navigation.rota')
    },
    labelKey: 'navigation.rota',
    icon: Waypoints,
  },
  {
    key: 'explorer',
    get label() {
      return i18next.t('navigation.explorer')
    },
    labelKey: 'navigation.explorer',
    icon: Orbit,
  },
  {
    key: 'board',
    get label() {
      return i18next.t('navigation.board')
    },
    labelKey: 'navigation.board',
    icon: LayoutGrid,
  },
  {
    key: 'schedules',
    get label() {
      return i18next.t('navigation.schedules')
    },
    labelKey: 'navigation.schedules',
    icon: Clock,
  },
  {
    key: 'flows',
    get label() {
      return i18next.t('navigation.flows')
    },
    labelKey: 'navigation.flows',
    icon: GitBranch,
  },
  {
    key: 'artifacts',
    get label() {
      return i18next.t('navigation.artifacts')
    },
    labelKey: 'navigation.artifacts',
    icon: FileCode,
  },
  {
    key: 'skills',
    get label() {
      return i18next.t('navigation.skills')
    },
    labelKey: 'navigation.skills',
    icon: Sparkles,
  },
  {
    key: 'tools',
    get label() {
      return i18next.t('navigation.tools')
    },
    labelKey: 'navigation.tools',
    icon: Plug,
  },
  {
    key: 'market',
    get label() {
      return i18next.t('navigation.market')
    },
    labelKey: 'navigation.market',
    icon: Store,
  },
  {
    key: 'budget',
    get label() {
      return i18next.t('navigation.budget')
    },
    labelKey: 'navigation.budget',
    icon: Wallet,
  },
  {
    key: 'prompts',
    get label() {
      return i18next.t('navigation.promptsFiles')
    },
    labelKey: 'navigation.promptsFiles',
    icon: FileText,
  },
  {
    key: 'insights',
    get label() {
      return i18next.t('navigation.insights')
    },
    labelKey: 'navigation.insights',
    icon: Lightbulb,
  },
]
