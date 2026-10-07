import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'
import { FieldHint, InfoPopover } from './InfoPopover'

// EmptyState — the centered icon + message placeholder shown when a list/panel
// has no content. Unifies the per-panel hand-rolled empty placeholders.
interface Props {
  icon?: LucideIcon
  title: string
  // Optional secondary "how to" explanation: a line under the title when short,
  // an (ⓘ) after the title when long.
  hint?: ReactNode
  children?: ReactNode
  className?: string
}

export function EmptyState({ icon: Icon, title, hint, children, className = '' }: Props) {
  return (
    <div
      className={`flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)] ${className}`}
    >
      {Icon && <Icon size={28} strokeWidth={1.5} className="opacity-40" />}
      <p>
        {title}
        {hint && (
          <>
            {' '}
            <InfoPopover text={hint} mode="long" />
          </>
        )}
      </p>
      <FieldHint text={hint} />
      {children && <div className="text-xs">{children}</div>}
    </div>
  )
}
