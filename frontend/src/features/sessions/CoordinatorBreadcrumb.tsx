import { useEffect, useState } from 'react'
import { ArrowLeft, ChevronRight } from 'lucide-react'
import { api } from '@/api'
import type { CoordinatorAncestor } from '@/types'
import { ComposerCard } from '@/features/chat/ComposerCard'

interface Props {
  sessionId: string
  // Opens another session. Without it the chain is rendered as plain text (there
  // is nowhere to navigate to).
  onSelectSession?: (id: string) => void
  // Floating variant for the read-only chat stack: wrap in a ComposerCard so the
  // panel gets the same opaque surface + shadow as its siblings (todo/wake), since
  // it overlays the transcript. Default (side panel) keeps the flat bordered box,
  // which already sits on a solid panel background.
  floating?: boolean
}

// CoordinatorBreadcrumb renders the chain of coordinators ABOVE a worker session,
// root first. It replaces the old single "back to coordinator" button, which only
// worked while trees were one level deep: in a nested tree the direct parent is
// often itself a worker, and jumping one level up told you nothing about where
// the work actually came from.
//
// Renders nothing for a root coordinator or an ordinary session (empty chain).
export function CoordinatorBreadcrumb({ sessionId, onSelectSession, floating }: Props) {
  const [chain, setChain] = useState<CoordinatorAncestor[]>([])

  useEffect(() => {
    let cancelled = false
    api
      .getCoordinatorAncestors(sessionId)
      .then((d) => {
        if (!cancelled) setChain(d.ancestors ?? [])
      })
      // A session with no coordinator above it is the normal case, not an error
      // worth surfacing — an empty chain simply renders nothing.
      .catch(() => {
        if (!cancelled) setChain([])
      })
    return () => {
      cancelled = true
    }
  }, [sessionId])

  if (chain.length === 0) return null

  const inner = (
    <>
      <div className="mb-1 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        <ArrowLeft size={12} /> Üst zincir
      </div>
      <div className="flex flex-wrap items-center gap-x-1 gap-y-1">
        {chain.map((a, i) => (
          <span key={a.sessionId} className="flex items-center gap-1">
            {i > 0 && <ChevronRight size={11} className="shrink-0 text-[var(--color-text-dim)]" />}
            {onSelectSession ? (
              <button
                type="button"
                onClick={() => onSelectSession(a.sessionId)}
                title={`${a.title || a.sessionId} oturumunu aç`}
                className="rounded-md border border-[var(--color-border)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                {a.agentName}
                <span className="ml-1 opacity-60">s{a.depth}</span>
              </button>
            ) : (
              <span className="text-[10px] text-[var(--color-text-dim)]">{a.agentName}</span>
            )}
          </span>
        ))}
        <ChevronRight size={11} className="shrink-0 text-[var(--color-text-dim)]" />
        <span className="rounded-md bg-[var(--color-accent)]/10 px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-accent)]">
          bu oturum
        </span>
      </div>
    </>
  )

  // Floating: same opaque surface + shadow as the sibling ComposerCards it stacks
  // with. Default: the flat bordered box, for the (solid) side panel.
  if (floating)
    return (
      <ComposerCard tone="plain" className="px-2.5 py-2">
        {inner}
      </ComposerCard>
    )
  return <div className="rounded-lg border border-[var(--color-border)] px-2.5 py-2">{inner}</div>
}
