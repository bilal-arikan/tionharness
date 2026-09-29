import { MAX_SOURCE_BYTES } from '@/features/image-annotator/imageAnnotatorLimits'
import { i18next } from '@/i18n'

function sourceTooLargeError() {
  return new Error(i18next.t('errors.imageTooLarge', { ns: 'artifacts' }))
}

export async function readImageResponse(response: Response): Promise<Uint8Array> {
  const declaredLength = Number(response.headers.get('Content-Length') || '0')
  if (declaredLength > MAX_SOURCE_BYTES) throw sourceTooLargeError()
  if (!response.body) {
    const bytes = new Uint8Array(await response.arrayBuffer())
    if (bytes.length > MAX_SOURCE_BYTES) throw sourceTooLargeError()
    return bytes
  }
  const reader = response.body.getReader()
  const chunks: Uint8Array[] = []
  let length = 0
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    length += value.byteLength
    if (length > MAX_SOURCE_BYTES) {
      await reader.cancel()
      throw sourceTooLargeError()
    }
    chunks.push(value)
  }
  const bytes = new Uint8Array(length)
  let offset = 0
  for (const chunk of chunks) {
    bytes.set(chunk, offset)
    offset += chunk.byteLength
  }
  return bytes
}
