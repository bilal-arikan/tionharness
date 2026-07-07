import { useEffect, useState } from 'react'
import { X, ExternalLink, Loader2 } from 'lucide-react'
import { api } from '@/api'
import type { Artifact } from '@/types'
import { ArtifactView } from './ArtifactView'
import { KIND_ICON, KIND_LABEL, OriginBadge } from './artifactMeta'

interface Props {
  artifactId: string
  onClose: () => void
  // Jump to the full Artifacts screen with this artifact selected.
  onOpenFull?: (id: string) => void
  onError?: (msg: string) => void
}

// ArtifactPreviewModal shows a single artifact in a centered overlay so clicking
// an artifact chip/card in chat (or anywhere) previews its content WITHOUT
// navigating to the Artifacts screen. It fetches the full artifact by id and
// renders it via the shared ArtifactView. Escape / backdrop click closes it; the
// header offers a shortcut to open the dedicated Artifacts screen for editing.
export function ArtifactPreviewModal({ artifactId, onClose, onOpenFull, onError }: Props) {
  const [artifact, setArtifact] = useState<Artifact | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setArtifact(null)
    api
      .getArtifact(artifactId)
      .then((a) => {
        if (!cancelled) setArtifact(a)
      })
      .catch((e) => {
        if (!cancelled) onError?.((e as Error).message)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [artifactId, onError])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const Icon = artifact ? KIND_ICON[artifact.kind] : null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm max-md:items-end max-md:p-0 max-md:[&>*]:!w-full max-md:[&>*]:!max-w-none max-md:[&>*]:!max-h-[92dvh] max-md:[&>*]:!rounded-b-none"
      onClick={onClose}
    >
      <div
        className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between gap-3 border-b border-[var(--color-border)] px-4 py-3">
          <div className="flex min-w-0 items-center gap-2">
            {Icon && <Icon size={16} className="shrink-0 text-[var(--color-accent)]" />}
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold">
                {artifact ? artifact.title : 'Yükleniyor…'}
              </div>
              {artifact && (
                <div className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                  <OriginBadge origin={artifact.origin} />
                  <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5">
                    {KIND_LABEL[artifact.kind] ?? artifact.kind}
                    {artifact.language ? ` · ${artifact.language}` : ''}
                  </span>
                </div>
              )}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {onOpenFull && (
              <button
                onClick={() => onOpenFull(artifactId)}
                title="Artifactlar ekranında aç"
                className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
              >
                <ExternalLink size={14} />
                <span className="hidden sm:inline">Ekranda aç</span>
              </button>
            )}
            <button
              onClick={onClose}
              title="Kapat (Esc)"
              className="flex h-7 w-7 items-center justify-center rounded-md text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <X size={16} />
            </button>
          </div>
        </div>

        {/* Body */}
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {loading ? (
            <div className="flex items-center justify-center gap-2 py-12 text-sm text-[var(--color-text-dim)]">
              <Loader2 size={16} className="animate-spin" /> Yükleniyor…
            </div>
          ) : artifact ? (
            <ArtifactView
              kind={artifact.kind}
              language={artifact.language}
              content={artifact.content}
              sourcePath={artifact.sourcePath}
            />
          ) : (
            <div className="py-12 text-center text-sm text-[var(--color-text-dim)]">
              Artifact yüklenemedi.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
