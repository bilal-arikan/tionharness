export const ACCEPTED_IMAGE_MIMES = ['image/png', 'image/jpeg', 'image/webp'] as const
export const MAX_SOURCE_BYTES = 20_971_520
export const MAX_SOURCE_DIMENSION = 8192
export const MAX_SOURCE_PIXELS = 40_000_000
export const MAX_POINTS_PER_STROKE = 20_000
export const MAX_TOTAL_POINTS = 200_000
export const MAX_STROKES = 500
export const MAX_HISTORY_ENTRIES = 100
export const MAX_MODEL_BYTES = 33_554_432
export const POINT_MODEL_BYTES = 24
export const STROKE_MODEL_BYTES = 128

export type ImageAnnotatorErrorCode =
  | 'IMAGE_UNSUPPORTED_TYPE'
  | 'IMAGE_TOO_LARGE_BYTES'
  | 'IMAGE_DIMENSION_EXCEEDED'
  | 'IMAGE_PIXEL_LIMIT_EXCEEDED'
  | 'IMAGE_HEADER_INVALID'
  | 'IMAGE_DECODE_FAILED'
  | 'STROKE_POINT_LIMIT'
  | 'DRAWING_POINT_LIMIT'
  | 'DRAWING_STROKE_LIMIT'
  | 'HISTORY_LIMIT'
  | 'DRAWING_MEMORY_LIMIT'
  | 'EXPORT_FAILED'

export const IMAGE_ANNOTATOR_ERROR_MESSAGES: Record<ImageAnnotatorErrorCode, string> = {
  IMAGE_UNSUPPORTED_TYPE: 'Bu görsel türü desteklenmiyor. PNG, JPEG veya WebP seçin.',
  IMAGE_TOO_LARGE_BYTES: 'Görsel 20 MB sınırını aşıyor.',
  IMAGE_DIMENSION_EXCEEDED: 'Görsel boyutu 8192 px sınırını aşıyor.',
  IMAGE_PIXEL_LIMIT_EXCEEDED: 'Görsel 40 megapiksel sınırını aşıyor.',
  IMAGE_HEADER_INVALID: 'Görsel başlığı geçersiz veya dosya türüyle uyuşmuyor.',
  IMAGE_DECODE_FAILED: 'Görsel açılamadı; dosya bozuk olabilir.',
  STROKE_POINT_LIMIT: 'Tek çizgi 20.000 nokta sınırına ulaştı.',
  DRAWING_POINT_LIMIT: 'Çizim 200.000 nokta sınırına ulaştı.',
  DRAWING_STROKE_LIMIT: 'Çizim 500 çizgi sınırına ulaştı.',
  HISTORY_LIMIT: 'En eski geri alma adımları kaldırıldı.',
  DRAWING_MEMORY_LIMIT: 'Çizim bellek sınırına ulaştı.',
  EXPORT_FAILED: 'Görsel dışa aktarılamadı.',
}

export class ImageAnnotatorError extends Error {
  readonly code: ImageAnnotatorErrorCode

  constructor(code: ImageAnnotatorErrorCode) {
    super(IMAGE_ANNOTATOR_ERROR_MESSAGES[code])
    this.name = 'ImageAnnotatorError'
    this.code = code
  }
}

export function validateImageHeader(mime: string, bytes: Uint8Array): void {
  if (!ACCEPTED_IMAGE_MIMES.includes(mime as (typeof ACCEPTED_IMAGE_MIMES)[number])) {
    throw new ImageAnnotatorError('IMAGE_UNSUPPORTED_TYPE')
  }
  const png =
    bytes.length >= 8 && [137, 80, 78, 71, 13, 10, 26, 10].every((value, i) => bytes[i] === value)
  const jpeg = bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff
  const webp =
    bytes.length >= 12 &&
    String.fromCharCode(...bytes.slice(0, 4)) === 'RIFF' &&
    String.fromCharCode(...bytes.slice(8, 12)) === 'WEBP'
  if (!(
    (mime === 'image/png' && png) ||
    (mime === 'image/jpeg' && jpeg) ||
    (mime === 'image/webp' && webp)
  )) {
    throw new ImageAnnotatorError('IMAGE_HEADER_INVALID')
  }
}

export function validateSourceLimits(byteLength: number, width: number, height: number): void {
  if (byteLength > MAX_SOURCE_BYTES) throw new ImageAnnotatorError('IMAGE_TOO_LARGE_BYTES')
  if (!(width > 0 && height > 0)) throw new ImageAnnotatorError('IMAGE_DECODE_FAILED')
  if (width > MAX_SOURCE_DIMENSION || height > MAX_SOURCE_DIMENSION)
    throw new ImageAnnotatorError('IMAGE_DIMENSION_EXCEEDED')
  if (width * height > MAX_SOURCE_PIXELS)
    throw new ImageAnnotatorError('IMAGE_PIXEL_LIMIT_EXCEEDED')
}

export function validateImageSource(
  mime: string,
  bytes: Uint8Array,
  width: number,
  height: number,
  declaredByteLength = bytes.byteLength,
): void {
  if (declaredByteLength > MAX_SOURCE_BYTES || bytes.byteLength > MAX_SOURCE_BYTES)
    throw new ImageAnnotatorError('IMAGE_TOO_LARGE_BYTES')
  validateImageHeader(mime, bytes)
  validateSourceLimits(bytes.byteLength, width, height)
}
