// ChatView renders the chat screen's main column: the transcript plus the
// floating bottom stack (ask/permission/plan prompts, todo panel, pending tray,
// wake banner, composer) and the rewind dialog. Extracted from App.tsx so the
// shell only composes; all chat-turn machinery arrives via the `chat` handle.
import { useEffect, useMemo, useRef, useState, type ComponentProps } from 'react'
import type { Agent, Artifact, Message } from '@/types'
import { MessageList } from './MessageList'
import { Composer } from './Composer'
import { ChatEmptyState } from './ChatEmptyState'
import { ChatSkeleton } from './ChatSkeleton'
import { RewindDialog } from './RewindDialog'
import { AskPrompt } from './AskPrompt'
import { PermissionPrompt } from './PermissionPrompt'
import { PlanPrompt } from './PlanPrompt'
import { PendingTray } from './PendingTray'
import { WakeWaitBanner } from './WakeWaitBanner'
import { WorkerWaitBanner } from './WorkerWaitBanner'
import { useRunningWorkers } from './useRunningWorkers'
import { TodoPanel } from './TodoPanel'
import { latestTodos } from './todos'
import { useDelayedFlag } from '@/shared/hooks/useDelayedFlag'
import type { useChatStream } from './useChatStream'

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
  // When true the session is a read-only run log (task / flow / schedule): the
  // composer and its ask/todo/pending/wake stack are hidden and a thin banner is
  // shown instead. The transcript, context preview, debug and info panels stay
  // fully available.
  readOnly: boolean
  // The open session's coordination role ('coordinator' | 'worker' | ''). Only a
  // coordinator gets the running-worker banner above the composer.
  sessionRole?: string
  // Opens another session's transcript (used to jump into a running worker).
  onSelectSession?: (id: string) => void
  // Empty-state ("Yeni sohbete başla") wiring, used when no session is active.
  defaultAgentId: string | null
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
  artifacts,
  activeSessionId,
  activeAgentId,
  bootstrapping,
  messagesLoading,
  readOnly,
  sessionRole,
  onSelectSession,
  defaultAgentId,
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
  }, [activeSessionId])

  // The active session's current checklist (latest todo_write across the
  // transcript). Pinned above the composer and updated as the agent ticks items.
  const currentTodos = useMemo(() => latestTodos(messages), [messages])

  // Bumped on every composer submit (send OR queue) to jump the transcript to the
  // bottom immediately. Both paths only enqueue — the turn is painted later, when
  // the backend worker picks it up — so a user who had scrolled up needs the jump
  // now to see their message land (or its chip appear in the pending tray).
  const [sendTick, setSendTick] = useState(0)
  const jumpToBottom = () => setSendTick((n) => n + 1)

  // Coordinator sessions: the live worker roster, so the chat can show that it is
  // waiting on background workers rather than looking idle. Disabled (and never
  // polled) for ordinary and worker sessions.
  const workers = useRunningWorkers(
    activeSessionId,
    sessionRole === 'coordinator',
    chat.activeStreaming,
  )
  const runningWorkers = useMemo(() => workers.filter((w) => w.running), [workers])

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
        agents={agents}
        defaultAgentId={defaultAgentId}
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
          messages={messages}
          sessionId={activeSessionId ?? undefined}
          pending={chat.activePending}
          agents={agents}
          artifacts={artifacts}
          streaming={chat.activeStreaming}
          highlightMessageId={scrollToMsgId}
          onHighlightConsumed={onHighlightConsumed}
          onOpenFile={onOpenFile}
          onOpenArtifact={onOpenArtifact}
          onDeleteMessage={onDeleteMessage}
          onRewind={onRewind}
          onRetry={chat.retryMessage}
          onFeedback={onFeedback}
          onOpenAgent={onOpenAgent}
          bottomInset={bottomInset}
          scrollBottomSignal={sendTick}
        />
      )}
      {/* Read-only run log (task / flow / schedule): the composer and its
          ask/todo/pending/wake stack are meaningless — a new user turn has no run
          to attach to — so they are hidden entirely and replaced by a thin banner.
          The transcript above (and the header's context/debug/info panels) stay
          fully available. */}
      {readOnly ? (
        <div
          ref={bottomStackRef}
          className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex justify-center px-4 pb-4"
        >
          <div className="pointer-events-auto rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-xs text-[var(--color-text-dim)] shadow-[var(--shadow-sm)]">
            Bu oturum salt-okunurdur (görev / akış / zamanlama günlüğü).
          </div>
        </div>
      ) : (
      /* Floating bottom stack: overlays the transcript so bubbles scroll UNDER
          the composer's transparent→black gradient. pointer-events pass through
          the transparent gaps to the transcript; each child re-enables them. */
      <div
        ref={bottomStackRef}
        className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex flex-col [&>*]:pointer-events-auto"
      >
        {(chat.activePresence > 1 || chat.activeTyping) && (
          <div className="flex justify-center pb-1">
            <span className="rounded-full border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-0.5 text-[11px] text-[var(--color-text-dim)] shadow-[var(--shadow-sm)]">
              {chat.activeTyping
                ? 'Başka bir pencere yazıyor…'
                : chat.activeAsk
                  ? `${chat.activePresence} pencerede açık — ilk cevaplayan geçerli`
                  : `Bu oturum ${chat.activePresence} pencerede açık`}
            </span>
          </div>
        )}
        {chat.activeAsk &&
          (chat.activeAsk.kind === 'permission' ? (
            <PermissionPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
          ) : chat.activeAsk.kind === 'plan' ? (
            <PlanPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
          ) : (
            <AskPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
          ))}
        <TodoPanel todos={currentTodos} />
        <PendingTray
          items={chat.activeQueued}
          onRemove={chat.removePending}
          onSendNext={chat.sendQueuedNext}
          onClear={chat.clearQueue}
        />
        <WorkerWaitBanner
          workers={runningWorkers}
          doneCount={workers.length - runningWorkers.length}
          onSelectSession={onSelectSession}
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
          onQueue={(text) => {
            jumpToBottom()
            chat.queueMessage(text)
          }}
          onSteer={chat.steerTurn}
          onTyping={chat.notifyTyping}
          thinkingLevel={chat.thinkingLevel}
          onThinkingLevelChange={chat.setThinkingLevel}
          permissionMode={chat.permissionMode}
          onPermissionModeChange={chat.setPermissionMode}
          agentId={activeAgentId ?? ''}
          onAgentChange={onAgentChange}
          agents={agents}
          commands={chat.chatCommands}
          artifacts={artifacts}
        />
      </div>
      )}
      {chat.rewindOpen && (
        <RewindDialog messages={messages} onClose={chat.closeRewind} onRewind={onRewind} />
      )}
    </div>
  )
}
