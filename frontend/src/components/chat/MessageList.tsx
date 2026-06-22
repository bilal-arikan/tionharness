import { useEffect, useRef, useState } from 'react'
import type { Agent, Artifact, Message } from '../../types'
import { MessageTime, LiveTimer } from './MessageMeta'
import { AgentHeader } from './AgentHeader'
import { WorkingDots } from './WorkingDots'
import { AutoPromptNote } from './AutoPromptNote'
import { UserTurn } from './UserTurn'
import { AssistantTurn } from './AssistantTurn'

interface Props {
  messages: Message[]
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
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
  // Delete a single message (prune a mistaken/test one). Shown on row hover.
  onDeleteMessage?: (id: string) => void
  // Retry the failed turn behind an assistant bubble that errored.
  onRetry?: (id: string) => void
}

// MessageList is the scrolling transcript. It owns scroll-pinning and per-message
// tool-trace collapse, then delegates each row to UserTurn / AutoPromptNote /
// AssistantTurn. The standalone pending bubble covers polled views with no live
// streaming placeholder.
export function MessageList({
  messages,
  pending,
  pendingAgentId,
  agents,
  artifacts,
  streaming,
  onOpenFile,
  onOpenArtifact,
  onDeleteMessage,
  onRetry,
}: Props) {
  const scrollRef = useRef<HTMLDivElement>(null)
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

  function onScroll() {
    const el = scrollRef.current
    if (!el) return
    const distance = el.scrollHeight - el.scrollTop - el.clientHeight
    pinnedRef.current = distance < 80
  }

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    if (prevFirstId.current !== firstId) {
      // Session changed: always land at the bottom of the new transcript.
      prevFirstId.current = firstId
      pinnedRef.current = true
    }
    if (!pinnedRef.current) return
    // Jump instantly (not smooth): rapid streaming deltas update `messages` on
    // every token, and a smooth animation restarted each delta never settles —
    // the symptom where the live reply seems to vanish until the turn finishes.
    el.scrollTop = el.scrollHeight
  }, [messages, pending, firstId])

  // When a live assistant bubble is already present (streaming), the standalone
  // pending bubble would duplicate it — suppress it in that case.
  const last = messages[messages.length - 1]
  const showStandalonePending = pending && (!last || last.role === 'user')

  return (
    <div ref={scrollRef} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto px-6 py-6">
      <div className="flex w-full flex-col gap-4">
        {messages.map((m, i) => {
          if (m.role === 'user') {
            return m.origin ? (
              <AutoPromptNote key={m.id} message={m} onDelete={onDeleteMessage} />
            ) : (
              <UserTurn
                key={m.id}
                message={m}
                agents={agents}
                artifacts={artifacts}
                onDelete={onDeleteMessage}
                onOpenArtifact={onOpenArtifact}
              />
            )
          }
          // The in-flight assistant bubble is the last message while streaming; its
          // createdAt marks the turn start, so a live timer counts up from it.
          const isLastLive = !!streaming && i === messages.length - 1
          // Completed-turn working time ≈ this message's createdAt (turn end) minus
          // the triggering user message's (turn start). Only meaningful when the
          // previous message is the user's — injected summaries or consecutive
          // assistant turns would otherwise report idle gaps, not real work.
          const prev = messages[i - 1]
          const workedSec = prev?.role === 'user' ? m.createdAt - prev.createdAt : 0
          return (
            <AssistantTurn
              key={m.id}
              message={m}
              agent={agentById(m.agentId)}
              isLastLive={isLastLive}
              workedSec={workedSec}
              toolsHidden={collapsedTools.has(m.id)}
              onToggleTools={toggleTools}
              onOpenFile={onOpenFile}
              onOpenArtifact={onOpenArtifact}
              onDelete={onDeleteMessage}
              onRetry={onRetry}
            />
          )
        })}

        {showStandalonePending && (
          <div className="group flex flex-col gap-1">
            <div className="flex w-full justify-start">
              <div className="w-full min-w-0 rounded-2xl bg-[color-mix(in_srgb,var(--color-surface-2)_65%,var(--color-bg))] px-4 py-3">
                <AgentHeader agent={agentById(pendingAgentId)} />
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
  )
}
