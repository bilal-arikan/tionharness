import { useState, useEffect } from 'react'
import type { VersionInfo } from '@/types'
import { api } from '@/api'
import { formatDate } from '@/shared/lib/intl'

export function AboutPanel() {
  const [info, setInfo] = useState<VersionInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api
      .getVersion()
      .then(setInfo)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const isDev = !info || info.version === 'dev'

  return (
    <div className="space-y-5 text-sm">
      {/* App identity */}
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-[var(--color-accent)] text-xl font-bold text-white select-none">
          S
        </div>
        <div>
          <div className="text-base font-semibold text-[var(--color-text)]">TionHarness</div>
          <div className="text-[var(--color-text-dim)]">Çok-ajanlı AI runtime</div>
        </div>
      </div>

      {/* Version table */}
      <div className="rounded-lg border border-[var(--color-border)] divide-y divide-[var(--color-border)]">
        <AboutRow label="Sürüm">
          {loading ? (
            <span className="text-[var(--color-text-dim)]">yükleniyor…</span>
          ) : error ? (
            <span className="text-[var(--color-danger)]">Alınamadı</span>
          ) : isDev ? (
            <span className="rounded bg-[var(--color-warning)]/15 px-1.5 py-0.5 text-[var(--color-warning)] font-mono text-xs">
              dev build
            </span>
          ) : (
            <span className="font-mono">{info!.version}</span>
          )}
        </AboutRow>

        {info && info.commit !== 'dev' && (
          <AboutRow label="Commit">
            <span className="font-mono text-xs">{info.commit}</span>
          </AboutRow>
        )}

        {info && info.buildDate !== 'dev' && (
          <AboutRow label="Build tarihi">
            <span>{formatDate(new Date(info.buildDate), { dateStyle: 'medium' })}</span>
          </AboutRow>
        )}

        <AboutRow label="Go sürümü">
          {loading ? (
            <span className="text-[var(--color-text-dim)]">—</span>
          ) : (
            <span className="font-mono text-xs">{info?.goVersion ?? '—'}</span>
          )}
        </AboutRow>
      </div>

      {/* Güncelleme notu */}
      <div className="rounded-lg border border-[var(--color-border)] px-3 py-2.5 text-[var(--color-text-dim)]">
        <span className="mr-1.5 text-[var(--color-warning)]">⚠</span>
        Otomatik güncelleme kontrolü henüz desteklenmiyor. Yeni sürümler için projeyi manuel olarak
        kontrol edin.
      </div>

      {/* Depolama açıklaması */}
      <p className="text-[var(--color-text-dim)] leading-relaxed">
        Uygulama ayarları{' '}
        <code className="rounded bg-[var(--color-surface-2)] px-1">settings.json</code>, workspace
        ayarları her workspace&apos;in{' '}
        <code className="rounded bg-[var(--color-surface-2)] px-1">ws-settings.json</code>{' '}
        dosyasında saklanır. Veritabanı kullanılmaz; tüm veriler düz dosyalardır.
      </p>
    </div>
  )
}

function AboutRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-4 px-3 py-2">
      <span className="w-28 shrink-0 text-[var(--color-text-dim)]">{label}</span>
      <span className="text-[var(--color-text)]">{children}</span>
    </div>
  )
}
