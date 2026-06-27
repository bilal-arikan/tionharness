import { RotateCcw, ThumbsUp, ThumbsDown } from 'lucide-react'
import type { Agent, Message } from '../../types'
import { Markdown } from '../markdown/Markdown'
import { TurnSteps, parseSteps } from './TurnSteps'
import { ThinkingBlock } from './ThinkingBlock'
import { MessageTime, TurnDuration, LiveTimer } from './MessageMeta'
import { AgentHeader } from './AgentHeader'
import { WorkingDots } from './WorkingDots'
import { DeleteButton } from './DeleteButton'

interface Props {
  message: Message
  agent?: Agent
  // isLastLive: this is the in-flight assistant bubble (last message while
  // streaming) — drives the live elapsed timer and suppresses retry/delete.
  isLastLive: boolean
  // workedSec: completed-turn working time (end − triggering user message start);
  // 0 when not meaningful (no preceding user turn).
  workedSec: number
  toolsHidden: boolean
  onToggleTools: (id: string) => void
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
  onDelete?: (id: string) => void
  onRetry?: (id: string) => void
  // Rate this assistant turn: rating +1 (up) / -1 (down) / 0 (clear). Absent →
  // the thumbs are not shown (e.g. the live in-flight bubble).
  onFeedback?: (id: string, rating: number) => void
}

// fmtTok renders a token count compactly (1234 → "1.2k").
function fmtTok(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return `${n}`
}

// stopReasonLabel maps a provider stop reason to a short Turkish note, or "" when
// it is the normal end (end_turn / tool_use) that needs no badge.
function stopReasonLabel(reason?: string): string {
  switch (reason) {
    case 'max_tokens':
      return 'çıktı limiti — kesilmiş olabilir'
    case 'refusal':
      return 'model reddetti'
    case 'stop_sequence':
      return 'durdurma dizisi'
    default:
      return ''
  }
}

// AssistantTurn renders one assistant message: the responding agent's header, an
// optional reasoning block, a collapsible tool-activity trace, the markdown answer
// (or a working indicator while empty), and a meta row with timing + retry/delete.
export function AssistantTurn({
  message: m,
  agent,
  isLastLive,
  workedSec,
  toolsHidden,
  onToggleTools,
  onOpenFile,
  onOpenArtifact,
  onDelete,
  onRetry,
  onFeedback,
}: Props) {
  const steps = parseSteps(m.steps)
  const stopNote = stopReasonLabel(m.stopReason)
  const rating = m.feedback?.rating ?? 0
  const toolCount = steps.filter(
    (st) => st.kind === 'tool' || st.kind === 'diff' || st.kind === 'todo',
  ).length
  // A turn that ended in an error step can be retried (resends the triggering user
  // message). Not offered while still streaming.
  const canRetry = !!onRetry && steps.some((st) => st.kind === 'error') && !isLastLive

  return (
    <div className="group flex flex-col gap-1">
      <div className="flex w-full justify-start">
        <div className="w-full min-w-0 rounded-2xl bg-[color-mix(in_srgb,var(--color-surface-2)_65%,var(--color-bg))] px-4 py-3 text-[var(--color-text)]">
          <AgentHeader agent={agent} />
          {m.reasoningContent && <ThinkingBlock text={m.reasoningContent} />}
          {/* Per-message toggle to hide/show the tool-activity trace. */}
          {steps.length > 0 && (
            <button
              onClick={() => onToggleTools(m.id)}
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
          {/* Working indicator: while the turn is in flight (isLastLive) keep the
              bouncing dots visible — through tool steps and partial text — until
              the agent finishes the whole turn. Hidden once it completes or was
              interrupted/stopped. Gets a small top margin when content precedes it. */}
          {isLastLive && !m.interrupted && !m.cancelled && (
            <div className={steps.length > 0 || m.text.trim() || m.reasoningContent ? 'mt-1.5' : undefined}>
              <WorkingDots />
            </div>
          )}
          {/* Reply recovered from a mid-stream server crash: flag it as cut off. */}
          {m.interrupted && (
            <div className="mt-2 flex items-center gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-warning,#d97706)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning,#d97706)_12%,transparent)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
              <span>⚠</span>
              <span>Bu yanıt yarıda kesildi (sunucu yeniden başladı). İçerik eksik olabilir.</span>
            </div>
          )}
          {/* Reply the user stopped mid-stream (intentional, not a crash). */}
          {m.cancelled && (
            <div className="mt-2 flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
              <span>⏹</span>
              <span>Bu yanıt manuel olarak durduruldu. O ana kadarki içerik korundu.</span>
            </div>
          )}
          {/* Stop-reason warning (truncation / refusal). */}
          {stopNote && (
            <div className="mt-2 flex items-center gap-1.5 text-[11px] text-[var(--color-warning,#d97706)]">
              <span>⚠</span>
              <span>{stopNote}</span>
            </div>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2 pl-1">
        <MessageTime unixSec={m.createdAt} />
        {isLastLive ? <LiveTimer startUnixSec={m.createdAt} /> : <TurnDuration seconds={workedSec} />}
        {/* Per-turn metadata: model + token usage (the per-bubble cost). */}
        {(m.model || m.usage) && (
          <span className="flex items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]" title="Bu turun modeli ve token tüketimi">
            {m.model && <span className="font-mono opacity-80">{m.model}</span>}
            {m.usage && (
              <span className="font-mono">
                ↑{fmtTok(m.usage.in)} ↓{fmtTok(m.usage.out)}
                {!!m.usage.cacheRead && <span className="opacity-60"> ⚡{fmtTok(m.usage.cacheRead)}</span>}
              </span>
            )}
          </span>
        )}
        {/* Feedback thumbs (not on the live bubble). */}
        {onFeedback && !isLastLive && (
          <span className="flex items-center gap-0.5">
            <button
              onClick={() => onFeedback(m.id, rating === 1 ? 0 : 1)}
              title="Bu yanıt iyi"
              className={`rounded p-0.5 transition hover:text-[var(--color-success)] ${rating === 1 ? 'text-[var(--color-success)]' : 'text-[var(--color-text-dim)] opacity-0 group-hover:opacity-100'}`}
            >
              <ThumbsUp size={12} />
            </button>
            <button
              onClick={() => onFeedback(m.id, rating === -1 ? 0 : -1)}
              title="Bu yanıt kötü"
              className={`rounded p-0.5 transition hover:text-[var(--color-danger)] ${rating === -1 ? 'text-[var(--color-danger)]' : 'text-[var(--color-text-dim)] opacity-0 group-hover:opacity-100'}`}
            >
              <ThumbsDown size={12} />
            </button>
          </span>
        )}
        {canRetry && (
          <button
            onClick={() => onRetry!(m.id)}
            title="Bu turu yeniden dene"
            className="flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)]"
          >
            <RotateCcw size={12} />
            Yeniden dene
          </button>
        )}
        {onDelete && !isLastLive && <DeleteButton onClick={() => onDelete(m.id)} />}
      </div>
    </div>
  )
}
