import type { ReactNode } from 'react'

// Keep controls mounted when collapsed so invalid input still blocks saving.
export function SettingsDisclosure({ title, children }: { title: string; children: ReactNode }) {
  return (
    <details className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
      <summary className="cursor-pointer px-4 py-3 text-sm font-semibold">{title}</summary>
      <div className="flex flex-col gap-4 border-t border-[var(--color-border)] p-4">
        {children}
      </div>
    </details>
  )
}
