// Bound both fetch and response-body reads. A stalled transport must release
// the caller's loading state; cancellation by the caller keeps its own reason.
export const API_REQUEST_TIMEOUT_MS = 30_000

export function requestDeadline(caller?: AbortSignal | null, timeoutMs = API_REQUEST_TIMEOUT_MS) {
  const controller = new AbortController()
  let timedOut = false
  const onAbort = () => controller.abort(caller?.reason)
  if (caller?.aborted) onAbort()
  else caller?.addEventListener('abort', onAbort, { once: true })
  const timer =
    timeoutMs > 0
      ? setTimeout(() => {
          timedOut = true
          controller.abort(new DOMException('API request timed out', 'TimeoutError'))
        }, timeoutMs)
      : null
  return {
    signal: controller.signal,
    timedOut: () => timedOut,
    close: () => {
      if (timer !== null) clearTimeout(timer)
      caller?.removeEventListener('abort', onAbort)
    },
  }
}
