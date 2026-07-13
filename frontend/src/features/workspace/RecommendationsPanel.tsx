// RecommendationsPanel is the "Öneriler" workspace tab: it lists every recommendation
// rule (see ./recommendations), shows each rule's live state — currently applicable,
// ignored, or not applicable — and lets the user toggle the per-workspace ignore.
//
// Self-contained (own load, own persistence via updateWorkspaceSettings), exempt from
// the shared workspace Save bar — like ExternalToolsPanel. It reuses the SAME rule
// catalog as the post-create toast, so the two never drift.
import { useEffect, useState } from 'react'
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
  const [loading, setLoading] = useState(true)
  // Keys of rules whose condition currently holds for this workspace.
  const [applicable, setApplicable] = useState<Set<string>>(new Set())
  // The workspace's persisted ignore list.
  const [ignored, setIgnored] = useState<string[]>([])
  const [busy, setBusy] = useState<string | null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const data = await fetchRecommendationData()
      setIgnored(data.ws.ignoredRecommendations ?? [])
      // NOOP_NAV: we only need which rules apply, never invoke their actions here.
      setApplicable(new Set(runRules({ ...data, nav: NOOP_NAV }).map((r) => r.key)))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
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
        Yeni bir workspace oluşturulduğunda sağ-altta çıkan öneri kartları. Buradan
        hepsinin <span className="font-medium text-[var(--color-text)]">şu anki durumunu</span> görür,
        <span className="font-medium text-[var(--color-text)]"> yok saydıklarını</span> gözden geçirir ve{' '}
        <span className="font-medium text-[var(--color-text)]">yok saymayı kaldırabilirsin</span>. Yok sayılan bir
        öneri bir daha kart olarak gösterilmez.
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={load}
          disabled={loading}
          className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-1.5 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] disabled:opacity-50"
        >
          <ScanSearch size={14} className="text-[var(--color-accent)]" />
          {loading ? 'Yükleniyor…' : 'Durumu yenile'}
        </button>
        {onShowCards && (
          <button
            type="button"
            onClick={onShowCards}
            disabled={loading || visibleCount === 0}
            data-testid="rec-show-cards"
            title={
              visibleCount === 0
                ? 'Şu an gösterilecek (geçerli ve yok sayılmamış) öneri yok'
                : 'Geçerli önerileri sağ-altta kart olarak göster'
            }
            className="inline-flex w-fit items-center gap-1.5 rounded-lg border border-[var(--color-accent)] bg-[var(--color-accent)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-50"
          >
            <Bell size={14} />
            {visibleCount > 0 ? `Kartları göster (${visibleCount})` : 'Kartları göster'}
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
                isIgnored ? 'border-[var(--color-border)] opacity-60' : 'border-[var(--color-border)]'
              }`}
            >
              <div className="flex min-w-0 items-start gap-2">
                <Icon size={16} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2 text-sm">
                    <span className="font-medium text-[var(--color-text)]">{title}</span>
                    {isIgnored ? (
                      <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                        Yok sayıldı
                      </span>
                    ) : isApplicable ? (
                      <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
                        Şu an geçerli
                      </span>
                    ) : (
                      <span className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                        Uygulanabilir değil
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
                title={isIgnored ? 'Yok saymayı kaldır — tekrar önerilebilir' : 'Yok say — bir daha önerme'}
              >
                {busy === key ? (
                  '…'
                ) : isIgnored ? (
                  <>
                    <Eye size={12} /> Yok saymayı kaldır
                  </>
                ) : (
                  <>
                    <EyeOff size={12} /> Yok say
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
