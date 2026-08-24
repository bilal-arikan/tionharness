import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react'
import type { Agent, Artifact, Message } from '@/types'
import { useStableCallback } from '@/shared/lib/useStableCallback'
import { MessageTime, LiveTimer } from './MessageMeta'
import { AgentHeader } from './AgentHeader'
import { WorkingDots } from './WorkingDots'
import { AutoPromptNote } from './AutoPromptNote'
import { TaskNotificationNote } from './TaskNotificationNote'
import { UserTurn } from './UserTurn'
import { UserBubble } from './UserBubble'
import { PeerTurn } from './PeerTurn'
import { AssistantTurn } from './AssistantTurn'
import { agentName, resolveAgent } from '@/shared/lib/agentLookup'
import { CACHE_TTL_SEC } from '@/features/sessions/sessionDetailFormat'

interface Props {
  messages: Message[]
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

// SKIPPED_ROW lets the browser skip layout/paint/style for a row that is
// scrolled out of view — a worker session's transcript is hundreds of tool
// cards, markdown blocks and diffs, and rendering all of them is what made
// opening one feel like it "reloads everything from scratch".
//
// This is deliberately NOT a virtualizer: the transcript's scroll logic
// (scrollRowIntoView, updateActivePinned, the search deep-link) queries real
// DOM nodes by data-msg-id, and unmounting off-screen rows would break all of
// it. content-visibility keeps every row in the DOM — only its subtree render
// is skipped — so the queries keep working. `contain-intrinsic-size: auto <h>`
// makes the browser remember each row's real height once painted, so scrollbar
// geometry converges instead of jumping.
const SKIPPED_ROW: CSSProperties = {
  contentVisibility: 'auto',
  containIntrinsicSize: 'auto 320px',
}
// How many trailing rows stay eagerly rendered. The live/most recent turns are
// in view anyway, and skipping them would fight the scroll-to-bottom pinning
// (which reads scrollHeight right after a streaming delta).
const EAGER_TAIL_ROWS = 3
// Below this many rows the whole transcript renders eagerly. Short sessions have
// no render problem to solve, and skipping rows there would only trade a
// non-issue for estimated-height scroll imprecision.
const SKIP_OFFSCREEN_MIN_ROWS = 30

// MessageList is the scrolling transcript. It owns scroll-pinning and per-message
// tool-trace collapse, then delegates each row to UserTurn / AutoPromptNote /
// AssistantTurn. The standalone pending bubble covers polled views with no live
// streaming placeholder.
export function MessageList({
  messages,
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
  onDeleteMessage: onDeleteMessageProp,
  onRewind: onRewindProp,
  onRetry: onRetryProp,
  onFeedback: onFeedbackProp,
  onOpenAgent: onOpenAgentProp,
  bottomInset,
  scrollBottomSignal,
}: Props) {
  // Identity-stable handlers. The rows below are React.memo'd, and callers hand
  // these in as inline arrow functions — without this, every row would re-render
  // on every streaming delta and the memo would buy nothing.
  const onOpenFile = useStableCallback(onOpenFileProp)
  const onOpenArtifact = useStableCallback(onOpenArtifactProp)
  const onDeleteMessage = useStableCallback(onDeleteMessageProp)
  const onRewind = useStableCallback(onRewindProp)
  const onRetry = useStableCallback(onRetryProp)
  const onFeedback = useStableCallback(onFeedbackProp)
  const onOpenAgent = useStableCallback(onOpenAgentProp)
  const scrollRef = useRef<HTMLDivElement>(null)
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
    const ids = new Set<string>()
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
  }, [messages])
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
    if (rid === '*') return 'herkes'
    return agentName(agents, rid)
  }
  // Resolve a turn's "→ <name>" recipient label from recipientId (falling back to
  // the legacy agentId a user turn carries). Empty in a 1:1 thread or an undirected
  // (thread-at-large) turn; "herkes" for a broadcast ("*").
  const recipientLabel = (m: Message): string | undefined => {
    if (!multiParticipant) return undefined
    const rid = m.recipientId || (m.role === 'user' ? m.agentId : undefined)
    if (!rid) return undefined
    if (rid === '*') return 'herkes'
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

  // updateActivePinned scans every typed user row and picks the last (largest
  // index) one whose top has reached/passed the viewport top — that becomes the
  // pinned header. Rows are in DOM order, so the last qualifying wins.
  function updateActivePinned(el: HTMLDivElement) {
    const cTop = el.getBoundingClientRect().top
    let active = -1
    el.querySelectorAll<HTMLElement>('[data-user-row]').forEach((r) => {
      if (r.getBoundingClientRect().top - cTop <= 1) {
        const idx = Number(r.dataset.idx)
        if (!Number.isNaN(idx)) active = idx
      }
    })
    setActivePinnedIndex((prev) => (prev === active ? prev : active))
  }

  function onScroll() {
    const el = scrollRef.current
    if (!el) return
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    pinnedRef.current = distance < 80
    updateActivePinned(el)
  }

  // Bring a message's row back into view just BELOW the container's top edge.
  // The 8px gap is deliberate: landing the row at exactly the top would still
  // satisfy updateActivePinned's "has reached the top" test, so the sticky header
  // would sit right on top of the very message we jumped to.
  function scrollRowIntoView(id: string) {
    const el = scrollRef.current
    const row = el?.querySelector<HTMLElement>(`[data-msg-id="${CSS.escape(id)}"]`)
    if (!el || !row) return
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
    if (prevFirstId.current !== firstId) {
      // Session changed: always land at the bottom of the new transcript.
      prevFirstId.current = firstId
      pinnedRef.current = true
    }
    // A newly-appended human turn (we just sent a message) always re-pins to the
    // bottom, even if the user had scrolled up — so the sent message is visible.
    // Peer/inbox deliveries (agent-authored role "user") and streaming assistant
    // deltas don't force this; they respect the existing pin state.
    const lastMsg = messages[messages.length - 1]
    if (lastMsg && lastMsg.id !== prevLastId.current) {
      if (lastMsg.role === 'user' && lastMsg.authorKind !== 'agent') {
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
    updateActivePinned(el)
  }, [messages, pending, firstId])

  // Deep-link: when a search result is opened, scroll to the target message once
  // it is present in the loaded transcript, flash it, then clear the request.
  useEffect(() => {
    if (!highlightMessageId) return
    const el = scrollRef.current?.querySelector<HTMLElement>(
      `[data-msg-id="${CSS.escape(highlightMessageId)}"]`,
    )
    if (!el) return // transcript not loaded yet; a later messages update re-runs this
    el.scrollIntoView({ block: 'center' })
    pinnedRef.current = false // don't yank back to bottom after the jump
    setFlashId(highlightMessageId)
    // Second pass after paint: the rows we scrolled past were skipped
    // (SKIPPED_ROW) and measured at estimated heights, so the first jump is
    // approximate. Setting flashId also makes the target itself render eagerly.
    requestAnimationFrame(() => {
      scrollRef.current
        ?.querySelector<HTMLElement>(`[data-msg-id="${CSS.escape(highlightMessageId)}"]`)
        ?.scrollIntoView({ block: 'center' })
    })
    onHighlightConsumed?.()
  }, [highlightMessageId, messages])

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
    pinnedRef.current = true
    el.scrollTop = el.scrollHeight
    updateActivePinned(el)
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
        <div className="pointer-events-none absolute inset-x-0 top-0 z-10 bg-gradient-to-b from-[var(--color-bg)] via-[var(--color-bg)] to-transparent px-[1px] pt-2 pb-6 md:px-6">
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
            title="Bu soruya dön"
            aria-label="Sabitlenen soruya dön"
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
        data-testid="chat-transcript"
        role="log"
        aria-live="polite"
        aria-label="Sohbet geçmişi"
        className="h-full overflow-y-auto px-[1px] pb-6 pt-2 md:px-6"
        style={bottomInset ? { paddingBottom: bottomInset } : undefined}
      >
        <div className="flex w-full flex-col gap-4">
          {messages.map((m, i) => {
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
                <TaskNotificationNote message={m} onDelete={onDeleteMessage} />
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
                  agent={agentById(m.agentId)}
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
            // The flashed (deep-linked) row must render eagerly: it is scrolled to
            // and highlighted, and a skipped subtree has no measurable height yet.
            const skipOffscreen =
              messages.length >= SKIP_OFFSCREEN_MIN_ROWS &&
              i < messages.length - EAGER_TAIL_ROWS &&
              flashId !== m.id
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
                style={skipOffscreen ? SKIPPED_ROW : undefined}
              >
                {row}
              </div>
            )
            if (!coldBoundary) return rowEl
            return (
              <div key={`cold-${m.id}`} className="flex flex-col gap-4">
                <ColdCacheDivider gapSec={gapSec} />
                {rowEl}
              </div>
            )
          })}

          {showStandalonePending && (
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
              <p className="text-lg">Sohbete başla</p>
              <p className="mt-1 text-sm">Aşağıya bir mesaj yaz.</p>
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
  return (
    <div
      className="flex items-center gap-2 px-2 text-[10px] text-[var(--color-text-dim)]"
      title={`Bu boşluk (${formatGap(gapSec)}) 1sa cache TTL'ini aştı — sonraki tur öneki soğuk olarak yeniden ödedi.`}
    >
      <span className="h-px flex-1 bg-[var(--color-border)]" />
      <span className="shrink-0 opacity-80">❄️ cache soğudu · {formatGap(gapSec)} ara</span>
      <span className="h-px flex-1 bg-[var(--color-border)]" />
    </div>
  )
}

// formatGap renders a between-messages gap in hours/days (it is always > 1h here).
function formatGap(sec: number): string {
  const h = Math.floor(sec / 3600)
  if (h < 24) return `${h} sa`
  const d = Math.floor(h / 24)
  return `${d} gün`
}
