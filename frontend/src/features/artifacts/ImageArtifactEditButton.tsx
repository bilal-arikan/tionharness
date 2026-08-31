import { useTranslation } from 'react-i18next'
import { Loader2, Pencil } from 'lucide-react'
import { api } from '@/api'
import type { Artifact } from '@/types'
import { ImageAnnotator } from '@/features/image-annotator/ImageAnnotator'
import { useImageArtifactSource } from './useImageArtifactSource'

interface Props {
  artifact: Artifact
  // Called with the persisted artifact after its source file was replaced.
  onUpdated: (artifact: Artifact) => void
  onError: (msg: string) => void
  className?: string
}

// ImageArtifactEditButton opens the ImageAnnotator on an image artifact and, on
// save, OVERWRITES that same artifact: the annotated bitmap is uploaded and the
// artifact is repointed at it via PUT /api/artifacts/{id} (sourcePath). The
// backend drops the now-unreferenced old file — no derivative artifact is
// created. Renders nothing for non-image artifacts or ones without a source file.
export function ImageArtifactEditButton({ artifact, onUpdated, onError, className }: Props) {
  const { t } = useTranslation('common')
  const editor = useImageArtifactSource(onError)

  if (artifact.kind !== 'image' || !artifact.sourcePath) return null

  const save = async ({ blob, mime }: { blob: Blob; mime: 'image/png' | 'image/webp' }) => {
    const generation = editor.generation()
    const extension = mime === 'image/webp' ? 'webp' : 'png'
    // Sessionless (manually imported) artifacts share the "_shared" bucket, the
    // same convention the drag-and-drop import uses.
    const upload = await api.uploadFile(
      artifact.sessionId || '_shared',
      new File([blob], `${artifact.id}-edited.${extension}`, { type: mime }),
    )
    if (!upload.relPath) throw new Error(t('artifactAnnotation.stagingPathError'))
    if (editor.isStale(generation)) {
      await api.deleteFile(upload.relPath)
      return
    }
    let updated: Artifact
    try {
      updated = await api.updateArtifact(artifact.id, { sourcePath: upload.relPath })
    } catch (updateError) {
      // The artifact still points at its old file, so the just-staged upload is
      // orphaned — remove it, and surface both failures if that also fails.
      try {
        await api.deleteFile(upload.relPath)
      } catch (cleanupError) {
        throw new AggregateError(
          [updateError, cleanupError],
          t('artifactAnnotation.overwriteAndCleanupError'),
          { cause: cleanupError },
        )
      }
      throw updateError
    }
    if (editor.isStale(generation)) return
    editor.close()
    onUpdated(updated)
  }

  return (
    <>
      <button
        data-testid="artifact-image-edit"
        onClick={() => void editor.open(artifact)}
        disabled={editor.opening}
        title={t('artifactAnnotation.editTitle')}
        className={className}
      >
        {editor.opening ? <Loader2 size={14} className="animate-spin" /> : <Pencil size={14} />}
        <span>{t('artifactAnnotation.edit')}</span>
      </button>
      {editor.source && (
        <ImageAnnotator source={editor.source} onClose={editor.close} onSave={save} />
      )}
    </>
  )
}
