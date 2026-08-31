import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import type { Artifact } from '@/types'
import type { DrawableSource } from '@/features/image-annotator/imageAnnotatorExport'
import { validateImageSource } from '@/features/image-annotator/imageAnnotatorLimits'
import { readImageResponse } from './imageArtifactSource'

// useImageArtifactSource fetches an image artifact's bytes and decodes them into
// a DrawableSource the ImageAnnotator can paint. The bitmap is owned here: it is
// closed when a new source replaces it, when the caller closes the editor, and
// on unmount, so a 40 MP decode never outlives the editor.
export function useImageArtifactSource(onError: (msg: string) => void) {
  const { t } = useTranslation('common')
  const [source, setSource] = useState<DrawableSource | null>(null)
  const [opening, setOpening] = useState(false)
  // `generation` invalidates in-flight loads: every open/close bumps it, so a
  // late response can neither overwrite a newer source nor leak its bitmap.
  const generationRef = useRef(0)
  const abortRef = useRef<AbortController | null>(null)
  const bitmapRef = useRef<ImageBitmap | null>(null)
  const mountedRef = useRef(true)

  const closeBitmap = () => {
    const bitmap = bitmapRef.current
    bitmapRef.current = null
    bitmap?.close()
  }

  useEffect(() => {
    mountedRef.current = true
    return () => {
      mountedRef.current = false
      generationRef.current += 1
      abortRef.current?.abort()
      abortRef.current = null
      closeBitmap()
    }
  }, [])

  const open = async (artifact: Artifact) => {
    if (artifact.kind !== 'image' || !artifact.sourcePath) return
    abortRef.current?.abort()
    const controller = new AbortController()
    abortRef.current = controller
    const generation = ++generationRef.current
    setOpening(true)
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
        throw new Error(t('imageAnnotator.decodeError'))
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
      if (generation !== generationRef.current) {
        bitmap.close()
        bitmap = null
        return
      }
      closeBitmap()
      bitmapRef.current = bitmap
      const drawableBitmap = bitmap
      setSource({
        width: bitmap.width,
        height: bitmap.height,
        opaque: mime === 'image/jpeg',
        draw: (ctx) => ctx.drawImage(drawableBitmap, 0, 0),
      })
      bitmap = null
    } catch (error) {
      bitmap?.close()
      if (generation === generationRef.current && !controller.signal.aborted)
        onError((error as Error).message)
    } finally {
      if (generation === generationRef.current) {
        abortRef.current = null
        setOpening(false)
      }
    }
  }

  const close = () => {
    generationRef.current += 1
    closeBitmap()
    setSource(null)
  }

  // isStale reports whether the editor was closed (or the component unmounted)
  // while an async save was running — the caller uses it to roll back instead of
  // writing into a dead editor.
  const isStale = (generation: number) =>
    !mountedRef.current || generation !== generationRef.current

  return { source, opening, open, close, isStale, generation: () => generationRef.current }
}
