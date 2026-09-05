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
  { key: 'dashboard', label: 'Panel', icon: LayoutDashboard },
  { key: 'chat', label: 'Sohbet', icon: MessageSquare },
  { key: 'agents', label: 'Ajanlar', icon: Users },
  { key: 'rota', label: 'Rota', icon: Waypoints },
  { key: 'explorer', label: 'Harita', icon: Orbit },
  { key: 'board', label: 'Görevler', icon: LayoutGrid },
  { key: 'schedules', label: 'Otomasyon', icon: Clock },
  { key: 'flows', label: 'Akışlar', icon: GitBranch },
  { key: 'artifacts', label: 'Artifactlar', icon: FileCode },
  { key: 'skills', label: 'Skills', icon: Sparkles },
  { key: 'tools', label: 'Araçlar & MCP', icon: Plug },
  { key: 'market', label: 'Market', icon: Store },
  { key: 'budget', label: 'Bütçe', icon: Wallet },
  { key: 'prompts', label: 'Prompts', labelKey: 'navigation.promptsFiles', icon: FileText },
  { key: 'insights', label: 'İçgörü', icon: Lightbulb },
]
