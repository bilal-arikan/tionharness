// ChatView renders the chat screen's main column: the transcript plus the
// floating bottom stack (ask/permission/plan prompts, todo panel, pending tray,
// wake banner, composer) and the rewind dialog. Extracted from App.tsx so the
// shell only composes; all chat-turn machinery arrives via the `chat` handle.
import { useCallback, useEffect, useMemo, useRef, useState, type ComponentProps } from 'react'
import type { Agent, Artifact, Message } from '@/types'
import { MessageList } from './MessageList'
import { Composer } from './Composer'
import { ChatEmptyState } from './ChatEmptyState'
import { ChatSkeleton } from './ChatSkeleton'
import { RewindDialog } from './RewindDialog'
import { InteractionPrompts } from './InteractionPrompts'
import { PendingTray } from './PendingTray'
import { WakeWaitBanner } from './WakeWaitBanner'
import { CacheWarmthStrip } from './CacheWarmthStrip'
import { TodoPanel } from './TodoPanel'
import { RunStatusStrip } from './RunStatusStrip'
import {
  dismissCompletedTodo,
  isCompletedTodoDismissed,
  latestTodos,
  todoDismissalKey,
} from './todos'
import { useDelayedFlag } from '@/shared/hooks/useDelayedFlag'
import { selectableChatAgents } from '@/app/startableAgents'
import type { useChatStream } from './useChatStream'
import { useTranslation } from 'react-i18next'

export interface ChatViewProps {
  chat: ReturnType<typeof useChatStream>
  messages: Message[]
  agents: Agent[]
  artifacts: Artifact[]
  activeSessionId: string | null
  activeAgentId: string | null
  // True while the workspace's agents+sessions are still loading. The empty state
  // is suppressed for its whole duration (a session may still be selected once the
  // list lands); the skeleton itself appears only if the load is slow enough.
  bootstrapping: boolean
  // True while the open session's transcript is being fetched.
  messagesLoading: boolean
  transcriptPaging?: ComponentProps<typeof MessageList>['transcriptPaging']
  // When true the session is a read-only run log (task / schedule): the
  // composer and its ask/todo/pending/wake stack are hidden and a thin banner is
  // shown instead. The transcript, context preview, debug and info panels stay
  // fully available.
  readOnly: boolean
  // Empty-state ("Yeni sohbete başla") wiring, used when no session is active.
  defaultAgentId: string | null
  defaultAgentDeleted?: boolean
  // Roster INCLUDING deleted agents. The transcript renders HISTORY, so it must
  // resolve an author that no longer exists; pickers keep using `agents`.
  allAgents: Agent[]
  // The subset a NEW chat may be started with: service system agents (titler,
  // compaction, …) are not conversation partners, so the empty state must not
  // offer them. Worker profiles stay selectable.
  sessionStartAgents: Agent[]
  onNewSession: () => void
  onSelectDefaultAgent: (id: string) => void
  onGoToAgents: () => void
  // Remount key for the Composer so it re-reads its persisted draft (rewind).
  composerKey: number
  focusSessionId: string | null
  scrollToMsgId: string | null
  onHighlightConsumed: () => void
  onOpenFile: (path: string) => void
  onOpenArtifact: (id: string) => void
  onDeleteMessage: ComponentProps<typeof MessageList>['onDeleteMessage']
  onRewind: ComponentProps<typeof RewindDialog>['onRewind']
  onFeedback: ComponentProps<typeof MessageList>['onFeedback']
  onOpenAgent: (id: string) => void
  onAgentChange: (id: string) => void
}

export function ChatView({
  chat,
  messages,
  agents,
  sessionStartAgents,
  artifacts,
  activeSessionId,
  activeAgentId,
  bootstrapping,
  messagesLoading,
  transcriptPaging,
  readOnly,
  defaultAgentId,
  defaultAgentDeleted,
  allAgents,
  onNewSession,
  onSelectDefaultAgent,
  onGoToAgents,
  composerKey,
  focusSessionId,
  scrollToMsgId,
  onHighlightConsumed,
  onOpenFile,
  onOpenArtifact,
  onDeleteMessage,
  onRewind,
  onFeedback,
  onOpenAgent,
  onAgentChange,
}: ChatViewProps) {
  const { t } = useTranslation('chat')
  // Floating composer overlay: the chat bottom stack (composer + ask/todo/pending/
  // wake banners) is absolutely positioned OVER the transcript so message bubbles
  // scroll UNDER its transparent-topped gradient. We measure the stack's live
  // height and feed it to MessageList as bottom padding, so the newest message
  // always clears the opaque input instead of hiding behind it (which is what made
  // the last user message look "missing" until the turn finished).
  const bottomStackRef = useRef<HTMLDivElement>(null)
  const [bottomInset, setBottomInset] = useState(0)
  useEffect(() => {
    const el = bottomStackRef.current
    if (!el) return
    const measure = () => setBottomInset(el.offsetHeight)
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [activeSessionId, bootstrapping, readOnly])

  // Composer agent dropdown: never offers worker profiles or service system
  // agents, but keeps the open session's own agent so a background session
  // still shows who answers there.
  const selectableAgents = useMemo(
    () => selectableChatAgents(sessionStartAgents, agents, activeAgentId),
    [sessionStartAgents, agents, activeAgentId],
  )
  // The active session's current checklist (latest todo_write across the
  // transcript). Pinned above the composer and updated as the agent ticks items.
  const currentTodo = useMemo(
    () =>
      transcriptPaging?.hasNewer
        ? (transcriptPaging.summary?.todo ?? null)
        : (latestTodos(messages) ?? transcriptPaging?.summary?.todo ?? null),
    [messages, transcriptPaging],
  )

  const currentTodos = useMemo(() => currentTodo?.todos ?? [], [currentTodo])
  const [locallyDismissedTodo, setLocallyDismissedTodo] = useState<string | null>(null)
  const todoDismissed = useMemo(() => {
    if (!activeSessionId) return false
    if (!currentTodo) return false
    const key = todoDismissalKey(activeSessionId, currentTodo.occurrenceId, currentTodos)
    if (locallyDismissedTodo === key) return true
    if (typeof window === 'undefined') return false
    try {
      return isCompletedTodoDismissed(
        window.localStorage,
        activeSessionId,
        currentTodo.occurrenceId,
        currentTodos,
      )
    } catch {
      return false
    }
  }, [activeSessionId, currentTodo, currentTodos, locallyDismissedTodo])
  const dismissTodo = useCallback(() => {
    if (!activeSessionId || !currentTodo) return
    setLocallyDismissedTodo(
      todoDismissalKey(activeSessionId, currentTodo.occurrenceId, currentTodos),
    )
    if (typeof window === 'undefined') return
    try {
      dismissCompletedTodo(
        window.localStorage,
        activeSessionId,
        currentTodo.occurrenceId,
        currentTodos,
      )
    } catch {
      // The in-memory dismissal above still closes the panel for this page load.
    }
  }, [activeSessionId, currentTodo, currentTodos])

  // Bumped on every composer submit (send OR queue) to jump the transcript to the
  // bottom immediately. Both paths only enqueue — the turn is painted later, when
  // the backend worker picks it up — so a user who had scrolled up needs the jump
  // now to see their message land (or its chip appear in the pending tray).
  const [sendTick, setSendTick] = useState(0)
  const jumpToBottom = () => setSendTick((n) => n + 1)

  // Retry policy for a failed turn's error card. Read-only run logs (task
  // / schedule) retry non-destructively so the audit trail is preserved;
  // interactive chat deletes the failed pair first to keep history clean.
  const onRetry = readOnly ? chat.retryMessagePreserve : chat.retryMessage

  // Rewind truncates the transcript at a checkpoint so the user can re-drive the
  // conversation from there. A read-only run log has no composer to re-drive it
  // with, so the rewind affordance would only destroy the record — the backend
  // refuses it too (rejectReadOnlySession), this just keeps the UI honest.
  const rewind = readOnly ? undefined : onRewind

  // Both flags gate a skeleton, so they go through the same delay: a local
  // backend answers in well under it, and a one-frame skeleton would only flicker.
  // Hooks must run before any early return (rules-of-hooks).
  const showBootSkeleton = useDelayedFlag(bootstrapping)
  const showTranscriptSkeleton = useDelayedFlag(messagesLoading && messages.length === 0)

  // Still bootstrapping → never show the empty state: the session list may well
  // contain a chat to open, and flashing "start a new chat" first is the bug this
  // guards against. Once the load is slow enough, the skeleton takes over.
  if (bootstrapping) {
    return showBootSkeleton ? <ChatSkeleton /> : <div className="flex min-h-0 flex-1" />
  }

  // No active session (and the workspace really has loaded) → show the "start a
  // new chat" screen instead of a bare, disabled composer with no agent selected.
  if (!activeSessionId) {
    return (
      <ChatEmptyState
        agents={sessionStartAgents}
        defaultAgentId={defaultAgentId}
        defaultAgentDeleted={defaultAgentDeleted}
        onNewSession={onNewSession}
        onSelectDefaultAgent={onSelectDefaultAgent}
        onGoToAgents={onGoToAgents}
      />
    )
  }

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      {/* The transcript is swapped for a skeleton while it loads; the bottom stack
          below stays mounted so the composer does not jump. */}
      {showTranscriptSkeleton ? (
        <ChatSkeleton />
      ) : (
        <MessageList
          transcriptPaging={transcriptPaging}
          messages={messages}
          sessionId={activeSessionId ?? undefined}
          pending={chat.activePending}
          // Who the not-yet-arrived turn belongs to, so the standalone "working"
          // bubble carries the agent header BEFORE the first token/AgentStart —
          // otherwise the identity only appears once the reply streams in. The
          // active session's agent (set to sess.agentId on select) is the
          // responder for the pending turn.
          pendingAgentId={activeAgentId ?? undefined}
          agents={allAgents}
          artifacts={artifacts}
          streaming={chat.activeStreaming}
          highlightMessageId={scrollToMsgId}
          onHighlightConsumed={onHighlightConsumed}
          onOpenFile={onOpenFile}
          onOpenArtifact={onOpenArtifact}
          onDeleteMessage={onDeleteMessage}
          onRewind={rewind}
          onRetry={onRetry}
          onFeedback={onFeedback}
          onOpenAgent={onOpenAgent}
          bottomInset={bottomInset}
          scrollBottomSignal={sendTick}
        />
      )}
      {/* Read-only run log (task / schedule): the composer and its
          ask/todo/pending/wake stack are meaningless — a new user turn has no run
          to attach to — so they are hidden entirely and replaced by a thin banner.
          The transcript above (and the header's context/debug/info panels) stay
          fully available. */}
      {readOnly ? (
        <div
          ref={bottomStackRef}
          className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex flex-col gap-1 px-4 pb-2 [&>*]:pointer-events-auto"
        >
          {/* Prompt-cache warmth also matters in a read-only log: a run parked
              on an ask/permission (below) is answered as a suspend-point resolve,
              and that reply reads the still-warm prefix — so the countdown is
              actionable here too. Same gate as the writable stack. */}
          {!chat.activeStreaming && (
            <CacheWarmthStrip messages={messages} summary={transcriptPaging?.summary} />
          )}
          {/* VISIBILITY (A): a durable ask/permission/plan that the run parked
              on. Hidden here before, this made a run silently block on input
              with no on-screen cue. Answering resolves a suspend point (not a new
              user turn), so it is legitimate even in a read-only log. */}
          <InteractionPrompts asks={chat.activeAsks} onAnswer={chat.answerAsk} />
          {/* Read-only status surface the transcript can't replace: the LIVE
              checklist (always the latest todo_write, updating as items tick).
              Display-only — no user input — so it belongs here even though the
              composer stack is gone. */}
          <TodoPanel todos={currentTodos} dismissed={todoDismissed} onDismiss={dismissTodo} />
          {/* CONTROL (C): the self-wake countdown for context — but with its Durdur
              hidden, since disarming an automation's own wake from a read-only log
              would silently derail it. */}
          {chat.activeWakeWait && (
            <WakeWaitBanner
              reason={chat.activeWakeWait.reason}
              fireAt={chat.activeWakeWait.fireAt}
              onCancel={chat.cancelWake}
              hideCancel
            />
          )}
          {/* VISIBILITY + CONTROL (A/C): while an autonomous turn is streaming,
              show it is alive and offer a Durdur. Every read-only log gets it —
              schedule and flow runs included: the server stops an autonomous turn
              through Runtime.CancelSession when it has no live chat run. */}
          {chat.activeStreaming && <RunStatusStrip onStop={chat.stopTurn} />}
        </div>
      ) : (
        /* Floating bottom stack: overlays the transcript so bubbles scroll UNDER
          the composer's transparent→black gradient. pointer-events pass through
          the transparent gaps to the transcript; each child re-enables them. */
        <div
          ref={bottomStackRef}
          className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex flex-col [&>*]:pointer-events-auto"
        >
          {/* Prompt-cache warmth: the one cache surface that can still change the
              outcome — it counts down the warm window BEFORE the next turn is sent.
              Sits at the TOP of the bottom stack so it stays visible above any
              transient panel (todo checklist, pending
              tray, ask/permission prompts). Hidden while a turn streams (the
              countdown is about to reset anyway) and on an empty session (nothing
              is cached yet). */}
          {!chat.activeStreaming && (
            <CacheWarmthStrip messages={messages} summary={transcriptPaging?.summary} />
          )}
          {(chat.activePresence > 1 || chat.activeTyping) && (
            <div className="flex justify-center pb-1">
              <span className="rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-0.5 text-[11px] text-[var(--color-text-dim)] shadow-[var(--shadow-sm)]">
                {chat.activeTyping
                  ? t('chatView.anotherWindowTyping')
                  : chat.activeAsk
                    ? t('chatView.windowsOpenAsk', { count: chat.activePresence })
                    : t('chatView.windowsOpen', { count: chat.activePresence })}
              </span>
            </div>
          )}
          <InteractionPrompts asks={chat.activeAsks} onAnswer={chat.answerAsk} />
          <TodoPanel todos={currentTodos} dismissed={todoDismissed} onDismiss={dismissTodo} />
          <PendingTray
            items={chat.activeQueued}
            onRemove={chat.removePending}
            onSendNext={chat.sendQueuedNext}
            onSteerNow={chat.steerQueued}
            canSteer={chat.activeSteerable}
            turnRunning={chat.activeStreaming}
            onClear={chat.clearQueue}
          />
          {chat.activeWakeWait && (
            <WakeWaitBanner
              reason={chat.activeWakeWait.reason}
              fireAt={chat.activeWakeWait.fireAt}
              onCancel={chat.cancelWake}
            />
          )}
          <Composer
            key={composerKey}
            disabled={!activeSessionId}
            sessionId={activeSessionId ?? undefined}
            focusSessionId={focusSessionId}
            streaming={chat.activeStreaming}
            waiting={!!chat.activeWakeWait}
            onCancelWait={chat.cancelWake}
            onSend={(text, attachments) => {
              jumpToBottom()
              return chat.sendMessage(text, undefined, attachments)
            }}
            onStop={chat.stopTurn}
            onInterrupt={chat.interruptTurn}
            onQueue={(text, attachments) => {
              jumpToBottom()
              chat.queueMessage(text, attachments)
            }}
            onSteer={chat.steerTurn}
            canSteer={chat.activeSteerable}
            onTyping={chat.notifyTyping}
            thinkingLevel={chat.thinkingLevel}
            onThinkingLevelChange={chat.setThinkingLevel}
            permissionMode={chat.permissionMode}
            onPermissionModeChange={chat.setPermissionMode}
            agentId={activeAgentId ?? ''}
            onAgentChange={onAgentChange}
            agents={agents}
            selectableAgents={selectableAgents}
            commands={chat.chatCommands}
            artifacts={artifacts}
          />
        </div>
      )}
      {chat.rewindOpen && rewind && (
        <RewindDialog
          messages={messages}
          paging={transcriptPaging}
          onClose={chat.closeRewind}
          onRewind={rewind}
        />
      )}
    </div>
  )
}
