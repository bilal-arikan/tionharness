import { useEffect, useRef, useState } from 'react'
import { X, ExternalLink, Loader2, Pencil } from 'lucide-react'
import { api } from '@/api'
import type { Artifact } from '@/types'
import { ModalOverlay } from '@/shared/components'
import { ArtifactView } from './ArtifactView'
import { KIND_ICON, KIND_LABEL } from './artifactMeta'
import { OriginBadge } from './OriginBadge'
import { ImageAnnotator } from '@/features/image-annotator/ImageAnnotator'
import type { DrawableSource } from '@/features/image-annotator/imageAnnotatorExport'
import { validateImageSource } from '@/features/image-annotator/imageAnnotatorLimits'
import { readImageResponse } from './imageArtifactSource'

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
  const [annotatorSource, setAnnotatorSource] = useState<DrawableSource | null>(null)
  const [openingEditor, setOpeningEditor] = useState(false)
  const editorGenerationRef = useRef(0)
  const editorAbortRef = useRef<AbortController | null>(null)
  const bitmapRef = useRef<ImageBitmap | null>(null)

  const closeBitmap = () => {
    const bitmap = bitmapRef.current
    bitmapRef.current = null
    bitmap?.close()
  }

  useEffect(
    () => () => {
      editorGenerationRef.current += 1
      editorAbortRef.current?.abort()
      editorAbortRef.current = null
      closeBitmap()
    },
    [],
  )

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

  const Icon = artifact ? KIND_ICON[artifact.kind] : null

  const openEditor = async () => {
    if (!artifact || artifact.kind !== 'image' || !artifact.sourcePath) return
    editorAbortRef.current?.abort()
    const controller = new AbortController()
    editorAbortRef.current = controller
    const generation = ++editorGenerationRef.current
    setOpeningEditor(true)
    let bitmap: ImageBitmap | null = null
    try {
      const response = await api.getArtifactSource(artifact.id, controller.signal)
      const declaredLength = Number(response.headers.get('Content-Length') || '0')
      const bytes = await readImageResponse(response)
      const mime = response.headers.get('Content-Type')?.split(';')[0].trim() || ''
      const imageBuffer = bytes.buffer.slice(
        bytes.byteOffset,
        bytes.byteOffset + bytes.byteLength,
      ) as ArrayBuffer
      const blob = new Blob([imageBuffer], { type: mime })
      try {
        bitmap = await createImageBitmap(blob)
      } catch {
        throw new Error('Görsel açılamadı; dosya bozuk olabilir.')
      }
      try {
        validateImageSource(
          mime,
          bytes,
          bitmap.width,
          bitmap.height,
          declaredLength || bytes.length,
        )
      } catch (error) {
        bitmap.close()
        bitmap = null
        throw error
      }
      if (generation !== editorGenerationRef.current) {
        bitmap.close()
        bitmap = null
        return
      }
      closeBitmap()
      bitmapRef.current = bitmap
      const drawableBitmap = bitmap
      setAnnotatorSource({
        width: bitmap.width,
        height: bitmap.height,
        opaque: mime === 'image/jpeg',
        draw: (ctx) => ctx.drawImage(drawableBitmap, 0, 0),
      })
      bitmap = null
    } catch (error) {
      bitmap?.close()
      if (generation === editorGenerationRef.current && !controller.signal.aborted)
        onError?.((error as Error).message)
    } finally {
      if (generation === editorGenerationRef.current) {
        editorAbortRef.current = null
        setOpeningEditor(false)
      }
    }
  }

  const closeEditor = () => {
    closeBitmap()
    setAnnotatorSource(null)
  }

  return (
    <ModalOverlay onClose={onClose} className="backdrop-blur-sm">
      <div className="flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]">
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
            {artifact?.kind === 'image' && artifact.sourcePath && (
              <button
                onClick={() => void openEditor()}
                disabled={openingEditor}
                title="Orijinali koruyarak üzerine çiz"
                className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-50"
              >
                {openingEditor ? (
                  <Loader2 size={14} className="animate-spin" />
                ) : (
                  <Pencil size={14} />
                )}
                <span className="hidden sm:inline">Üzerine çiz</span>
              </button>
            )}
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
      {artifact && annotatorSource && (
        <ImageAnnotator
          source={annotatorSource}
          onClose={closeEditor}
          onSave={async ({ blob, mime }) => {
            const extension = mime === 'image/webp' ? 'webp' : 'png'
            const upload = await api.uploadFile(
              artifact.sessionId || '_shared',
              new File([blob], `${artifact.id}-derived.${extension}`, { type: mime }),
            )
            const derived = await api.createArtifact({
              title: `${artifact.title} — Düzenleme`,
              kind: 'image',
              sessionId: artifact.sessionId,
              sourcePath: upload.relPath,
              origin: 'manual',
              derivedFromArtifactId: artifact.id,
            })
            closeEditor()
            onOpenFull?.(derived.id)
          }}
        />
      )}
    </ModalOverlay>
  )
}
