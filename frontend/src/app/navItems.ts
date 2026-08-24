// The primary view list, shared by the desktop rail and the mobile bottom bar.
// Split out so NavRail.tsx exports only components (fast refresh).
import {
  LayoutDashboard,
  MessageSquare,
  Users,
  Share2,
  Map as MapIcon,
  LayoutGrid,
  Clock,
  GitBranch,
  FileCode,
  Sparkles,
  Plug,
  Store,
  Wallet,
  ScrollText,
  Lightbulb,
  type LucideIcon,
} from 'lucide-react'
import type { View } from './NavRail'

export const NAV: { key: View; label: string; icon: LucideIcon }[] = [
  { key: 'dashboard', label: 'Panel', icon: LayoutDashboard },
  { key: 'chat', label: 'Sohbet', icon: MessageSquare },
  { key: 'agents', label: 'Ajanlar', icon: Users },
  { key: 'network', label: 'Ağ', icon: Share2 },
  { key: 'explorer', label: 'Harita', icon: MapIcon },
  { key: 'board', label: 'Görevler', icon: LayoutGrid },
  { key: 'schedules', label: 'Otomasyon', icon: Clock },
  { key: 'flows', label: 'Akışlar', icon: GitBranch },
  { key: 'artifacts', label: 'Artifactlar', icon: FileCode },
  { key: 'skills', label: 'Skills', icon: Sparkles },
  { key: 'tools', label: 'Araçlar & MCP', icon: Plug },
  { key: 'market', label: 'Market', icon: Store },
  { key: 'budget', label: 'Bütçe', icon: Wallet },
  { key: 'logs', label: 'Loglar', icon: ScrollText },
  { key: 'insights', label: 'İçgörü', icon: Lightbulb },
]
