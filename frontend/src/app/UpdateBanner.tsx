// A dismissible strip shown at the top of the app when the release feed reports
// a newer version. It links to the release notes and NOTHING else: this version
// deliberately offers no download or self-update action, so replacing the binary
// stays an explicit, user-driven step.
import { useEffect, useState } from 'react'
import { ArrowUpCircle, ExternalLink, X } from 'lucide-react'
import { api } from '@/api'
import type { UpdateStatus } from '@/types'
import { readDismissedVersion, shouldShowUpdate, writeDismissedVersion } from './updateCheck'

export function UpdateBanner() {
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [dismissed, setDismissed] = useState(() => readDismissedVersion())

  useEffect(() => {
    let alive = true
    // The check is advisory and cached server-side; a failure here means the
    // banner simply never appears, which is the intended quiet fallback.
    api
      .getUpdateStatus()
      .then((s) => {
        if (alive) setStatus(s)
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  if (!shouldShowUpdate(status, dismissed)) return null

  return (
    <div
      role="status"
      className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-accent-soft)] px-3 py-1.5 text-sm md:px-6"
    >
      <ArrowUpCircle size={15} className="shrink-0 text-[var(--color-accent)]" />
      <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
        <span className="font-medium">Yeni sürüm mevcut: {status.latest}</span>
        <span className="text-[var(--color-text-dim)]"> · yüklü sürüm {status.current}</span>
      </span>
      {status.notesUrl && (
        <a
          href={status.notesUrl}
          target="_blank"
          rel="noreferrer"
          className="flex shrink-0 items-center gap-1 rounded-lg px-2 py-1 text-[var(--color-accent)] transition hover:bg-[var(--color-surface-2)]"
        >
          <ExternalLink size={13} />
          Sürüm notları
        </a>
      )}
      <button
        onClick={() => {
          writeDismissedVersion(status.latest)
          setDismissed(status.latest)
        }}
        aria-label="Sürüm bildirimini kapat"
        title="Kapat"
        className="flex h-6 w-6 shrink-0 items-center justify-center rounded-lg text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
      >
        <X size={14} />
      </button>
    </div>
  )
}
