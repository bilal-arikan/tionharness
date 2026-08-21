export const SESSION_STREAM_CLOSED_MESSAGE = 'session stream subscription closed'

export function isExpectedSessionStreamAbort(reason: unknown): boolean {
  return (
    reason instanceof DOMException &&
    reason.name === 'AbortError' &&
    reason.message === SESSION_STREAM_CLOSED_MESSAGE
  )
}
