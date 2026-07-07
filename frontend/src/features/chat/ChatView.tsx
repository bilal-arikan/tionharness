// ChatView renders the chat screen's main column: the transcript plus the
// floating bottom stack (ask/permission/plan prompts, todo panel, pending tray,
// wake banner, composer) and the rewind dialog. Extracted from App.tsx so the
// shell only composes; all chat-turn machinery arrives via the `chat` handle.
import { useEffect, useMemo, useRef, useState, type ComponentProps } from 'react'
import type { Agent, Artifact, Message } from '@/types'
import { MessageList } from './MessageList'
import { Composer } from './Composer'
import { ChatEmptyState } from './ChatEmptyState'
import { RewindDialog } from './RewindDialog'
import { AskPrompt } from './AskPrompt'
import { PermissionPrompt } from './PermissionPrompt'
import { PlanPrompt } from './PlanPrompt'
import { PendingTray } from './PendingTray'
import { WakeWaitBanner } from './WakeWaitBanner'
import { TodoPanel } from './TodoPanel'
import { latestTodos } from './todos'
import type { useChatStream } from './useChatStream'

export interface ChatViewProps {
  chat: ReturnType<typeof useChatStream>
  messages: Message[]
  agents: Agent[]
  artifacts: Artifact[]
  activeSessionId: string | null
  activeAgentId: string | null
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

  // No active session → show the "start a new chat" screen instead of a bare,
  // disabled composer with no agent selected.
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
      />
      {/* Floating bottom stack: overlays the transcript so bubbles scroll UNDER
          the composer's transparent→black gradient. pointer-events pass through
          the transparent gaps to the transcript; each child re-enables them. */}
      <div
        ref={bottomStackRef}
        className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex flex-col [&>*]:pointer-events-auto"
      >
        {chat.activeAsk &&
          (chat.activeAsk.kind === 'permission' ? (
            <PermissionPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
          ) : chat.activeAsk.kind === 'plan' ? (
            <PlanPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
          ) : (
            <AskPrompt ask={chat.activeAsk} onAnswer={chat.answerAsk} />
          ))}
        <TodoPanel todos={currentTodos} />
        <PendingTray items={chat.activeQueued} onRemove={chat.removePending} />
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
          onSend={(text, attachments) => chat.sendMessage(text, undefined, attachments)}
          onStop={chat.stopTurn}
          onInterrupt={chat.interruptTurn}
          onQueue={chat.queueMessage}
          onSteer={chat.steerTurn}
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
      {chat.rewindOpen && (
        <RewindDialog messages={messages} onClose={chat.closeRewind} onRewind={onRewind} />
      )}
    </div>
  )
}
