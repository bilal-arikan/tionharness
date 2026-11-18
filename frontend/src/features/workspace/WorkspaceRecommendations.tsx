// WorkspaceRecommendations is the post-workspace-create advisory surface. Like
// ClaudeAuthGate it is driven by a `trigger` counter the parent bumps once per
// successful create/attach (NOT on chat-open, so the user is never nagged mid-work).
//
// The rule catalog + engine live in ./recommendations (shared with the "Öneriler"
// workspace tab). This file is just the toast presentation + ignore persistence.
// A card can be put away two ways: X closes it for now (it may be offered again
// after the next create/attach), while "don't show again" records its key in the
// workspace's ignoredRecommendations so it is never re-offered; the workspace tab
// can review and un-ignore later.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { X } from 'lucide-react'
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
  const { t } = useTranslation('workspace')
  const [recs, setRecs] = useState<Rec[]>([])
  const [busy, setBusy] = useState<string | null>(null)
  // The workspace's persisted ignore list, kept in sync so dismiss can append to it.
  const [ignored, setIgnored] = useState<string[]>([])

  // Clear a previous workspace's cards before re-probing.
  useKeyedReset(trigger, () => {
    if (trigger > 0) setRecs([])
  })
  useEffect(() => {
    if (trigger <= 0) return
    let alive = true
    ;(async () => {
      try {
        const data = await fetchRecommendationData()
        if (!alive) return
        const ign = data.ws.ignoredRecommendations ?? []
        setIgnored(ign)
        const all = runRules({
          ...data,
          nav: { view: onNavigateView, settings: onNavigateSettings },
        })
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

  // Close for now: nothing is persisted, so the card can come back on a later
  // create/attach if its condition still holds.
  const closeForNow = (key: string) => setRecs((r) => r.filter((x) => x.key !== key))

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
      className="pointer-events-none fixed bottom-36 right-4 z-40 flex max-h-[60vh] w-80 max-w-[calc(100vw-2rem)] flex-col gap-2 overflow-y-auto"
    >
      {recs.map((rec) => {
        const Icon = rec.icon
        const warn = rec.variant === 'warning'
        // Neutral card; only the icon carries the tone, so a stack of cards does
        // not read as a block of warning colour over the transcript.
        const iconColor = warn ? 'text-[var(--color-warning)]' : 'text-[var(--color-text-dim)]'
        return (
          <div
            key={rec.key}
            data-testid={`workspace-rec-${rec.key}`}
            className="pointer-events-auto flex items-start gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2.5 text-sm shadow-[var(--shadow-md)]"
          >
            <Icon size={16} className={`mt-0.5 shrink-0 ${iconColor}`} />
            <div className="min-w-0 flex-1">
              <p className="font-medium text-[var(--color-text)]">{rec.title}</p>
              <p className="mt-0.5 break-words text-xs text-[var(--color-text-dim)]">{rec.desc}</p>
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
                <button
                  type="button"
                  onClick={() => run(rec)}
                  disabled={busy === rec.key}
                  data-testid={`workspace-rec-act-${rec.key}`}
                  className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-xs font-medium hover:border-[var(--color-text-dim)] disabled:opacity-50"
                >
                  {busy === rec.key ? '…' : rec.actionLabel}
                </button>
                <button
                  type="button"
                  onClick={() => ignore(rec.key)}
                  title={t('recommendations.toast.ignoreTitle')}
                  data-testid={`workspace-rec-ignore-${rec.key}`}
                  className="text-xs text-[var(--color-text-dim)] underline-offset-2 hover:text-[var(--color-text)] hover:underline"
                >
                  {t('recommendations.toast.ignore')}
                </button>
              </div>
            </div>
            <button
              type="button"
              onClick={() => closeForNow(rec.key)}
              title={t('recommendations.toast.closeTitle')}
              aria-label={t('recommendations.toast.closeTitle')}
              data-testid={`workspace-rec-close-${rec.key}`}
              className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface)] hover:text-[var(--color-text)]"
            >
              <X size={14} />
            </button>
          </div>
        )
      })}
    </div>
  )
}
