import { Skeleton } from '@/shared/components'

// Widths of the placeholder bubbles, alternating right (user) / left (assistant)
// so the skeleton reads as a transcript rather than a generic loading block.
const BUBBLES = [
  { align: 'end', width: 'w-1/3' },
  { align: 'start', width: 'w-2/3' },
  { align: 'end', width: 'w-1/4' },
  { align: 'start', width: 'w-1/2' },
] as const

// ChatSkeleton stands in for the transcript while the session's messages are
// being fetched (or while the workspace is still bootstrapping), so the user
// never sees the previous chat's messages or a misleading empty state.
export function ChatSkeleton() {
  return (
    <div
      data-testid="chat-skeleton"
      className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden px-6 py-8"
    >
      {BUBBLES.map((b, i) => (
        <div key={i} className={`flex ${b.align === 'end' ? 'justify-end' : 'justify-start'}`}>
          <div className={`flex flex-col gap-2 ${b.width}`}>
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-3/4" />
          </div>
        </div>
      ))}
    </div>
  )
}
