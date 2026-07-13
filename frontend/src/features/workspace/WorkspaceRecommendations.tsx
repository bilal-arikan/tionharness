// WorkspaceRecommendations is the post-workspace-create advisory surface. Like
// ClaudeAuthGate it is driven by a `trigger` counter the parent bumps once per
// successful create/attach (NOT on chat-open, so the user is never nagged mid-work).
//
// The rule catalog + engine live in ./recommendations (shared with the "Öneriler"
// workspace tab). This file is just the toast presentation + ignore persistence:
// dismissing a card (X) records its key in the workspace's ignoredRecommendations so
// it is never re-offered; the workspace tab can review and un-ignore later.
import { useEffect, useState } from 'react'
import { Lightbulb, X } from 'lucide-react'
import { api } from '@/api'
import { fetchRecommendationData, runRules, type Rec } from './recommendations'

interface Props {
  // Bumped by the parent once per successful create/attach. A value <= 0 is ignored.
  trigger: number
  // Navigate to a top-level view (NavRail View id).
  onNavigateView: (view: string) => void
  // Open the Settings screen on a given category id (e.g. 'exttools', 'backup').
  onNavigateSettings: (cat: string) => void
  onError?: (msg: string) => void
}

export function WorkspaceRecommendations({
  trigger,
  onNavigateView,
  onNavigateSettings,
  onError,
}: Props) {
  const [recs, setRecs] = useState<Rec[]>([])
  const [busy, setBusy] = useState<string | null>(null)
  // The workspace's persisted ignore list, kept in sync so dismiss can append to it.
  const [ignored, setIgnored] = useState<string[]>([])

  useEffect(() => {
    if (trigger <= 0) return
    let alive = true
    setRecs([]) // clear a previous workspace's cards before re-probing
    ;(async () => {
      try {
        const data = await fetchRecommendationData()
        if (!alive) return
        const ign = data.ws.ignoredRecommendations ?? []
        setIgnored(ign)
        const all = runRules({ ...data, nav: { view: onNavigateView, settings: onNavigateSettings } })
        // Hide anything the user already ignored for this workspace.
        if (alive) setRecs(all.filter((r) => !ign.includes(r.key)))
      } catch (e) {
        // A probe failure must never block workspace use — surface it quietly.
        onError?.((e as Error).message)
      }
    })()
    return () => {
      alive = false
    }
    // Keyed only on `trigger`; nav callback identity is stable and re-running on it
    // would double-probe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trigger])

  // Persist an ignore for this workspace so the card is not re-offered on later
  // creates. Best-effort: the card is removed from view regardless; a persistence
  // failure only means it may reappear next time (and is surfaced).
  const ignore = async (key: string) => {
    setRecs((r) => r.filter((x) => x.key !== key))
    if (ignored.includes(key)) return
    const next = [...ignored, key]
    setIgnored(next)
    try {
      await api.updateWorkspaceSettings({ ignoredRecommendations: next })
    } catch (e) {
      onError?.((e as Error).message)
    }
  }

  const run = async (rec: Rec) => {
    setBusy(rec.key)
    try {
      await rec.act()
      // Acting resolves the underlying condition, so just drop the card from view —
      // no need to persist an ignore (detection won't re-fire next time).
      setRecs((r) => r.filter((x) => x.key !== rec.key))
    } catch (e) {
      onError?.((e as Error).message)
    } finally {
      setBusy(null)
    }
  }

  if (recs.length === 0) return null

  return (
    <div
      data-testid="workspace-recommendations"
      className="pointer-events-none fixed bottom-24 right-4 z-40 flex max-h-[70vh] max-w-md flex-col gap-2 overflow-y-auto"
    >
      {recs.map((rec) => {
        const Icon = rec.icon
        const warn = rec.variant === 'warning'
        const border = warn
          ? 'border-[color-mix(in_srgb,var(--color-warning)_45%,transparent)]'
          : 'border-[color-mix(in_srgb,var(--color-accent)_40%,transparent)]'
        const bg = warn
          ? 'bg-[color-mix(in_srgb,var(--color-warning)_14%,var(--color-surface))]'
          : 'bg-[color-mix(in_srgb,var(--color-accent)_10%,var(--color-surface))]'
        const iconColor = warn ? 'text-[var(--color-warning)]' : 'text-[var(--color-accent)]'
        return (
          <div
            key={rec.key}
            data-testid={`workspace-rec-${rec.key}`}
            className={`pointer-events-auto flex items-start gap-2 rounded-lg border ${border} ${bg} px-3 py-2.5 text-sm shadow-[var(--shadow-md)]`}
          >
            <Icon size={16} className={`mt-0.5 shrink-0 ${iconColor}`} />
            <div className="min-w-0 flex-1">
              <p className="flex items-center gap-1.5 font-medium text-[var(--color-text)]">
                {!warn && <Lightbulb size={12} className="text-[var(--color-accent)]" />}
                {rec.title}
              </p>
              <p className="mt-0.5 break-words text-xs text-[var(--color-text-dim)]">{rec.desc}</p>
              <button
                onClick={() => run(rec)}
                disabled={busy === rec.key}
                data-testid={`workspace-rec-act-${rec.key}`}
                className="mt-2 inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs font-medium hover:border-[var(--color-accent)] disabled:opacity-50"
              >
                {busy === rec.key ? '…' : rec.actionLabel}
              </button>
            </div>
            <button
              onClick={() => ignore(rec.key)}
              title="Yok say (bir daha gösterme)"
              data-testid={`workspace-rec-ignore-${rec.key}`}
              className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <X size={14} />
            </button>
          </div>
        )
      })}
    </div>
  )
}
