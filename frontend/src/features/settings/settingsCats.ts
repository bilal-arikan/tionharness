import {
  User,
  KeyRound,
  Brain,
  Shield,
  BookOpen,
  Bug,
  Gauge,
  Info,
  Wrench,
  SlidersHorizontal,
  Webhook,
  Archive,
  ScanSearch,
  Volume2,
  Bot,
  Scale,
} from 'lucide-react'
// Settings category tables. Split out so primitives.tsx exports only components
// (fast refresh).
import type { CatMeta } from './primitives'
import { i18next } from '@/i18n'

function category(key: CatMeta['key'], icon: CatMeta['icon']): CatMeta {
  return {
    key,
    get label() {
      return i18next.t(`categories.${key}`, { ns: 'settings' })
    },
    icon,
  }
}

export const APP_CATS: CatMeta[] = [
  category('general', SlidersHorizontal),
  category('profile', User),
  category('sound', Volume2),
  category('providers', KeyRound),
  category('secrets', Shield),
  category('context', Brain),
  category('tools', Wrench),
  category('execution', Gauge),
  category('sysagents', Bot),
  category('decider', Scale),
  category('hooks', Webhook),
  category('exttools', ScanSearch),
  category('diagnostics', Bug),
  category('backup', Archive),
]

// Reference material is separate from the editable settings menu.
export const HELP_CATS: CatMeta[] = [category('reference', BookOpen), category('about', Info)]

export function resolveSettingsCat(value: string | null | undefined): CatMeta['key'] {
  if (value === 'advanced') return 'general'
  if (value === 'commands' || value === 'stepkinds') return 'reference'
  return [...APP_CATS, ...HELP_CATS].find((cat) => cat.key === value)?.key ?? 'profile'
}
