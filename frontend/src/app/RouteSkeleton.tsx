import { Skeleton } from '@/shared/components'

interface Props {
  // Announced to assistive tech; nothing is drawn for it.
  label: string
  // Headerless views draw their own pane header, so the skeleton sketches one.
  // Views under the shared AppHeader already have it on screen.
  header?: boolean
}

// RouteSkeleton is the Suspense fallback for a lazily loaded screen. It keeps
// the screen's frame — the pane header geometry (same padding and border as
// PaneHeader) and a few content blocks — so the layout does not jump from a
// centred spinner to the real screen when the chunk arrives.
export function RouteSkeleton({ label, header = true }: Props) {
  return (
    <div
      role="status"
      aria-label={label}
      data-testid="route-skeleton"
      className="flex min-h-0 flex-1 flex-col overflow-hidden"
    >
      {header && (
        <div className="flex shrink-0 items-center gap-2 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
          <Skeleton className="h-8 w-40" />
          <div className="ml-auto flex items-center gap-2">
            <Skeleton className="h-8 w-20" />
            <Skeleton className="h-8 w-24" />
          </div>
        </div>
      )}
      <div className="flex min-h-0 flex-1 flex-col gap-4 py-4 max-md:px-3 md:px-6">
        <Skeleton className="h-16 w-full shrink-0" />
        <div className="grid shrink-0 grid-cols-1 gap-4 md:grid-cols-2">
          <Skeleton className="h-36" />
          <Skeleton className="h-36" />
        </div>
        <Skeleton className="min-h-0 w-full flex-1" />
      </div>
    </div>
  )
}
