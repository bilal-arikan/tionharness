import { useCallback, useEffect, useState } from 'react'
import { RefreshCw, Trash2 } from 'lucide-react'
import { api } from '@/api'
import type { Lesson } from '@/types'

// LessonsList shows the workspace's auto-collected failure lessons (read-only
// store written by the lesson reflector) with a per-row prune button. Rendered
// inside the Settings "Self-healing" section; fetches on mount + manual refresh.
export function LessonsList() {
  const [lessons, setLessons] = useState<Lesson[] | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setBusy(true)
    setError('')
    try {
      setLessons(await api.listLessons())
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const remove = async (id: string) => {
    try {
      await api.deleteLesson(id)
      setLessons((cur) => (cur ? cur.filter((l) => l.id !== id) : cur))
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
      <div className="mb-1 flex items-center justify-between">
        <span className="text-xs font-medium text-[var(--color-text)]">
          Kayıtlı dersler {lessons ? `(${lessons.length})` : ''}
        </span>
        <button
          type="button"
          onClick={() => void load()}
          disabled={busy}
          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)] disabled:opacity-50"
          title="Yenile"
        >
          <RefreshCw size={12} className={busy ? 'animate-spin' : ''} /> Yenile
        </button>
      </div>
      {error && <p className="text-xs text-[var(--color-danger)]">{error}</p>}
      {lessons && lessons.length === 0 && !error && (
        <p className="text-xs text-[var(--color-text-dim)]">
          Henüz ders yok — kötü biten bir turdan sonra burada görünür. En yeni 5 ders her turun bağlamına otomatik enjekte edilir.
        </p>
      )}
      {lessons && lessons.length > 0 && (
        <ul className="max-h-56 space-y-1.5 overflow-y-auto">
          {lessons.map((l) => (
            <li key={l.id} className="group flex items-start gap-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5">
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                  {l.tool && (
                    <span className="rounded bg-[var(--color-surface-2)] px-1 py-px font-mono text-[10px] text-[var(--color-text)]">{l.tool}</span>
                  )}
                  {l.count > 1 && <span>{l.count}× görüldü</span>}
                  <span>{new Date(l.ts * 1000).toLocaleString('tr-TR')}</span>
                </div>
                <p className="mt-0.5 text-xs leading-snug text-[var(--color-text)]">{l.text}</p>
              </div>
              <button
                type="button"
                onClick={() => void remove(l.id)}
                className="mt-0.5 shrink-0 rounded p-1 text-[var(--color-text-dim)] opacity-0 transition-opacity hover:text-[var(--color-danger)] group-hover:opacity-100"
                title="Dersi sil (bir daha enjekte edilmez)"
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
