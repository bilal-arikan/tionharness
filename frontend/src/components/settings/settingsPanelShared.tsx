// Shared building blocks for the app-global settings category panels. The
// individual panels live in their own files (ProfilePanel, ContextPanel, …) and
// are re-exported from appPanels.tsx.
import type { LucideIcon } from 'lucide-react'
import type { AppSettings } from '../../types'
import type { AppSet } from './primitives'

export interface PanelProps {
  draft: AppSettings
  set: AppSet
  setDraft: React.Dispatch<React.SetStateAction<AppSettings | null>>
}

// SubHead is a small icon + label group header used to organise a settings
// panel into labelled sections (accent icon, matches the rail/header style).
export function SubHead({ icon: Icon, children }: { icon: LucideIcon; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-1.5 pt-1 text-sm font-medium text-[var(--color-text)]">
      <Icon size={14} className="text-[var(--color-accent)]" />
      {children}
    </div>
  )
}
