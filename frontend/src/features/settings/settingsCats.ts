import {
  User,
  KeyRound,
  Brain,
  Shield,
  Command,
  Blocks,
  Info,
  Wrench,
  SlidersHorizontal,
  Webhook,
  Archive,
  ScanSearch,
  Volume2,
} from 'lucide-react'
// Settings category tables. Split out so primitives.tsx exports only components
// (fast refresh).
import type { CatMeta } from './primitives'

export const APP_CATS: CatMeta[] = [
  { key: 'profile', label: 'Profil', icon: User },
  { key: 'providers', label: 'Sağlayıcılar', icon: KeyRound },
  { key: 'secrets', label: 'Sırlar', icon: Shield },
  { key: 'context', label: 'Bağlam & Bellek', icon: Brain },
  { key: 'tools', label: 'Yetenekler (Araçlar)', icon: Wrench },
  { key: 'hooks', label: 'Hooks', icon: Webhook },
  { key: 'exttools', label: 'Harici Araçlar', icon: ScanSearch },
  // Dedicated audio page: sound effects, speech input (STT) + output (TTS).
  { key: 'sound', label: 'Ses', icon: Volume2 },
  // Combined screen: notifications, autonomy, auto-title, MCP, diagnostics.
  { key: 'advanced', label: 'Gelişmiş', icon: SlidersHorizontal },
  // Dedicated page: backup schedule + archive list / restore.
  { key: 'backup', label: 'Yedekleme', icon: Archive },
  { key: 'commands', label: 'Komutlar', icon: Command },
  { key: 'stepkinds', label: 'Adım Türleri', icon: Blocks },
  { key: 'about', label: 'Hakkında', icon: Info },
]

// Setters threaded into the per-category panels.
