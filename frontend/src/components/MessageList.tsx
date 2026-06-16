import { useEffect, useRef } from 'react'
import type { Agent, Message } from '../types'
import { Markdown } from './markdown/Markdown'
import { TurnSteps, parseSteps } from './chat/TurnSteps'
import { ThinkingBlock } from './chat/ThinkingBlock'
import { UserBubble } from './chat/UserBubble'
import { AgentAvatar } from './AgentAvatar'

interface Props {
  messages: Message[]
  pending: boolean
  agents: Agent[]
  onOpenFile?: (path: string) => void
}

// Bouncing-dots "working" indicator shown while a turn is in flight.
function WorkingDots() {
  return (
    <span className="inline-flex gap-1 text-[var(--color-text-dim)]">
      <span className="animate-bounce">●</span>
      <span className="animate-bounce [animation-delay:0.15s]">●</span>
      <span className="animate-bounce [animation-delay:0.3s]">●</span>
    </span>
  )
}

export function MessageList({ messages, pending, agents, onOpenFile }: Props) {
  const endRef = useRef<HTMLDivElement>(null)
  const agentById = (id?: string) => (id ? agents.find((a) => a.id === id) : undefined)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, pending])

  // When a live assistant bubble is already present (streaming), the standalone
  // pending bubble would duplicate it — suppress it in that case.
  const last = messages[messages.length - 1]
  const showStandalonePending = pending && (!last || last.role === 'user')

  return (
    <div className="flex-1 overflow-y-auto px-6 py-6">
      <div className="flex w-full flex-col gap-4">
        {messages.map((m) =>
          m.role === 'user' ? (
            <UserBubble key={m.id} text={m.text} agents={agents} />
          ) : (
            // Assistant turn: who answered (avatar+name) + activity trace above
            // the final markdown answer — the External Agent chat layout.
            <div key={m.id} className="flex w-full justify-start">
              <div className="w-full min-w-0 rounded-2xl bg-[var(--color-surface-2)] px-4 py-3 text-[var(--color-text)]">
                {agentById(m.agentId) && (
                  <div className="mb-1.5 flex items-center gap-2">
                    <AgentAvatar agent={agentById(m.agentId)!} size={20} />
                    <span className="text-xs font-medium text-[var(--color-text-dim)]">
                      {agentById(m.agentId)!.name}
                    </span>
                  </div>
                )}
                {m.reasoningContent && <ThinkingBlock text={m.reasoningContent} />}
                <TurnSteps steps={parseSteps(m.steps)} onOpenFile={onOpenFile} />
                {m.text.trim() && <Markdown onOpenFile={onOpenFile}>{m.text}</Markdown>}
                {/* Empty live assistant bubble → show the working indicator. */}
                {!m.text.trim() &&
                  !m.reasoningContent &&
                  parseSteps(m.steps).length === 0 && <WorkingDots />}
              </div>
            </div>
          ),
        )}

        {showStandalonePending && (
          <div className="flex justify-start">
            <div className="rounded-2xl bg-[var(--color-surface-2)] px-4 py-3 text-sm">
              <WorkingDots />
            </div>
          </div>
        )}

        {messages.length === 0 && !pending && (
          <div className="mt-20 text-center text-[var(--color-text-dim)]">
            <p className="text-lg">Sohbete başla</p>
            <p className="mt-1 text-sm">Aşağıya bir mesaj yaz.</p>
          </div>
        )}

        <div ref={endRef} />
      </div>
    </div>
  )
}
