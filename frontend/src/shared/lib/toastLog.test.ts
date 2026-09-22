import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from '@/shared/components/toastStore'
import { TOAST_BURST_MAX, TOAST_BURST_WINDOW_MS, logToast, resetToastLog } from './toastLog'

type Body = { level: string; source: string; message: string; detail: string; stack: string }

let fetchMock: ReturnType<typeof vi.fn>

function bodies(): Body[] {
  return fetchMock.mock.calls.map((c) => JSON.parse((c[1] as RequestInit).body as string) as Body)
}

beforeEach(() => {
  resetToastLog()
  fetchMock = vi.fn(() => Promise.resolve(new Response(null, { status: 204 })))
  vi.stubGlobal('fetch', fetchMock)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('toast store → log store', () => {
  it('logs error and warning toasts, not success/info', () => {
    toast.error('store-err-1', 8000, 'HTTP 500')
    toast.warning('store-warn-1')
    toast.success('store-ok-1')
    toast.info('store-info-1')
    const b = bodies()
    expect(b.map((x) => [x.level, x.source, x.message])).toEqual([
      ['error', 'toast', 'store-err-1'],
      ['warn', 'toast', 'store-warn-1'],
    ])
    expect(b[0].detail).toBe('HTTP 500')
    expect(b[0].stack).toContain('Error')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/logs')
  })
})

describe('logToast guards', () => {
  it('dedupes an identical toast inside the window', () => {
    logToast({ tone: 'error', message: 'dup-1' }, 1_000)
    logToast({ tone: 'error', message: 'dup-1' }, 2_000)
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('caps bursts and reports the suppressed count once the window rolls over', () => {
    const t0 = 50_000
    for (let i = 0; i < TOAST_BURST_MAX + 5; i++) {
      logToast({ tone: 'error', message: `burst-${i}` }, t0 + i)
    }
    expect(fetchMock).toHaveBeenCalledTimes(TOAST_BURST_MAX)
    logToast({ tone: 'error', message: 'after-burst' }, t0 + TOAST_BURST_WINDOW_MS + 100)
    const msgs = bodies().map((b) => b.message)
    expect(msgs).toContain('5 toast log report(s) suppressed by the burst limit')
    expect(msgs[msgs.length - 1]).toBe('after-burst')
  })

  it('does not toast or throw when the log POST fails', async () => {
    fetchMock.mockImplementation(() => Promise.reject(new TypeError('network down')))
    expect(() => toast.error('post-fails-1')).not.toThrow()
    await Promise.resolve()
    await Promise.resolve()
    // Only the original toast was reported; the failure produced no new report.
    expect(bodies().map((b) => b.message)).toEqual(['post-fails-1'])
  })

  it('drops toasts raised synchronously while a report is being sent', () => {
    fetchMock.mockImplementation(() => {
      toast.error('reentrant-1')
      return Promise.resolve(new Response(null, { status: 204 }))
    })
    toast.error('outer-1')
    expect(bodies().map((b) => b.message)).toEqual(['outer-1'])
  })
})
