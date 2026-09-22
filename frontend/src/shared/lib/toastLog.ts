// Toast persistence: every error/warning toast is forwarded to the backend log
// store (POST /api/logs, source "toast" → component "ui-toast") so it can be
// inspected in the Logs screen, GET /api/logs?component=ui-toast or the
// read_logs tool after the bubble has faded away. Called centrally from
// toastStore.push — call sites never log toasts themselves.
//
// Loop safety: the report goes through reportClientError, a raw fetch whose
// failure is swallowed, so a failed log POST can never raise another toast. A
// re-entrancy flag additionally drops any toast raised synchronously while a
// report is being sent. Bursts are deduped (same tone + message) and capped per
// window; the suppressed count is reported once the window rolls over.

import { reportClientError } from './reportError'

export type LoggedToastTone = 'error' | 'warning'

export interface ToastLogInput {
  tone: LoggedToastTone
  message: string
  detail?: string
}

export const TOAST_DEDUPE_MS = 10_000
export const TOAST_BURST_WINDOW_MS = 10_000
export const TOAST_BURST_MAX = 20

const lastSeen = new Map<string, number>()
let windowStart = 0
let windowCount = 0
let suppressed = 0
let sending = false

function send(input: ToastLogInput, stack: string | undefined, now: number) {
  sending = true
  try {
    reportClientError({
      source: 'toast',
      level: input.tone === 'error' ? 'error' : 'warn',
      message: input.message,
      detail: input.detail,
      stack,
      time: now,
    })
  } finally {
    sending = false
  }
}

// logToast records one toast, subject to dedupe and the burst cap. It never
// throws: logging is a side channel and must not break showing the toast.
export function logToast(input: ToastLogInput, now: number = Date.now()): void {
  if (sending || !input.message) return
  try {
    const key = `${input.tone}:${input.message}`
    const last = lastSeen.get(key)
    if (last !== undefined && now - last < TOAST_DEDUPE_MS) return
    lastSeen.set(key, now)
    if (lastSeen.size > 200) {
      for (const [k, t] of lastSeen) if (now - t >= TOAST_DEDUPE_MS) lastSeen.delete(k)
    }

    if (now - windowStart >= TOAST_BURST_WINDOW_MS) {
      if (suppressed > 0) {
        send(
          {
            tone: 'warning',
            message: `${suppressed} toast log report(s) suppressed by the burst limit`,
          },
          undefined,
          now,
        )
      }
      windowStart = now
      windowCount = 0
      suppressed = 0
    }
    if (windowCount >= TOAST_BURST_MAX) {
      suppressed++
      return
    }
    windowCount++
    // The call-site stack tells which component raised the toast.
    send(input, new Error('toast').stack, now)
  } catch {
    // Swallow — see the loop-safety note above.
  }
}

// resetToastLog clears the dedupe/burst state (tests only).
export function resetToastLog(): void {
  lastSeen.clear()
  windowStart = 0
  windowCount = 0
  suppressed = 0
  sending = false
}
