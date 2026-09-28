import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react'
import { api } from '@/api'
import type { Message } from '@/types'
import { mergeLiveTranscriptPage } from './transcriptPageMerge'

const PAGE_SIZE = 50
const WINDOW_SIZE = 150
type Page = Awaited<ReturnType<typeof api.listMessagePage>>
type Options = Parameters<typeof api.listMessagePage>[1]

// One owner for initial load, paging, hub recovery and workspace notifications.
// Every response is fenced by workspace, session and request generation. An
// unmounted/switched view cancels both requests and scheduled refreshes.
export function useTranscript(
  workspaceId: string | null,
  sessionId: string | null,
  highlightId: string | null,
  reportError: (message: string | null) => void,
) {
  const [messages, rawSetMessages] = useState<Message[]>([])
  const [page, setPage] = useState<Page | null>(null)
  const [loading, setLoading] = useState(false)
  const [paging, setPaging] = useState(false)
  const current = useRef({ key: '', messages, page })
  const generation = useRef(0)
  const request = useRef<AbortController | null>(null)
  const scheduled = useRef<{
    timer: ReturnType<typeof setTimeout>
    promise: Promise<Message[] | undefined>
    resolve: (messages?: Message[]) => void
    started: boolean
    repeat: boolean
  } | null>(null)
  const key = `${workspaceId ?? ''}/${sessionId ?? ''}`
  useLayoutEffect(() => {
    current.current = { key, messages, page }
  }, [key, messages, page])

  const setMessages: Dispatch<SetStateAction<Message[]>> = useCallback((update) => {
    rawSetMessages((previous) => {
      const next = typeof update === 'function' ? update(previous) : update
      // An older window must not append a live tail across a missing middle.
      if (!current.current.page?.hasNewer) return next
      const ids = new Set(previous.map((m) => m.id))
      return next.filter((m) => ids.has(m.id))
    })
  }, [])

  const load = useCallback(
    async (options: Options = {}, mode: 'replace' | 'older' | 'newer' = 'replace') => {
      if (!workspaceId || !sessionId) return
      const token = ++generation.current
      request.current?.abort()
      const controller = new AbortController()
      const before = current.current.messages
      request.current = controller
      try {
        let result: Page
        try {
          result = await api.listMessagePage(sessionId, options, controller.signal)
        } catch (error) {
          // A removed cursor invalidates the window; recover from the live edge.
          if ((error as { status?: number }).status !== 404 || controller.signal.aborted)
            throw error
          result = await api.listMessagePage(sessionId, {}, controller.signal)
          mode = 'replace'
        }
        if (
          controller.signal.aborted ||
          token !== generation.current ||
          current.current.key !== key
        )
          return
        let items = result.items
        if (mode !== 'replace') {
          const previous = current.current.messages.filter((m) => !m.id.startsWith('live-'))
          const combined = mode === 'older' ? [...items, ...previous] : [...previous, ...items]
          const seen = new Set<string>()
          const unique = combined.filter((m) => !seen.has(m.id) && !!seen.add(m.id))
          items = mode === 'older' ? unique.slice(0, WINDOW_SIZE) : unique.slice(-WINDOW_SIZE)
          const offset =
            mode === 'older'
              ? result.offset
              : Math.max(0, result.offset + result.items.length - items.length)
          result = {
            ...result,
            items,
            offset,
            hasMore: offset > 0,
            hasNewer: offset + items.length < result.total,
          }
        } else if (!result.hasNewer && !options?.around && !options?.start) {
          items = mergeLiveTranscriptPage(items, before, current.current.messages)
        }
        current.current = { key, messages: items, page: result }
        rawSetMessages(items)
        setPage(result)
        return items
      } catch (error) {
        if (
          !controller.signal.aborted &&
          token === generation.current &&
          current.current.key === key
        )
          reportError((error as Error).message)
      } finally {
        if (token === generation.current && current.current.key === key) {
          setLoading(false)
          setPaging(false)
        }
      }
    },
    [workspaceId, sessionId, key, reportError],
  )

  useEffect(() => {
    rawSetMessages([])
    setPage(null)
    setLoading(!!sessionId)
    setPaging(false)
    current.current = { key, messages: [], page: null }
    if (sessionId) void load()
    return () => {
      generation.current++
      request.current?.abort()
      if (scheduled.current) {
        clearTimeout(scheduled.current.timer)
        scheduled.current.resolve()
        scheduled.current = null
      }
    }
  }, [key, sessionId, load])

  useEffect(() => {
    if (highlightId && sessionId && !messages.some((m) => m.id === highlightId)) {
      setPaging(true)
      void load({ around: highlightId })
    }
  }, [highlightId, sessionId, load])

  const refreshMessages = useCallback(
    (sid: string): Promise<Message[] | undefined> => {
      if (sid !== sessionId || current.current.key !== key) return Promise.resolve(undefined)
      if (scheduled.current) {
        if (scheduled.current.started) scheduled.current.repeat = true
        return scheduled.current.promise
      }
      let resolve!: (messages?: Message[]) => void
      const promise = new Promise<Message[] | undefined>((done) => {
        resolve = done
      })
      const timer = setTimeout(async () => {
        const pending = scheduled.current
        if (!pending || pending.promise !== promise) return
        pending.started = true
        let items: Message[] | undefined
        do {
          pending.repeat = false
          const state = current.current
          const options =
            state.page?.hasNewer && state.messages[0]
              ? {
                  start: state.messages[0].id,
                  limit: Math.min(WINDOW_SIZE, Math.max(PAGE_SIZE, state.messages.length)),
                }
              : { limit: Math.min(WINDOW_SIZE, Math.max(PAGE_SIZE, state.messages.length)) }
          items = await load(options)
        } while (pending.repeat && scheduled.current === pending && current.current.key === key)
        resolve(items)
        if (scheduled.current === pending) scheduled.current = null
      }, 75)
      scheduled.current = { timer, promise, resolve, started: false, repeat: false }
      return promise
    },
    [key, sessionId, load],
  )

  const loadOlder = useCallback(() => {
    const state = current.current
    if (paging || !state.page?.hasMore || !state.messages[0]) return
    setPaging(true)
    void load({ before: state.messages[0].id }, 'older')
  }, [paging, load])
  const loadNewer = useCallback(() => {
    const state = current.current
    const last = state.messages.at(-1)
    if (paging || !state.page?.hasNewer || !last) return
    setPaging(true)
    void load({ after: last.id }, 'newer')
  }, [paging, load])
  const loadLatest = useCallback(() => {
    setPaging(true)
    void load()
  }, [load])
  return {
    messages,
    setMessages,
    messagesLoading: loading,
    refreshMessages,
    transcriptPaging: {
      offset: page?.offset ?? 0,
      total: page?.total ?? messages.length,
      summary: page?.summary,
      participants: page?.participants,
      hasOlder: !!page?.hasMore,
      hasNewer: !!page?.hasNewer,
      loading: paging,
      loadOlder,
      loadNewer,
      loadLatest,
    },
  }
}
