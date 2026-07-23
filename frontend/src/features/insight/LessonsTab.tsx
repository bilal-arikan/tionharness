import { useEffect, useState } from 'react'
import { api } from '@/api'
import { LessonsList } from '@/features/settings/LessonsList'

interface Props {
  onError: (msg: string) => void
}

// LessonsTab surfaces the REACTIVE side of self-improvement inside the Insight
// cockpit: the "learn from failures" toggle (mirrors the Settings ▸ Context one —
// same lessonReflect setting) plus the workspace's stored lessons. Insight's own
// retrospective scanning and this per-turn lesson memory share the same
// db.Lesson store, so viewing/configuring both here keeps all learning in one
// place. The lessons LIST is reused verbatim from Settings.
export function LessonsTab({ onError }: Props) {
  const [reflect, setReflect] = useState<boolean | null>(null)

  useEffect(() => {
    api
      .getSettings()
      .then((s) => setReflect(s.lessonReflect))
      .catch((e) => onError((e as Error).message))
  }, [onError])

  const toggle = async (v: boolean) => {
    const prev = reflect
    setReflect(v) // optimistic
    try {
      await api.updateSettings({ lessonReflect: v })
    } catch (e) {
      setReflect(prev ?? null)
      onError((e as Error).message)
    }
  }

  return (
    <div className="space-y-4">
      <label className="flex cursor-pointer items-start gap-3 rounded-md border border-[var(--color-border)] p-3">
        <input
          type="checkbox"
          className="mt-1"
          checked={reflect ?? false}
          disabled={reflect === null}
          onChange={(e) => toggle(e.target.checked)}
        />
        <span>
          <span className="text-sm font-medium">Hatalardan ders çıkar (lesson reflect)</span>
          <span className="mt-1 block text-xs text-[var(--color-text-muted)]">
            Kötü biten turdan arka planda kısa bir ders damıtılır (hata turu başına 1 ucuz çağrı) ve
            workspace-geneli lessons store'a yazılır; en yeni 5 ders her turun bağlamına otomatik
            enjekte edilir. Aynı hata şekli tekrarında mevcut ders güncellenir (yığılmaz). Bu ayar
            Ayarlar ▸ Bağlam ile aynıdır.
          </span>
        </span>
      </label>

      <LessonsList />
    </div>
  )
}
