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

export const APP_CATS: CatMeta[] = [
  { key: 'general', label: 'General', icon: SlidersHorizontal },
  { key: 'profile', label: 'Profile', icon: User },
  { key: 'sound', label: 'Sound & notifications', icon: Volume2 },
  { key: 'providers', label: 'Providers', icon: KeyRound },
  { key: 'secrets', label: 'Secrets', icon: Shield },
  { key: 'context', label: 'Context & memory', icon: Brain },
  { key: 'tools', label: 'Tool permissions', icon: Wrench },
  { key: 'execution', label: 'Agent execution', icon: Gauge },
  { key: 'sysagents', label: 'Agent library', icon: Bot },
  { key: 'decider', label: 'Decision authorities', icon: Scale },
  { key: 'hooks', label: 'Hooks', icon: Webhook },
  { key: 'exttools', label: 'External tools', icon: ScanSearch },
  { key: 'diagnostics', label: 'Diagnostics', icon: Bug },
  { key: 'backup', label: 'Backup', icon: Archive },
]

// Reference material is separate from the editable settings menu.
export const HELP_CATS: CatMeta[] = [
  { key: 'reference', label: 'Reference', icon: BookOpen },
  { key: 'about', label: 'About', icon: Info },
]

export function resolveSettingsCat(value: string | null | undefined): CatMeta['key'] {
  if (value === 'advanced') return 'general'
  if (value === 'commands' || value === 'stepkinds') return 'reference'
  return [...APP_CATS, ...HELP_CATS].find((cat) => cat.key === value)?.key ?? 'profile'
}
