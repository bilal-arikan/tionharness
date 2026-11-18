import type { ReactNode } from 'react'

interface Props {
  // Draws its own opaque ground. Needed where no composer sits underneath (the
  // read-only run log), so the transcript never shows through the line.
  opaque?: boolean
  children: ReactNode
}

// ComposerStatusLine is the one quiet row for ambient session status (prompt-cache
// warmth, other windows on the same session). It replaces the pills that used to
// float at the top of the bottom stack, where the transcript scrolled through
// them: the line sits on opaque ground below the composer card instead, so it
// never overlaps message text. Collapses entirely when it has nothing to show.
export function ComposerStatusLine({ opaque, children }: Props) {
  return (
    <div
      data-testid="composer-status-line"
      className={`flex flex-wrap items-center justify-center gap-x-3 gap-y-0.5 pt-1 text-[11px] leading-4 text-[var(--color-text-dim)] empty:hidden ${
        opaque ? '-mx-4 -mb-2 bg-[var(--color-bg)] px-4 pb-1' : ''
      }`}
    >
      {children}
    </div>
  )
}
