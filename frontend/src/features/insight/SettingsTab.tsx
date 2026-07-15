import { useState } from 'react'
import { Save } from 'lucide-react'
import { api } from '@/api'
import type { InsightSettings } from '@/types'

interface Props {
  settings: InsightSettings
  setSettings: (s: InsightSettings) => void
  onError: (msg: string) => void
}

const inputCls = 'mt-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm'

export function SettingsTab({ settings, setSettings, onError }: Props) {
  const [saving, setSaving] = useState(false)

  const save = async () => {
    setSaving(true)
    try {
      setSettings(await api.updateInsightSettings(settings))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="max-w-xl space-y-3 rounded-md border border-[var(--color-border)] p-3">
      <label className="block">
        <span className="text-sm">App-Fix repo yolu (backlog hedefi)</span>
        <input
          type="text"
          value={settings.appFixRepoPath ?? ''}
          onChange={(e) => setSettings({ ...settings, appFixRepoPath: e.target.value })}
          placeholder="C:/Users/.../TionSwarm"
          className={`${inputCls} w-full`}
        />
      </label>
      <label className="block">
        <span className="text-sm">Maks. oturum / tarama (0 = sınırsız)</span>
        <input
          type="number"
          value={settings.maxSessions ?? 0}
          onChange={(e) => setSettings({ ...settings, maxSessions: Number(e.target.value) })}
          className={`${inputCls} w-32`}
        />
      </label>
      <label className="block">
        <span className="text-sm">Maks. analiz (LLM çağrısı) / tarama (0 = sınırsız)</span>
        <input
          type="number"
          value={settings.maxAnalyzed ?? 0}
          onChange={(e) => setSettings({ ...settings, maxAnalyzed: Number(e.target.value) })}
          className={`${inputCls} w-32`}
        />
        <span className="mt-1 block text-xs text-[var(--color-text-muted)]">
          Maliyet tavanı — aşan çiftler sonraki taramada işlenir.
        </span>
      </label>
      <label className="block">
        <span className="text-sm">Otomatik tarama cron (boş = kapalı)</span>
        <input
          type="text"
          value={settings.autoScanCron ?? ''}
          onChange={(e) => setSettings({ ...settings, autoScanCron: e.target.value })}
          placeholder="0 3 * * *  (her gece 03:00)"
          className={`${inputCls} w-full font-mono`}
        />
        <span className="mt-1 block text-xs text-[var(--color-text-muted)]">
          Standart 5 alanlı cron (dakika saat gün ay haftagünü).
        </span>
      </label>
      <button
        onClick={save}
        disabled={saving}
        className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-white disabled:opacity-50"
      >
        <Save className="h-4 w-4" /> {saving ? 'Kaydediliyor…' : 'Kaydet'}
      </button>
    </div>
  )
}
