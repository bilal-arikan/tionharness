import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import type { Agent, Artifact, Message } from '@/types'
import { useStableCallback } from '@/shared/lib/useStableCallback'
import { MessageTime, LiveTimer } from './MessageMeta'
import { AgentHeader } from './AgentHeader'
import { WorkingDots } from './WorkingDots'
import { AutoPromptNote } from './AutoPromptNote'
import { TaskNotificationNote } from './TaskNotificationNote'
import { parseTaskNotification } from './parseTaskNotification'
import { UserTurn } from './UserTurn'
import { UserBubble } from './UserBubble'
import { PeerTurn } from './PeerTurn'
import { AssistantTurn } from './AssistantTurn'
import { agentName, resolveAgent } from '@/shared/lib/agentLookup'
import { useTranscriptWindow } from './useTranscriptWindow'
import { TranscriptPaging, type TranscriptPagingState } from './TranscriptPaging'
import { CACHE_TTL_SEC } from '@/features/sessions/sessionDetailFormat'
import { useTranslation } from 'react-i18next'

const SNOWFLAKE = String.fromCodePoint(0x2744, 0xfe0f)

interface Props {
  messages: Message[]
  transcriptPaging?: TranscriptPagingState
  // Session these messages belong to — enables the per-message debug panel.
  sessionId?: string
  pending: boolean
  // When the standalone "working" bubble shows (no live assistant message yet),
  // attribute it to this agent — renders its avatar+name header like a real
  // assistant turn. Used by polled views (executions/flows) that have no live
  // streaming placeholder; the chat leaves it unset (it injects a live message).
  pendingAgentId?: string
  agents: Agent[]
  // Session artifacts, used to resolve an attachment chip to its captured
  // artifact (matched by sourcePath === attachment.relPath) so clicking it opens
  // the artifact viewer.
  artifacts?: Artifact[]
  // True when the open session has a turn currently streaming — drives the live
  // elapsed timer on the last (in-flight) assistant bubble.
  streaming?: boolean
  // A message id to scroll to and briefly highlight — set when the user opens a
  // cross-session search result. Consumed (and cleared via onHighlightConsumed)
  // once the transcript has rendered and the scroll has run.
  highlightMessageId?: string | null
  onHighlightConsumed?: () => void
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
  onSelectSession?: (id: string) => void
  // Delete a single message (prune a mistaken/test one). Shown on row hover.
  onDeleteMessage?: (id: string) => void
  // Rewind the conversation to a user message (remove it + everything after).
  onRewind?: (id: string) => void
  // Retry the failed turn behind an assistant bubble that errored.
  onRetry?: (id: string) => void
  // Rate an assistant turn (👍/👎): rating +1 / -1 / 0 (clear).
  onFeedback?: (id: string, rating: number) => void
  // Open an agent's settings page (Agents view) — fired when the assistant's
  // avatar/name header is clicked in the transcript.
  onOpenAgent?: (id: string) => void
  // Height (px) of the floating bottom stack (composer + banners) that overlays
  // the transcript. Applied as extra scroll padding so the newest message always
  // clears the opaque input instead of hiding behind it, while the rows above it
  // still scroll UNDER the composer's transparent-topped gradient.
  bottomInset?: number
  // Bumped by the parent the moment the user submits (sends or queues) a message.
  // Jumping to the bottom right then — rather than waiting for the turn to appear
  // — matters because the send path only enqueues: the message is painted when the
  // backend worker picks it up, so a user who had scrolled up would otherwise stare
  // at old history with no sign their message went anywhere.
  scrollBottomSignal?: number
}

// MessageList is the scrolling transcript. It owns scroll-pinning and per-message
// tool-trace collapse, then delegates each row to UserTurn / AutoPromptNote /
// AssistantTurn. The standalone pending bubble covers polled views with no live
// streaming placeholder.
export function MessageList({
  messages,
  transcriptPaging,
  sessionId,
  pending,
  pendingAgentId,
  agents,
  artifacts,
  streaming,
  highlightMessageId,
  onHighlightConsumed,
  onOpenFile: onOpenFileProp,
  onOpenArtifact: onOpenArtifactProp,
  onSelectSession: onSelectSessionProp,
  onDeleteMessage: onDeleteMessageProp,
  onRewind: onRewindProp,
  onRetry: onRetryProp,
  onFeedback: onFeedbackProp,
  onOpenAgent: onOpenAgentProp,
  bottomInset,
  scrollBottomSignal,
}: Props) {
  const { t } = useTranslation('chat')
  // Identity-stable handlers. The rows below are React.memo'd, and callers hand
  // these in as inline arrow functions — without this, every row would re-render
  // on every streaming delta and the memo would buy nothing.
  const onOpenFile = useStableCallback(onOpenFileProp)
  const onOpenArtifact = useStableCallback(onOpenArtifactProp)
  const onSelectSession = useStableCallback(onSelectSessionProp)
  const onDeleteMessage = useStableCallback(onDeleteMessageProp)
  const onRewind = useStableCallback(onRewindProp)
  const onRetry = useStableCallback(onRetryProp)
  const onFeedback = useStableCallback(onFeedbackProp)
  const onOpenAgent = useStableCallback(onOpenAgentProp)
  const scrollRef = useRef<HTMLDivElement>(null)
  const virtual = useTranscriptWindow(messages, scrollRef)
  const paging = transcriptPaging
    ? {
        ...transcriptPaging,
        loadOlder: () => {
          virtual.captureAnchor()
          transcriptPaging.loadOlder()
        },
        loadNewer: () => {
          virtual.captureAnchor()
          transcriptPaging.loadNewer()
        },
        loadLatest: () => {
          virtual.clearAnchor()
          pinnedRef.current = true
          transcriptPaging.loadLatest()
        },
      }
    : undefined
  const previousSession = useRef(sessionId)
  // Transiently highlighted message (from a search deep-link); cleared after the
  // flash animation so the highlight doesn't stick.
  const [flashId, setFlashId] = useState<string | null>(null)
  // Whether the user is currently pinned to the bottom of the transcript. When
  // they scroll up to read history we stop auto-scrolling so streaming deltas
  // don't yank them back down.
  const pinnedRef = useRef(true)
  // First message id, used to detect a session switch (full list swap) and
  // re-pin to the bottom regardless of the previous scroll position.
  const firstId = messages[0]?.id
  const prevFirstId = useRef(firstId)
  // Last message id, used to detect a freshly-appended turn. When the newest turn
  // is the human's own just-sent message we always re-pin to the bottom (see the
  // scroll effect), even if the user had scrolled up to read history.
  const prevLastId = useRef(messages[messages.length - 1]?.id)
  // resolveAgent, not a raw find: a session outlives its agent, so the author of
  // an old turn may be deleted. It comes back flagged (AgentIdentity shows the
  // "silinmiş" badge) instead of undefined or a bare id.
  const agentById = (id?: string) => resolveAgent(agents, id) ?? undefined
  // Multi-participant thread detection (generic participant model): count the
  // distinct agents that authored or were addressed in this transcript. Only when
  // 2+ agents take part do we surface the "→ <recipient>" direction cue — a 1:1
  // chat stays clean (every user turn is trivially "→ the one agent").
  const multiParticipant = useMemo(() => {
    const ids = new Set<string>(transcriptPaging?.participants)
    for (const m of messages) {
      if (m.role === 'assistant' && m.agentId) ids.add(m.agentId)
      // A peer message (agent-authored, stored role "user") contributes its SENDER
      // — otherwise an inbox thread (owner + one sender) would read as 1:1 and hide
      // the direction cue.
      if (m.authorKind === 'agent' && m.authorId) ids.add(m.authorId)
      if (m.recipientId && m.recipientId !== '*') ids.add(m.recipientId)
      if (ids.size > 1) return true
    }
    return false
  }, [messages, transcriptPaging?.participants])
  // A message authored by ANOTHER agent but stored with role "user" (a peer/inbox
  // delivery). It renders as an incoming LEFT bubble (PeerTurn), not the human's
  // own right-aligned turn.
  const isPeer = (m: Message): boolean =>
    m.role === 'user' && m.authorKind === 'agent' && !!m.authorId
  // Resolve a peer message's addressee label unconditionally (not gated by the
  // multiParticipant heuristic): an inbox delivery always states whom it reached.
  const peerRecipient = (m: Message): string | undefined => {
    const rid = m.recipientId
    if (!rid) return undefined
    if (rid === '*') return t('messageList.everyone')
    return agentName(agents, rid)
  }
  // Resolve a turn's "→ <name>" recipient label from recipientId (falling back to
  // the legacy agentId a user turn carries). Empty in a 1:1 thread or an undirected
  // (thread-at-large) turn; "herkes" for a broadcast ("*").
  const recipientLabel = (m: Message): string | undefined => {
    if (!multiParticipant) return undefined
    const rid = m.recipientId || (m.role === 'user' ? m.agentId : undefined)
    if (!rid) return undefined
    if (rid === '*') return t('messageList.everyone')
    return agentName(agents, rid)
  }
  // Per-message collapse of the tool-activity trace (the TurnSteps block). Keyed
  // by message id; a message is shown expanded unless its id is in the set.
  const [collapsedTools, setCollapsedTools] = useState<ReadonlySet<string>>(() => new Set())
  // useCallback: passed to every memoized AssistantTurn row.
  const toggleTools = useCallback((id: string) => {
    setCollapsedTools((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  // Message index of the user message currently pinned to the top — the most
  // recent typed user message that has scrolled above the viewport's top edge.
  // It updates as you scroll (section-header behavior): a newer question takes
  // over the moment it reaches the top; -1 means nothing has scrolled past yet.
  const [activePinnedIndex, setActivePinnedIndex] = useState(-1)

  // updateActivePinned scans mounted user rows and picks the last (largest
  // index) one whose top has reached/passed the viewport top — that becomes the
  // pinned header. Rows are in DOM order, so the last qualifying wins.
  const updateActivePinned = useStableCallback((el: HTMLDivElement) => {
    if (el.scrollTop <= 1) {
      setActivePinnedIndex(-1)
      return
    }
    const cTop = el.getBoundingClientRect().top
    let active = -1
    for (let i = virtual.start - 1; i >= 0; i--) {
      const m = messages[i]
      if (m.role === 'user' && !m.origin && !isPeer(m)) {
        active = i
        break
      }
    }
    el.querySelectorAll<HTMLElement>('[data-user-row]').forEach((r) => {
      if (r.getBoundingClientRect().top - cTop <= 1) {
        const idx = Number(r.dataset.idx)
        if (!Number.isNaN(idx)) active = idx
      }
    })
    setActivePinnedIndex((prev) => (prev === active ? prev : active))
  })!

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (el) updateActivePinned(el)
  }, [virtual.start, virtual.end, messages, updateActivePinned])

  // Pending rAF handle for the scroll-driven pinned-header measurement.
  const pinnedFrameRef = useRef<number | null>(null)

  // Coalesce geometry reads for the mounted rows to one animation frame, even
  // when a wheel gesture emits several scroll events before the next paint.
  function schedulePinnedUpdate() {
    if (pinnedFrameRef.current !== null) return
    pinnedFrameRef.current = requestAnimationFrame(() => {
      pinnedFrameRef.current = null
      const el = scrollRef.current
      if (el) updateActivePinned(el)
    })
  }

  useEffect(
    () => () => {
      if (pinnedFrameRef.current !== null) cancelAnimationFrame(pinnedFrameRef.current)
    },
    [],
  )

  function onScroll() {
    const el = scrollRef.current
    if (!el) return
    // The pin distance stays synchronous: the layout effect below reads
    // pinnedRef on the very next commit, so it must reflect the latest scroll.
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    pinnedRef.current = distance < 80
    virtual.onScroll()
    schedulePinnedUpdate()
  }

  // Bring a message's row back into view just BELOW the container's top edge.
  // The 8px gap is deliberate: landing the row at exactly the top would still
  // satisfy updateActivePinned's "has reached the top" test, so the sticky header
  // would sit right on top of the very message we jumped to.
  function scrollRowIntoView(id: string) {
    const el = scrollRef.current
    const index = messages.findIndex((m) => m.id === id)
    if (index >= 0) virtual.scrollToIndex(index)
    const row = el?.querySelector<HTMLElement>(`[data-msg-id="${CSS.escape(id)}"]`)
    if (!el) return
    if (!row) {
      pinnedRef.current = false
      setFlashId(id)
      return
    }
    el.scrollTop += row.getBoundingClientRect().top - el.getBoundingClientRect().top - 8
    pinnedRef.current = false // a deliberate jump must not be yanked back down
    updateActivePinned(el)
    setFlashId(id)
    // Off-screen rows render lazily (SKIPPED_ROW), so the rows we just scrolled
    // past were measured at their ESTIMATED height — the jump can land off by a
    // few hundred pixels. One frame later they have painted at their real size;
    // re-align against those to land exactly.
    requestAnimationFrame(() => {
      const el2 = scrollRef.current
      const row2 = el2?.querySelector<HTMLElement>(`[data-msg-id="${CSS.escape(id)}"]`)
      if (!el2 || !row2) return
      el2.scrollTop += row2.getBoundingClientRect().top - el2.getBoundingClientRect().top - 8
      updateActivePinned(el2)
    })
  }

  // useLayoutEffect (NOT useEffect): the scroll-to-bottom + active-pinned-index
  // update must run BEFORE the browser paints. With a post-paint useEffect, each
  // streaming/tool-step messages change painted one frame at the stale scroll
  // position and stale pinned index first, which flashed the full-width sticky
  // pinned-question overlay for a single frame before the effect corrected it.
  // Running pre-paint ties the scroll and the overlay's visibility to the same
  // commit, so there is no intermediate inconsistent frame.
  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return
    if (
      previousSession.current !== sessionId ||
      (prevFirstId.current === undefined && firstId !== undefined)
    ) {
      previousSession.current = sessionId
      // Session changed: always land at the bottom of the new transcript.
      prevFirstId.current = firstId
      pinnedRef.current = true
    }
    prevFirstId.current = firstId
    // A newly-appended human turn (we just sent a message) always re-pins to the
    // bottom, even if the user had scrolled up — so the sent message is visible.
    // Peer/inbox deliveries (agent-authored role "user") and streaming assistant
    // deltas don't force this; they respect the existing pin state.
    const lastMsg = messages[messages.length - 1]
    if (lastMsg && lastMsg.id !== prevLastId.current) {
      if (
        lastMsg.role === 'user' &&
        lastMsg.authorKind !== 'agent' &&
        !transcriptPaging?.hasNewer
      ) {
        pinnedRef.current = true
      }
    }
    prevLastId.current = lastMsg?.id
    if (!pinnedRef.current) {
      // Streaming/layout grew the content; the active pinned header may change.
      updateActivePinned(el)
      return
    }
    // Jump instantly (not smooth): rapid streaming deltas update `messages` on
    // every token, and a smooth animation restarted each delta never settles —
    // the symptom where the live reply seems to vanish until the turn finishes.
    el.scrollTop = el.scrollHeight
    virtual.updateViewport()
    updateActivePinned(el)
    // The transcript window object is intentionally excluded: its identity is not
    // stable, while the scalar dimensions above are the actual scroll triggers.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [messages, pending, firstId, sessionId, virtual.total, virtual.height, bottomInset])

  // Deep-link: when a search result is opened, scroll to the target message once
  // it is present in the loaded transcript, flash it, then clear the request.
  const consumeHighlight = useStableCallback(onHighlightConsumed)
  useEffect(() => {
    if (!highlightMessageId) return
    const index = messages.findIndex((m) => m.id === highlightMessageId)
    if (index < 0) return
    // Imperative scroll pin state is intentionally kept outside React rendering.
    // eslint-disable-next-line react-hooks/immutability
    pinnedRef.current = false
    virtual.scrollToIndex(index)
    // This state mirrors an external navigation request consumed by the effect.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setFlashId(highlightMessageId)
    requestAnimationFrame(() => {
      const container = scrollRef.current
      const row = container?.querySelector<HTMLElement>(
        `[data-msg-id="${CSS.escape(highlightMessageId)}"]`,
      )
      if (container && row)
        container.scrollTop +=
          row.getBoundingClientRect().top - container.getBoundingClientRect().top - 8
      virtual.updateViewport()
    })
    consumeHighlight?.()
    // The transcript window object is intentionally excluded because its identity
    // changes independently of the navigation request handled here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [highlightMessageId, messages, consumeHighlight])

  // Fade the transient highlight out, whichever jump set it (search deep-link or
  // a click on the sticky pinned question).
  useEffect(() => {
    if (!flashId) return
    const t = setTimeout(() => setFlashId(null), 1600)
    return () => clearTimeout(t)
  }, [flashId])

  // Explicit jump to the bottom, fired when the user submits a message. Separate
  // from the pin-driven scroll above because the send path only ENQUEUES: the
  // message is painted later, when the backend worker picks it up, so waiting for
  // it would leave a scrolled-up user with no feedback that anything happened.
  useEffect(() => {
    if (!scrollBottomSignal) return
    const el = scrollRef.current
    if (!el) return
    // Imperative scroll pin state is intentionally kept outside React rendering.
    // eslint-disable-next-line react-hooks/immutability
    pinnedRef.current = true
    if (transcriptPaging?.hasNewer) transcriptPaging.loadLatest()
    el.scrollTop = el.scrollHeight
    virtual.updateViewport()
    updateActivePinned(el)
    // The signal is the sole trigger; paging and virtual helpers expose the latest
    // mutable transcript state without needing to retrigger this effect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scrollBottomSignal])

  // When a live assistant bubble is already present, the standalone pending bubble
  // would duplicate it — the `last.role === 'user'` test is what suppresses it.
  //
  // `streaming` counts alongside `pending`: opening a session whose turn is already
  // running lands us between the turn's start and its first assistant frame, and in
  // that window only the streaming latch is set. Without it the transcript ends on
  // our own message with no sign the agent is working. The two hand over cleanly —
  // once the assistant bubble exists, `last` is no longer the user's turn and the
  // in-bubble WorkingDots take over.
  const last = messages[messages.length - 1]
  const showStandalonePending = (pending || !!streaming) && (!last || last.role === 'user')

  // The typed user question that has scrolled above the top edge — rendered as a
  // compact overlay header (below), NOT as an in-flow sticky row. Keeping it out
  // of the scroll flow means engaging/disengaging the pin never changes the
  // container's scrollHeight, which is what previously caused a clamp↔unclamp
  // reflow oscillation (visible flicker + fighting the scroll).
  const pinned = activePinnedIndex >= 0 ? messages[activePinnedIndex] : undefined
  const pinnedTyped =
    pinned && pinned.role === 'user' && !pinned.origin && !isPeer(pinned) ? pinned : undefined

  return (
    <div className="relative min-h-0 flex-1">
      {/* Overlay pinned-question header. The WRAPPER stays pointer-events-none so
          wheel/touch scroll over the gradient passes straight through to the
          transcript underneath; only the bubble itself re-enables them, so it can
          be clicked to jump back to that question. Because the overlay sits
          OUTSIDE the scroll container, a wheel over the bubble has no scrollable
          ancestor to bubble into — hence the explicit forward in onWheel. */}
      {pinnedTyped && (
        <div className="th-measure pointer-events-none absolute inset-x-0 top-0 z-10 bg-gradient-to-b from-[var(--color-bg)] via-[var(--color-bg)] to-transparent pt-2 pb-6">
          {/* role="button" on a div rather than a real <button>: UserBubble renders
              block-level markup, which a button's phrasing-only content model
              forbids. Keyboard activation is wired explicitly to match. */}
          <div
            role="button"
            tabIndex={0}
            onClick={() => scrollRowIntoView(pinnedTyped.id)}
            onKeyDown={(e) => {
              if (e.key !== 'Enter' && e.key !== ' ') return
              e.preventDefault()
              scrollRowIntoView(pinnedTyped.id)
            }}
            onWheel={(e) => {
              const el = scrollRef.current
              if (el) el.scrollTop += e.deltaY
            }}
            title={t('messageList.returnToQuestion')}
            aria-label={t('messageList.returnToPinnedQuestion')}
            className="pointer-events-auto cursor-pointer rounded-2xl outline-none ring-[var(--color-accent)] focus-visible:ring-2"
          >
            <div aria-hidden>
              <UserBubble text={pinnedTyped.text} agents={agents} clamp />
            </div>
          </div>
        </div>
      )}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        onWheel={virtual.clearAnchor}
        onPointerDown={virtual.clearAnchor}
        onTouchStart={virtual.clearAnchor}
        onKeyDown={virtual.clearAnchor}
        data-testid="chat-transcript"
        role="log"
        aria-live="polite"
        aria-label={t('messageList.history')}
        className="th-measure h-full overflow-y-auto pb-6 pt-2"
        style={{ paddingBottom: bottomInset || undefined, overflowAnchor: 'none' }}
      >
        <div className="w-full">
          <TranscriptPaging state={paging} edge="older" />
          <div aria-hidden style={{ height: virtual.top }} />
          {messages.slice(virtual.start, virtual.end).map((m, localIndex) => {
            const i = virtual.start + localIndex
            let row: ReactNode
            // Cold boundary: the gap to the previous message outran the prompt
            // cache's 1h TTL, so this turn started from a fully cold prefix. Drawn
            // as a divider (like a date separator) because the cause is the GAP
            // between two messages, not either message — it answers "why was that
            // turn expensive?" while scrolling, with no request and no backend field.
            const gapSec = i > 0 ? m.createdAt - messages[i - 1].createdAt : 0
            const coldBoundary = gapSec > CACHE_TTL_SEC
            // isLastLive (assistant): the in-flight bubble while streaming. Hoisted
            // here so the row wrapper can expose it as a DOM signal (data-streaming)
            // for external automation to detect turn completion without polling.
            const rowLive = m.role !== 'user' && !!streaming && i === messages.length - 1
            // A real (typed) user message — the only rows eligible to pin at top.
            // Peer/inbox deliveries share role "user" but are incoming, not typed.
            const isTypedUser = m.role === 'user' && !m.origin && !isPeer(m)
            if (m.role === 'user') {
              // Peer/inbox delivery (another agent authored it): render as an
              // incoming LEFT bubble with the sender's identity — not our own turn.
              row = isPeer(m) ? (
                <PeerTurn
                  message={m}
                  sender={agentById(m.authorId)}
                  recipientLabel={peerRecipient(m)}
                  onDelete={onDeleteMessage}
                  onOpenAgent={onOpenAgent}
                />
              ) : // Worker <task-notification> injections get their own collapsible
              // card (raw XML is unreadable as a plain note); other origins keep
              // the generic auto-continuation note.
              m.origin === 'worker-note' ? (
                <TaskNotificationNote
                  message={m}
                  agent={agentById(parseTaskNotification(m.text)?.agentId)}
                  onSelectSession={onSelectSession}
                  onOpenFile={onOpenFile}
                  onDelete={onDeleteMessage}
                />
              ) : m.origin ? (
                <AutoPromptNote message={m} onDelete={onDeleteMessage} />
              ) : (
                <UserTurn
                  message={m}
                  agents={agents}
                  artifacts={artifacts}
                  onDelete={onDeleteMessage}
                  onRewind={onRewind}
                  onOpenArtifact={onOpenArtifact}
                  recipientLabel={recipientLabel(m)}
                />
              )
            } else {
              // The in-flight assistant bubble is the last message while streaming; its
              // createdAt marks the turn start, so a live timer counts up from it.
              const isLastLive = rowLive
              // Completed-turn working time comes FROM THE SERVER: the backend times
              // the agent run and persists it as Message.durationMs. Legacy fallback
              // (messages written before that field existed): the createdAt gap to the
              // triggering user message — only meaningful when the previous message is
              // the user's, since injected summaries or consecutive assistant turns
              // would otherwise report idle gaps rather than real work. It is flagged
              // as derived so the UI marks it approximate.
              const prev = messages[i - 1]
              const serverMs = m.durationMs ?? 0
              const workedDerived = serverMs <= 0
              const workedMs = workedDerived
                ? prev?.role === 'user'
                  ? (m.createdAt - prev.createdAt) * 1000
                  : 0
                : serverMs
              row = (
                <AssistantTurn
                  message={m}
                  agent={agentById(m.agentId ?? (isLastLive ? pendingAgentId : undefined))}
                  sessionId={sessionId}
                  isLastLive={isLastLive}
                  workedMs={workedMs}
                  workedDerived={workedDerived}
                  toolsHidden={collapsedTools.has(m.id)}
                  onToggleTools={toggleTools}
                  onOpenFile={onOpenFile}
                  onOpenArtifact={onOpenArtifact}
                  onDelete={onDeleteMessage}
                  onRetry={onRetry}
                  onFeedback={onFeedback}
                  onOpenAgent={onOpenAgent}
                  recipientLabel={recipientLabel(m)}
                />
              )
            }
            // Rows stay full-height and in normal flow — the pinned question is a
            // separate overlay header (rendered above), so nothing here changes the
            // scroll layout. flashCls is the transient search deep-link highlight.
            const flashCls =
              flashId === m.id
                ? 'rounded-2xl ring-2 ring-[var(--color-accent)] ring-offset-2 ring-offset-[var(--color-bg)] transition-shadow'
                : ''
            const wrapperCls = flashCls || undefined
            const rowEl = (
              <div
                key={m.id}
                data-msg-id={m.id}
                data-testid="chat-message"
                data-role={m.role}
                data-streaming={rowLive ? 'true' : 'false'}
                data-user-row={isTypedUser ? 'true' : undefined}
                data-idx={isTypedUser ? i : undefined}
                className={wrapperCls}
              >
                {row}
              </div>
            )
            // A CLI cold start is a SEPARATE boundary from the cache one: the
            // underlying CLI conversation restarted here even though no long gap
            // preceded it. Both can land on the same turn (a day-long pause makes
            // the CLI thread unresumable too), so they stack rather than compete.
            return (
              <div
                key={m.id}
                ref={virtual.measure}
                data-virtual-id={m.id}
                className="flex flex-col gap-4 pb-4"
              >
                {coldBoundary && <ColdCacheDivider gapSec={gapSec} />}
                {m.cliColdStart && <CLIColdStartDivider />}
                {rowEl}
              </div>
            )
          })}

          <div aria-hidden style={{ height: virtual.bottom }} />
          <TranscriptPaging state={paging} edge="newer" />
          {showStandalonePending && !transcriptPaging?.hasNewer && (
            <div className="group flex flex-col gap-1">
              <div className="flex w-full justify-start">
                <div className="w-full min-w-0 rounded-2xl bg-[color-mix(in_srgb,var(--color-surface-2)_65%,var(--color-bg))] px-4 py-3">
                  <AgentHeader agent={agentById(pendingAgentId)} onOpenAgent={onOpenAgent} />
                  <WorkingDots />
                </div>
              </div>
              {/* Turn started at the triggering (last) message; count up from it. */}
              {last && (
                <div className="flex items-center gap-2 pl-1">
                  <MessageTime unixSec={last.createdAt} />
                  <LiveTimer startUnixSec={last.createdAt} />
                </div>
              )}
            </div>
          )}

          {messages.length === 0 && !pending && (
            <div className="mt-20 text-center text-[var(--color-text-dim)]">
              <p className="text-lg">{t('messageList.start')}</p>
              <p className="mt-1 text-sm">{t('messageList.writeBelow')}</p>
            </div>
          )}

          <div />
        </div>
      </div>
    </div>
  )
}

// ColdCacheDivider marks a gap in the transcript longer than the prompt cache's
// TTL: everything before it had gone cold, so the turn below re-paid the whole
// cached prefix. Purely derived from the two timestamps — this is NOT a detected
// cache_break event (those are attributed server-side and carded separately); it
// is the ambient "why did this turn cost more" context while scrolling.
function ColdCacheDivider({ gapSec }: { gapSec: number }) {
  const { t } = useTranslation('chat')
  const gap = formatGap(gapSec, t)
  return (
    <div
      className="flex items-center gap-2 px-2 text-[10px] text-[var(--color-text-dim)]"
      title={t('messageList.cacheGapDescription', { gap })}
    >
      <span className="h-px flex-1 bg-[var(--color-border)]" />
      <span className="shrink-0 opacity-80">
        {SNOWFLAKE} {t('messageList.cacheCooled', { gap })}
      </span>
      <span className="h-px flex-1 bg-[var(--color-border)]" />
    </div>
  )
}

// CLIColdStartDivider marks the turn where the underlying CLI conversation
// restarted: `--resume` was enabled but no warm thread carried into this turn, so
// the provider opened a fresh CLI session and re-sent the prepared transcript.
// Backend-attributed (db.Message.CLIColdStart), unlike ColdCacheDivider which is
// derived from timestamps alone.
function CLIColdStartDivider() {
  const { t } = useTranslation('chat')
  return (
    <div
      className="flex items-center gap-2 px-2 text-[10px] text-[var(--color-text-dim)]"
      title={t('messageList.cliRestartDescription')}
    >
      <span className="h-px flex-1 bg-[var(--color-border)]" />
      <span className="shrink-0 opacity-80">🔄 {t('messageList.cliRestart')}</span>
      <span className="h-px flex-1 bg-[var(--color-border)]" />
    </div>
  )
}

// formatGap renders a between-messages gap in hours/days (it is always > 1h here).
function formatGap(sec: number, t: (key: string, options?: { count: number }) => string): string {
  const h = Math.floor(sec / 3600)
  if (h < 24) return t('messageList.hours', { count: h })
  const d = Math.floor(h / 24)
  return t('messageList.days', { count: d })
}
