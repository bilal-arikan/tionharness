import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

// EmptyState — the centered icon + message placeholder shown when a list/panel
// has no content. Unifies the per-panel hand-rolled empty placeholders.
interface Props {
  icon?: LucideIcon
  title: string
  children?: ReactNode
  className?: string
}

export function EmptyState({ icon: Icon, title, children, className = '' }: Props) {
  return (
    <div
      className={`flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)] ${className}`}
    >
      {Icon && <Icon size={28} strokeWidth={1.5} className="opacity-40" />}
      <p>{title}</p>
      {children && <div className="text-xs">{children}</div>}
    </div>
  )
}
