import { i18next } from '@/i18n'

const ACCEPTED_IMAGE_MIMES = ['image/png', 'image/jpeg', 'image/webp'] as const
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

const ERROR_MESSAGE_KEYS: Record<ImageAnnotatorErrorCode, string> = {
  IMAGE_UNSUPPORTED_TYPE: 'unsupportedType',
  IMAGE_TOO_LARGE_BYTES: 'tooLargeBytes',
  IMAGE_DIMENSION_EXCEEDED: 'dimensionExceeded',
  IMAGE_PIXEL_LIMIT_EXCEEDED: 'pixelLimitExceeded',
  IMAGE_HEADER_INVALID: 'headerInvalid',
  IMAGE_DECODE_FAILED: 'decodeFailed',
  STROKE_POINT_LIMIT: 'strokePointLimit',
  DRAWING_POINT_LIMIT: 'drawingPointLimit',
  DRAWING_STROKE_LIMIT: 'drawingStrokeLimit',
  HISTORY_LIMIT: 'historyLimit',
  DRAWING_MEMORY_LIMIT: 'drawingMemoryLimit',
  EXPORT_FAILED: 'exportFailed',
}

export function imageAnnotatorErrorMessage(code: ImageAnnotatorErrorCode): string {
  return i18next.t(`errors.${ERROR_MESSAGE_KEYS[code]}`, { ns: 'image-annotator' })
}

// Preserve the exported lookup object while resolving each value lazily. These
// messages are used both by the modal and errors raised before the modal opens.
export const IMAGE_ANNOTATOR_ERROR_MESSAGES: Record<ImageAnnotatorErrorCode, string> = {} as Record<
  ImageAnnotatorErrorCode,
  string
>
for (const code of Object.keys(ERROR_MESSAGE_KEYS) as ImageAnnotatorErrorCode[]) {
  Object.defineProperty(IMAGE_ANNOTATOR_ERROR_MESSAGES, code, {
    enumerable: true,
    get: () => imageAnnotatorErrorMessage(code),
  })
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
