import { useEffect, useRef, useState } from 'react'
import { Trash2, X } from 'lucide-react'
import type { Agent, Message } from '../../types'
import { Markdown } from '../markdown/Markdown'
import { TurnSteps, parseSteps } from './TurnSteps'
import { ThinkingBlock } from './ThinkingBlock'
import { UserBubble } from './UserBubble'
import { MessageTime, TurnDuration, LiveTimer } from './MessageMeta'
import { AgentAvatar } from '../agents/AgentAvatar'

interface Props {
  messages: Message[]
  pending: boolean
  agents: Agent[]
  // True when the open session has a turn currently streaming — drives the live
  // elapsed timer on the last (in-flight) assistant bubble.
  streaming?: boolean
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
  // Delete a single message (prune a mistaken/test one). Shown on row hover.
  onDeleteMessage?: (id: string) => void
}

// DeleteButton is the small destructive control revealed on message hover. It
// uses a two-step inline confirm (🗑 → "Sil" / ✕) instead of a blocking native
// dialog, so deleting a message stays in the UI.
function DeleteButton({ onClick }: { onClick: () => void }) {
  const [armed, setArmed] = useState(false)
  if (armed) {
    return (
      <span className="flex shrink-0 items-center gap-1">
        <button
          onClick={() => {
            setArmed(false)
            onClick()
          }}
          className="rounded px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)]"
        >
          Sil
        </button>
        <button
          onClick={() => setArmed(false)}
          title="Vazgeç"
          className="rounded px-1 py-0.5 text-[10px] text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
        >
          <X size={12} />
        </button>
      </span>
    )
  }
  return (
    <button
      onClick={() => setArmed(true)}
      title="Mesajı sil"
      className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-danger)] group-hover:opacity-100"
    >
      <Trash2 size={14} />
    </button>
  )
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

export function MessageList({ messages, pending, agents, streaming, onOpenFile, onOpenArtifact, onDeleteMessage }: Props) {
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
          const prev = messages[i - 1]
          // The in-flight assistant bubble is the last message while streaming;
          // its createdAt marks the turn start, so a live timer counts up from it.
          const isLastLive = !!streaming && i === messages.length - 1 && m.role === 'assistant'
          // For a completed assistant turn, working time ≈ this message's
          // createdAt (turn end) minus the triggering user message's (turn
          // start). Only meaningful when the previous message is the user's —
          // injected summaries or consecutive assistant turns would otherwise
          // report idle wall-clock gaps, not real work.
          const workedSec =
            m.role === 'assistant' && prev?.role === 'user' ? m.createdAt - prev.createdAt : 0
          // Activity trace + how many of its steps are tool usages (for the
          // per-message collapse toggle label).
          const steps = m.role === 'assistant' ? parseSteps(m.steps) : []
          const toolCount = steps.filter((st) =>
            st.kind === 'tool' || st.kind === 'diff' || st.kind === 'todo',
          ).length
          const toolsHidden = collapsedTools.has(m.id)
          return m.role === 'user' ? (
            <div key={m.id} className="group flex flex-col gap-1">
              <UserBubble text={m.text} agents={agents} attachments={m.attachments} />
              <div className="flex items-center justify-end gap-2 pr-1">
                {onDeleteMessage && <DeleteButton onClick={() => onDeleteMessage(m.id)} />}
                <MessageTime unixSec={m.createdAt} />
              </div>
            </div>
          ) : (
            // Assistant turn: who answered (avatar+name) + activity trace above
            // the final markdown answer — the External Agent chat layout.
            <div key={m.id} className="group flex flex-col gap-1">
              <div className="flex w-full justify-start">
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
                  {/* Per-message toggle to hide/show the tool-activity trace. */}
                  {steps.length > 0 && (
                    <button
                      onClick={() => toggleTools(m.id)}
                      title={toolsHidden ? 'Araç adımlarını göster' : 'Araç adımlarını gizle'}
                      className="mb-1.5 flex items-center gap-1 text-[10px] text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                    >
                      <span>🔧</span>
                      <span>{toolCount > 0 ? `${toolCount} araç` : `${steps.length} adım`}</span>
                      <span className="opacity-70">{toolsHidden ? '▸ göster' : '▾ gizle'}</span>
                    </button>
                  )}
                  {!toolsHidden && (
                    <TurnSteps steps={steps} onOpenFile={onOpenFile} onOpenArtifact={onOpenArtifact} />
                  )}
                  {m.text.trim() && <Markdown onOpenFile={onOpenFile}>{m.text}</Markdown>}
                  {/* Empty live assistant bubble → show the working indicator. */}
                  {!m.text.trim() && !m.reasoningContent && steps.length === 0 && !m.interrupted && <WorkingDots />}
                  {/* Reply recovered from a mid-stream server crash: flag it as cut off. */}
                  {m.interrupted && (
                    <div className="mt-2 flex items-center gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-warning,#d97706)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning,#d97706)_12%,transparent)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
                      <span>⚠</span>
                      <span>Bu yanıt yarıda kesildi (sunucu yeniden başladı). İçerik eksik olabilir.</span>
                    </div>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-2 pl-1">
                <MessageTime unixSec={m.createdAt} />
                {isLastLive ? (
                  <LiveTimer startUnixSec={m.createdAt} />
                ) : (
                  <TurnDuration seconds={workedSec} />
                )}
                {onDeleteMessage && !isLastLive && (
                  <DeleteButton onClick={() => onDeleteMessage(m.id)} />
                )}
              </div>
            </div>
          )
        })}

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

        <div />
      </div>
    </div>
  )
}
