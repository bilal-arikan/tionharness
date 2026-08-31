import type { Stroke } from './imageAnnotatorModel'
import { ImageAnnotatorError } from './imageAnnotatorLimits'

export interface DrawableSource {
  width: number
  height: number
  draw(ctx: CanvasRenderingContext2D): void
  opaque: boolean
}
export interface AnnotatorExport {
  blob: Blob
  mime: 'image/png' | 'image/webp'
  width: number
  height: number
}

export function drawStrokes(ctx: CanvasRenderingContext2D, strokes: Stroke[]): void {
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  for (const stroke of strokes) {
    if (!stroke.points.length) continue
    ctx.beginPath()
    ctx.strokeStyle = stroke.color
    ctx.lineWidth = stroke.width
    ctx.moveTo(stroke.points[0].x, stroke.points[0].y)
    if (stroke.points.length === 1) ctx.lineTo(stroke.points[0].x + 0.01, stroke.points[0].y)
    else
      for (let pointIndex = 1; pointIndex < stroke.points.length; pointIndex++) {
        const point = stroke.points[pointIndex]
        ctx.lineTo(point.x, point.y)
      }
    ctx.stroke()
  }
}

function encode(canvas: HTMLCanvasElement, mime: string): Promise<Blob | null> {
  return new Promise((resolve, reject) => {
    try {
      canvas.toBlob(resolve, mime)
    } catch (error) {
      reject(error)
    }
  })
}

export async function exportAnnotation(
  source: DrawableSource,
  strokes: Stroke[],
): Promise<AnnotatorExport> {
  const canvas = document.createElement('canvas')
  canvas.width = source.width
  canvas.height = source.height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new ImageAnnotatorError('EXPORT_FAILED')
  try {
    source.draw(ctx)
    drawStrokes(ctx, strokes)
    if (source.opaque) {
      try {
        const webp = await encode(canvas, 'image/webp')
        if (webp?.type === 'image/webp')
          return { blob: webp, mime: 'image/webp', width: source.width, height: source.height }
      } catch {
        /* exactly one PNG fallback below */
      }
    }
    try {
      const png = await encode(canvas, 'image/png')
      if (png?.type === 'image/png')
        return { blob: png, mime: 'image/png', width: source.width, height: source.height }
    } catch {
      /* converted to stable public error */
    }
    throw new ImageAnnotatorError('EXPORT_FAILED')
  } finally {
    // Release the potentially 40 MP backing store as soon as encoding settles.
    canvas.width = 0
    canvas.height = 0
  }
}
