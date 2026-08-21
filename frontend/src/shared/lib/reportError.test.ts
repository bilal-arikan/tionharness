import { describe, expect, it } from 'vitest'
import { SESSION_STREAM_CLOSED_MESSAGE } from './expectedAbort'
import { isExpectedUnhandledRejection } from './reportError'

describe('isExpectedUnhandledRejection', () => {
  it('ignores only the intentional session stream unsubscribe abort', () => {
    expect(
      isExpectedUnhandledRejection(new DOMException(SESSION_STREAM_CLOSED_MESSAGE, 'AbortError')),
    ).toBe(true)
  })

  it('keeps unrelated aborts and errors reportable', () => {
    expect(isExpectedUnhandledRejection(new DOMException('signal is aborted', 'AbortError'))).toBe(
      false,
    )
    expect(isExpectedUnhandledRejection(new Error(SESSION_STREAM_CLOSED_MESSAGE))).toBe(false)
  })
})
