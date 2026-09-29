import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { RefreshCw, Trash2 } from 'lucide-react'
import { api } from '@/api'
import { toast } from '@/shared/components'
import type { Lesson } from '@/types'
import { formatDateTime } from '@/shared/lib/intl'

// LessonsList shows the workspace's auto-collected failure lessons (read-only
// store written by the lesson reflector) with a per-row prune button. fetches on
// mount + manual refresh. fill=true makes it a full-height page (flex column with
// the list growing to fill); the default is the compact embedded card (max-h-56).
export function LessonsList({ fill = false }: { fill?: boolean } = {}) {
  const { t } = useTranslation('settingsMain')
  const [lessons, setLessons] = useState<Lesson[] | null>(null)
  const [error, setError] = useState('')
  // Busy from the first paint: the mount fetch is already in flight. run lands
  // results through callbacks only; load is the manual-refresh entry point.
  const [busy, setBusy] = useState(true)

  const run = useCallback(
    () =>
      api
        .listLessons()
        .then(setLessons)
        .catch((e) => setError((e as Error).message))
        .finally(() => setBusy(false)),
    [],
  )
  const load = useCallback(() => {
    setBusy(true)
    setError('')
    return run()
  }, [run])

  useEffect(() => {
    void run()
  }, [run])

  const remove = async (id: string) => {
    try {
      await api.deleteLesson(id)
      setLessons((cur) => (cur ? cur.filter((l) => l.id !== id) : cur))
      toast.success(t('lessons.deleted'))
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <div
      className={`rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 ${fill ? 'flex h-full min-h-0 flex-col' : ''}`}
    >
      <div className="mb-1 flex items-center justify-between">
        <span className="text-xs font-medium text-[var(--color-text)]">
          {t('lessons.title')} {lessons ? `(${lessons.length})` : ''}
        </span>
        <button
          type="button"
          onClick={() => void load()}
          disabled={busy}
          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)] disabled:opacity-50"
          title={t('shared.refresh')}
        >
          <RefreshCw size={12} className={busy ? 'animate-spin' : ''} /> {t('shared.refresh')}
        </button>
      </div>
      {error && <p className="text-xs text-[var(--color-danger)]">{error}</p>}
      {lessons && lessons.length === 0 && !error && (
        <p className="text-xs text-[var(--color-text-dim)]">{t('lessons.empty')}</p>
      )}
      {lessons && lessons.length > 0 && (
        <ul className={`space-y-1.5 overflow-y-auto ${fill ? 'min-h-0 flex-1' : 'max-h-56'}`}>
          {lessons.map((l) => (
            <li
              key={l.id}
              className="group flex items-start gap-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                  {l.tool && (
                    <span className="rounded bg-[var(--color-surface-2)] px-1 py-px font-mono text-[10px] text-[var(--color-text)]">
                      {l.tool}
                    </span>
                  )}
                  {l.count > 1 && <span>{t('lessons.seen', { count: l.count })}</span>}
                  <span>
                    {formatDateTime(new Date(l.ts * 1000), {
                      dateStyle: 'short',
                      timeStyle: 'medium',
                    })}
                  </span>
                </div>
                <p className="mt-0.5 text-xs leading-snug text-[var(--color-text)]">{l.text}</p>
              </div>
              <button
                type="button"
                onClick={() => void remove(l.id)}
                className="mt-0.5 shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100"
                title={t('lessons.deleteTitle')}
              >
                <Trash2 size={13} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
