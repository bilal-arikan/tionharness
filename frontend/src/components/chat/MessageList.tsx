import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Agent, Artifact, Message } from '../../types'
import { MessageTime, LiveTimer } from './MessageMeta'
import { AgentHeader } from './AgentHeader'
import { WorkingDots } from './WorkingDots'
import { AutoPromptNote } from './AutoPromptNote'
import { UserTurn } from './UserTurn'
import { UserBubble } from './UserBubble'
import { AssistantTurn } from './AssistantTurn'

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
}

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
  onOpenFile,
  onOpenArtifact,
  onDeleteMessage,
  onRewind,
  onRetry,
  onFeedback,
  onOpenAgent,
}: Props) {
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
  const agentById = (id?: string) => (id ? agents.find((a) => a.id === id) : undefined)
  // Per-message collapse of the tool-activity trace (the TurnSteps block). Keyed
  // by message id; a message is shown expanded unless its id is in the set.
  const [collapsedTools, setCollapsedTools] = useState<ReadonlySet<string>>(() => new Set())
  const toggleTools = (id: string) =>
    setCollapsedTools((prev) => {
      const next = new Set(prev)
      next.has(id) ? next.delete(id) : next.add(id)
      return next
    })

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

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    if (prevFirstId.current !== firstId) {
      // Session changed: always land at the bottom of the new transcript.
      prevFirstId.current = firstId
      pinnedRef.current = true
    }
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
    const t = setTimeout(() => setFlashId(null), 1600)
    onHighlightConsumed?.()
    return () => clearTimeout(t)
  }, [highlightMessageId, messages])

  // When a live assistant bubble is already present (streaming), the standalone
  // pending bubble would duplicate it — suppress it in that case.
  const last = messages[messages.length - 1]
  const showStandalonePending = pending && (!last || last.role === 'user')

  // The typed user question that has scrolled above the top edge — rendered as a
  // compact overlay header (below), NOT as an in-flow sticky row. Keeping it out
  // of the scroll flow means engaging/disengaging the pin never changes the
  // container's scrollHeight, which is what previously caused a clamp↔unclamp
  // reflow oscillation (visible flicker + fighting the scroll).
  const pinned = activePinnedIndex >= 0 ? messages[activePinnedIndex] : undefined
  const pinnedTyped = pinned && pinned.role === 'user' && !pinned.origin ? pinned : undefined

  return (
    <div className="relative min-h-0 flex-1">
      {/* Overlay pinned-question header. pointer-events-none so wheel/touch scroll
          passes straight through to the transcript underneath; the top-down
          gradient fades the real question that scrolls beneath it. */}
      {pinnedTyped && (
        <div
          aria-hidden
          className="pointer-events-none absolute inset-x-0 top-0 z-10 bg-gradient-to-b from-[var(--color-bg)] via-[var(--color-bg)] to-transparent px-6 pt-2 pb-6"
        >
          <UserBubble text={pinnedTyped.text} agents={agents} clamp />
        </div>
      )}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        data-testid="chat-transcript"
        role="log"
        aria-live="polite"
        aria-label="Sohbet geçmişi"
        className="h-full overflow-y-auto px-6 pb-6 pt-2"
      >
      <div className="flex w-full flex-col gap-4">
        {messages.map((m, i) => {
          let row: ReactNode
          // isLastLive (assistant): the in-flight bubble while streaming. Hoisted
          // here so the row wrapper can expose it as a DOM signal (data-streaming)
          // for external automation to detect turn completion without polling.
          const rowLive = m.role !== 'user' && !!streaming && i === messages.length - 1
          // A real (typed) user message — the only rows eligible to pin at top.
          const isTypedUser = m.role === 'user' && !m.origin
          if (m.role === 'user') {
            row = m.origin ? (
              <AutoPromptNote message={m} onDelete={onDeleteMessage} />
            ) : (
              <UserTurn
                message={m}
                agents={agents}
                artifacts={artifacts}
                onDelete={onDeleteMessage}
                onRewind={onRewind}
                onOpenArtifact={onOpenArtifact}
              />
            )
          } else {
            // The in-flight assistant bubble is the last message while streaming; its
            // createdAt marks the turn start, so a live timer counts up from it.
            const isLastLive = rowLive
            // Completed-turn working time ≈ this message's createdAt (turn end) minus
            // the triggering user message's (turn start). Only meaningful when the
            // previous message is the user's — injected summaries or consecutive
            // assistant turns would otherwise report idle gaps, not real work.
            const prev = messages[i - 1]
            const workedSec = prev?.role === 'user' ? m.createdAt - prev.createdAt : 0
            row = (
              <AssistantTurn
                message={m}
                agent={agentById(m.agentId)}
                sessionId={sessionId}
                isLastLive={isLastLive}
                workedSec={workedSec}
                toolsHidden={collapsedTools.has(m.id)}
                onToggleTools={toggleTools}
                onOpenFile={onOpenFile}
                onOpenArtifact={onOpenArtifact}
                onDelete={onDeleteMessage}
                onRetry={onRetry}
                onFeedback={onFeedback}
                onOpenAgent={onOpenAgent}
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
          return (
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
