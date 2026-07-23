import { useState } from 'react'
import { Save, Trash2 } from 'lucide-react'
import { api } from '@/api'
import type { InsightSettings } from '@/types'

interface Props {
  settings: InsightSettings
  setSettings: (s: InsightSettings) => void
  onError: (msg: string) => void
  // Called after a reset so the panel can refresh its (now-empty) findings.
  onReset: () => void
}

const inputCls = 'mt-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm'

export function SettingsTab({ settings, setSettings, onError, onReset }: Props) {
  const [saving, setSaving] = useState(false)
  const [deep, setDeep] = useState(false)
  const [resetting, setResetting] = useState(false)

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

  const reset = async () => {
    const msg = deep
      ? 'TÜM içgörü verisi + ledger silinecek. Sonraki tarama tüm oturumları sıfırdan tarar (düzeltilmiş sorunlar bile eski oturumlardan geri gelebilir). Emin misin?'
      : 'Tüm bulgular + tarama geçmişi + workspace-opt aksiyon dokümanı silinecek (ledger korunur → eski oturumlar yeniden taranmaz). Emin misin?'
    if (!confirm(msg)) return
    setResetting(true)
    try {
      await api.resetInsight(deep)
      onReset()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setResetting(false)
    }
  }

  return (
    <div className="max-w-xl space-y-4">
    <div className="space-y-3 rounded-md border border-[var(--color-border)] p-3">
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
        <span className="mt-1 block text-xs text-[var(--color-text-dim)]">
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
        <span className="mt-1 block text-xs text-[var(--color-text-dim)]">
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

    {/* Danger zone: reset all insight data for this workspace. */}
    <div className="space-y-2 rounded-md border border-[var(--color-danger)]/40 p-3">
      <div className="text-sm font-semibold text-[var(--color-danger)]">Tehlikeli bölge</div>
      <p className="text-xs text-[var(--color-text-dim)]">
        Bu workspace'in TÜM içgörü verisini sıfırlar (bulgular + tarama geçmişi + workspace-opt
        aksiyon dokümanı). Lensler ve ayarlar korunur. Her taramada benzer bulgular biriktiyse
        panoyu temizler.
      </p>
      <label className="flex items-center gap-2 text-xs">
        <input type="checkbox" checked={deep} onChange={(e) => setDeep(e.target.checked)} />
        Ledger'i de temizle — eski oturumlar sıfırdan yeniden taranır (düzeltilmiş sorunlar geri gelebilir)
      </label>
      <button
        onClick={reset}
        disabled={resetting}
        className="flex items-center gap-1 rounded-md border border-[var(--color-danger)] px-3 py-1 text-sm text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
      >
        <Trash2 className="h-4 w-4" /> {resetting ? 'Sıfırlanıyor…' : 'Tüm içgörüyü sıfırla'}
      </button>
    </div>
    </div>
  )
}
