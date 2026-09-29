// RecommendationsPanel is the "Öneriler" workspace tab: it lists every recommendation
// rule (see ./recommendations), shows each rule's live state — currently applicable,
// ignored, or not applicable — and lets the user toggle the per-workspace ignore.
//
// Self-contained (own load, own persistence via updateWorkspaceSettings), exempt from
// the shared workspace Save bar — like ExternalToolsPanel. It reuses the SAME rule
// catalog as the post-create toast, so the two never drift.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { EyeOff, Eye, ScanSearch, Bell } from 'lucide-react'
import { api } from '@/api'
import { RULES, fetchRecommendationData, runRules, NOOP_NAV } from './recommendations'

interface Props {
  onError: (msg: string) => void
  // Re-trigger the post-create recommendation toast on demand (App bumps recsTrigger).
  // Absent → the "show cards" button is hidden.
  onShowCards?: () => void
}

export function RecommendationsPanel({ onError, onShowCards }: Props) {
  const { t } = useTranslation('workspace')
  const [loading, setLoading] = useState(true)
  // Keys of rules whose condition currently holds for this workspace.
  const [applicable, setApplicable] = useState<Set<string>>(new Set())
  // The workspace's persisted ignore list.
  const [ignored, setIgnored] = useState<string[]>([])
  const [busy, setBusy] = useState<string | null>(null)

  // run lands the probe through callbacks only (so the mount effect may call
  // it; loading starts true); load is the button entry point that re-arms the
  // spinner.
  const run = () =>
    fetchRecommendationData()
      .then((data) => {
        setIgnored(data.ws.ignoredRecommendations ?? [])
        // NOOP_NAV: we only need which rules apply, never invoke their actions here.
        setApplicable(new Set(runRules({ ...data, nav: NOOP_NAV }).map((r) => r.key)))
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  const load = () => {
    setLoading(true)
    return run()
  }

  useEffect(() => {
    void run()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Add/remove a rule key from this workspace's ignore list and persist it.
  const toggleIgnore = async (key: string) => {
    const next = ignored.includes(key) ? ignored.filter((k) => k !== key) : [...ignored, key]
    setBusy(key)
    setIgnored(next) // optimistic
    try {
      await api.updateWorkspaceSettings({ ignoredRecommendations: next })
    } catch (e) {
      onError((e as Error).message)
      await load() // reconcile on failure
    } finally {
      setBusy(null)
    }
  }

  // How many cards the toast would actually show right now: applicable and not
  // ignored. Drives the "show cards" button's enabled state so it never fires an
  // empty toast.
  const visibleCount = [...applicable].filter((k) => !ignored.includes(k)).length

  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs leading-relaxed text-[var(--color-text-dim)]">
        {t('recommendations.panel.description')}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={load}
          disabled={loading}
          className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-50"
        >
          <ScanSearch size={14} className="text-[var(--color-accent)]" />
          {loading ? t('actions.loading') : t('recommendations.panel.refresh')}
        </button>
        {onShowCards && (
          <button
            type="button"
            onClick={onShowCards}
            disabled={loading || visibleCount === 0}
            data-testid="rec-show-cards"
            title={
              visibleCount === 0
                ? t('recommendations.panel.noCardsTitle')
                : t('recommendations.panel.showCardsTitle')
            }
            className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-accent)] bg-[var(--color-accent)] px-3 py-1.5 text-sm font-medium text-[var(--color-on-accent)] transition hover:opacity-90 disabled:opacity-50"
          >
            <Bell size={14} />
            {visibleCount > 0
              ? t('recommendations.panel.showCardsCount', { count: visibleCount })
              : t('recommendations.panel.showCards')}
          </button>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        {RULES.map((rule) => {
          const { key, icon: Icon, title, summary } = rule.meta
          const isIgnored = ignored.includes(key)
          const isApplicable = applicable.has(key)
          return (
            <div
              key={key}
              data-testid={`rec-row-${key}`}
              className={`flex items-start justify-between gap-3 rounded-lg border px-3 py-2 ${
                isIgnored
                  ? 'border-[var(--color-border)] opacity-60'
                  : 'border-[var(--color-border)]'
              }`}
            >
              <div className="flex min-w-0 items-start gap-2">
                <Icon size={16} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-medium text-[var(--color-text)]">{title}</span>
                    {isIgnored ? (
                      <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                        {t('recommendations.panel.state.ignored')}
                      </span>
                    ) : isApplicable ? (
                      <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
                        {t('recommendations.panel.state.applicable')}
                      </span>
                    ) : (
                      <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                        {t('recommendations.panel.state.notApplicable')}
                      </span>
                    )}
                  </div>
                  <div className="mt-0.5 text-xs text-[var(--color-text-dim)]">{summary}</div>
                </div>
              </div>
              <button
                type="button"
                onClick={() => toggleIgnore(key)}
                disabled={busy === key}
                data-testid={`rec-toggle-${key}`}
                className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs font-medium transition hover:border-[var(--color-accent)] disabled:opacity-50"
                title={
                  isIgnored
                    ? t('recommendations.panel.unignoreTitle')
                    : t('recommendations.panel.ignoreTitle')
                }
              >
                {busy === key ? (
                  '…'
                ) : isIgnored ? (
                  <>
                    <Eye size={12} /> {t('recommendations.panel.unignore')}
                  </>
                ) : (
                  <>
                    <EyeOff size={12} /> {t('recommendations.panel.ignore')}
                  </>
                )}
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}
