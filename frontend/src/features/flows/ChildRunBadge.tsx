import type { ChildProgress } from './runTree'

// ChildRunBadge is the rollup line a subflow/spawn node shows while the run it
// launched is executing: which node of the CHILD is live, and how far in.
//
// Deliberately NOT a "3/7" ratio. The denominator would be the child flow's node
// count, which is not a bound on how many nodes actually run — a loop re-enters
// nodes and a branch skips them — so the ratio would be wrong in both directions
// and could exceed 1. The child's current node name says more and cannot lie.
//
// Renders nothing without progress, so a node whose child has not started yet (or
// a view that does not follow the run tree) keeps its plain appearance.
export function ChildRunBadge({ child }: { child: ChildProgress | undefined }) {
  if (!child) return null
  const { phase, title, index } = child
  const mark = phase === 'error' ? '✕' : phase === 'done' ? '✓' : phase === 'waiting' ? '⏳' : '▶'
  const tone =
    phase === 'error'
      ? 'text-[var(--color-danger)]'
      : phase === 'done'
        ? 'text-[var(--color-success)]'
        : 'text-[var(--color-accent)]'
  return (
    <div
      className={`mt-0.5 flex items-center gap-1 text-[11px] ${tone}`}
      title="Çift tıkla alt koşuya in"
    >
      <span className="shrink-0">{mark}</span>
      <span className="shrink-0 opacity-70">{index}.</span>
      <span className="truncate">{title || 'alt koşu'}</span>
    </div>
  )
}
