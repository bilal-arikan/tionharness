import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Button, ModalOverlay } from '@/shared/components'
import {
  drawStrokes,
  exportAnnotation,
  type AnnotatorExport,
  type DrawableSource,
} from './imageAnnotatorExport'
import {
  IMAGE_ANNOTATOR_ERROR_MESSAGES,
  type ImageAnnotatorErrorCode,
} from './imageAnnotatorLimits'
import {
  appendPoint,
  beginStroke,
  clear,
  createDrawingModel,
  finalizeStroke,
  redo,
  snapshot,
  snapshotsEqual,
  undo,
  type DrawingModel,
  type StrokePoint,
} from './imageAnnotatorModel'

interface Props {
  source: DrawableSource
  initialStrokes?: DrawingModel['strokes']
  onSave(result: AnnotatorExport & { revision: number }): Promise<void>
  onClose(): void
}

const MAX_PREVIEW_PIXELS = 8_000_000

export function ImageAnnotator({ source, initialStrokes = [], onSave, onClose }: Props) {
  const { t } = useTranslation('common')
  const defaultPenWidth = Math.max(2, source.width / 300)
  const [model, setModel] = useState(() => createDrawingModel(initialStrokes))
  const [penWidth, setPenWidth] = useState(defaultPenWidth)
  const modelRef = useRef(model)
  const [baseline, setBaseline] = useState(() => snapshot(createDrawingModel(initialStrokes)))
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)
  const mountedRef = useRef(true)
  const saveGenerationRef = useRef(0)
  const canvasRef = useRef<HTMLCanvasElement>(null)
  const dialogRef = useRef<HTMLElement>(null)
  const frameRef = useRef<number | null>(null)
  const pendingPointRef = useRef<StrokePoint | null>(null)
  const activePointerRef = useRef<number | null>(null)
  const dirty = useMemo(() => !snapshotsEqual(snapshot(model), baseline), [baseline, model])

  useEffect(() => {
    mountedRef.current = true
    const previouslyFocused = document.activeElement as HTMLElement | null
    canvasRef.current?.focus()
    return () => {
      mountedRef.current = false
      saveGenerationRef.current += 1
      if (previouslyFocused?.isConnected) previouslyFocused.focus()
    }
  }, [])

  const apply = useCallback((next: DrawingModel, warning?: ImageAnnotatorErrorCode) => {
    modelRef.current = next
    setModel(next)
    if (warning) setError(IMAGE_ANNOTATOR_ERROR_MESSAGES[warning])
  }, [])

  const render = useCallback(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const rect = canvas.getBoundingClientRect()
    const deviceRatio = Math.min(3, Math.max(1, window.devicePixelRatio || 1))
    const pixelRatio = Math.sqrt(MAX_PREVIEW_PIXELS / Math.max(1, rect.width * rect.height))
    const ratio = Math.min(deviceRatio, pixelRatio)
    const width = Math.max(1, Math.round(rect.width * ratio))
    const height = Math.max(1, Math.round(rect.height * ratio))
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width
      canvas.height = height
    }
    const ctx = canvas.getContext('2d')
    if (!ctx) {
      setError(IMAGE_ANNOTATOR_ERROR_MESSAGES.EXPORT_FAILED)
      return
    }
    ctx.clearRect(0, 0, width, height)
    ctx.save()
    ctx.scale(width / source.width, height / source.height)
    source.draw(ctx)
    drawStrokes(ctx, model.active ? [...model.strokes, model.active] : model.strokes)
    ctx.restore()
  }, [model, source])

  useEffect(() => {
    render()
  }, [render])
  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return
    const observer = new ResizeObserver(render)
    observer.observe(canvas)
    return () => observer.disconnect()
  }, [render])
  useEffect(() => {
    const handleDprChange = () => {
      render()
      watchDpr()
    }
    let mediaQuery: MediaQueryList | null = null
    const watchDpr = () => {
      mediaQuery?.removeEventListener('change', handleDprChange)
      mediaQuery = window.matchMedia(`(resolution: ${window.devicePixelRatio || 1}dppx)`)
      mediaQuery.addEventListener('change', handleDprChange, { once: true })
    }
    watchDpr()
    window.addEventListener('resize', render)
    return () => {
      mediaQuery?.removeEventListener('change', handleDprChange)
      window.removeEventListener('resize', render)
    }
  }, [render])

  const finish = useCallback(() => {
    if (frameRef.current !== null) cancelAnimationFrame(frameRef.current)
    frameRef.current = null
    pendingPointRef.current = null
    activePointerRef.current = null
    const result = finalizeStroke(modelRef.current)
    apply(result.model, result.warning)
  }, [apply])

  useEffect(() => {
    const blur = () => finish()
    window.addEventListener('blur', blur)
    return () => {
      window.removeEventListener('blur', blur)
      finish()
    }
  }, [finish])

  const pointFromEvent = (event: React.PointerEvent<HTMLCanvasElement>): StrokePoint => {
    const rect = event.currentTarget.getBoundingClientRect()
    return {
      x: ((event.clientX - rect.left) / rect.width) * source.width,
      y: ((event.clientY - rect.top) / rect.height) * source.height,
      pressure: event.pressure || 0.5,
    }
  }

  const close = useCallback(() => {
    if (savingRef.current) return
    if (!dirty || window.confirm(t('imageAnnotator.closeQuestion'))) onClose()
  }, [dirty, onClose, t])

  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        close()
        return
      }
      if (!(event.ctrlKey || event.metaKey)) return
      if (event.key.toLowerCase() === 'z') {
        event.preventDefault()
        apply(event.shiftKey ? redo(modelRef.current) : undo(modelRef.current))
        return
      }
      if (event.key.toLowerCase() === 'y') {
        event.preventDefault()
        apply(redo(modelRef.current))
      }
    }
    window.addEventListener('keydown', keydown)
    return () => window.removeEventListener('keydown', keydown)
  }, [apply, close])

  const save = async () => {
    if (savingRef.current || !dirty) return
    savingRef.current = true
    const generation = ++saveGenerationRef.current
    const savingSnapshot = snapshot(modelRef.current)
    const savingRevision = modelRef.current.revision
    setSaving(true)
    setError(null)
    try {
      const exported = await exportAnnotation(source, savingSnapshot.strokes)
      await onSave({ ...exported, revision: savingRevision })
      if (mountedRef.current && generation === saveGenerationRef.current)
        setBaseline(savingSnapshot)
    } catch (reason) {
      if (mountedRef.current && generation === saveGenerationRef.current)
        setError(
          reason instanceof Error ? reason.message : IMAGE_ANNOTATOR_ERROR_MESSAGES.EXPORT_FAILED,
        )
    } finally {
      if (mountedRef.current && generation === saveGenerationRef.current) {
        savingRef.current = false
        setSaving(false)
      }
    }
  }

  return createPortal(
    <ModalOverlay onClose={close} closeOnEscape={false} padding="p-3">
      <section
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={t('imageAnnotator.dialogLabel')}
        aria-describedby="image-annotator-help"
        aria-busy={saving}
        onKeyDown={(event) => {
          if (event.key !== 'Tab') return
          const focusable = dialogRef.current?.querySelectorAll<HTMLElement>(
            'button:not([disabled]), input:not([disabled]), canvas[tabindex="0"]',
          )
          if (!focusable?.length) return
          const first = focusable[0]
          const last = focusable[focusable.length - 1]
          if (event.shiftKey && document.activeElement === first) {
            event.preventDefault()
            last.focus()
          } else if (!event.shiftKey && document.activeElement === last) {
            event.preventDefault()
            first.focus()
          }
        }}
        className="flex h-[min(90vh,900px)] w-[min(96vw,1200px)] flex-col overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--bg-primary)]"
      >
        <header className="flex flex-wrap items-center gap-2 border-b border-[var(--border)] p-3">
          <Button
            aria-label={t('imageAnnotator.undo')}
            disabled={!model.undo.length || !!model.active}
            onClick={() => apply(undo(modelRef.current))}
          >
            {t('imageAnnotator.undo')}
          </Button>
          <Button
            aria-label={t('imageAnnotator.redo')}
            disabled={!model.redo.length || !!model.active}
            onClick={() => apply(redo(modelRef.current))}
          >
            {t('imageAnnotator.redo')}
          </Button>
          <Button
            aria-label={t('imageAnnotator.clear')}
            disabled={!model.strokes.length || !!model.active}
            onClick={() => apply(clear(modelRef.current))}
          >
            {t('imageAnnotator.clear')}
          </Button>
          <label className="flex items-center gap-2 text-sm" htmlFor="image-annotator-pen-width">
            {t('imageAnnotator.penSize')}
            <input
              id="image-annotator-pen-width"
              aria-label={t('imageAnnotator.penSize')}
              type="range"
              min={Math.max(1, defaultPenWidth / 2)}
              max={Math.max(12, defaultPenWidth * 4)}
              step={Math.max(0.5, defaultPenWidth / 4)}
              value={penWidth}
              onChange={(event) => setPenWidth(Number(event.currentTarget.value))}
              aria-valuetext={`${penWidth}px`}
            />
          </label>
          <span className="flex-1" />
          <Button aria-label={t('imageAnnotator.close')} onClick={close}>
            {t('imageAnnotator.cancel')}
          </Button>
          <Button
            aria-label={t('imageAnnotator.save')}
            disabled={!dirty || saving}
            onClick={() => void save()}
          >
            {saving ? t('common.saving') : t('common.save')}
          </Button>
        </header>
        {error && (
          <p role="alert" className="m-2 rounded bg-red-500/10 px-3 py-2 text-sm text-red-500">
            {error}
          </p>
        )}
        <p id="image-annotator-help" className="sr-only">
          {t('imageAnnotator.keyboardHelp')}
        </p>
        <div className="min-h-0 flex-1 bg-transparent p-2" data-testid="drawing-viewport">
          <canvas
            ref={canvasRef}
            tabIndex={0}
            aria-label={t('imageAnnotator.canvas')}
            className="mx-auto block h-full max-w-full cursor-crosshair outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
            style={{ touchAction: 'none', aspectRatio: `${source.width} / ${source.height}` }}
            onPointerDown={(event) => {
              if (!event.isPrimary || event.button !== 0 || activePointerRef.current !== null)
                return
              try {
                event.currentTarget.setPointerCapture(event.pointerId)
              } catch {
                setError(t('imageAnnotator.pointerCaptureError'))
                return
              }
              activePointerRef.current = event.pointerId
              const result = beginStroke(
                modelRef.current,
                pointFromEvent(event),
                '#ef4444',
                penWidth,
              )
              apply(result.model, result.warning)
              if (!result.model.active) activePointerRef.current = null
            }}
            onPointerMove={(event) => {
              if (
                !event.isPrimary ||
                activePointerRef.current !== event.pointerId ||
                !modelRef.current.active
              )
                return
              pendingPointRef.current = pointFromEvent(event)
              if (frameRef.current !== null) return
              frameRef.current = requestAnimationFrame(() => {
                frameRef.current = null
                const point = pendingPointRef.current
                pendingPointRef.current = null
                if (!point) return
                const result = appendPoint(modelRef.current, point)
                apply(result.model, result.warning)
                if (!result.model.active) activePointerRef.current = null
              })
            }}
            onPointerUp={(event) => {
              if (activePointerRef.current === event.pointerId) finish()
            }}
            onPointerCancel={(event) => {
              if (activePointerRef.current === event.pointerId) finish()
            }}
            onLostPointerCapture={(event) => {
              if (activePointerRef.current === event.pointerId) finish()
            }}
          />
        </div>
      </section>
    </ModalOverlay>,
    document.body,
  )
}
