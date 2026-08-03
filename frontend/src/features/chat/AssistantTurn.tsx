import { memo, useEffect, useMemo, useState } from 'react'
import { RotateCcw, ThumbsUp, ThumbsDown, Volume2, Square } from 'lucide-react'
import type { Agent, Message } from '@/types'
import { api } from '@/api'
import { speak, stopSpeaking, ttsAvailable } from '@/shared/lib/tts'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { TtsVolumeSlider } from './TtsVolumeSlider'
import { TurnSteps, parseSteps, stepTruncated } from './TurnSteps'
import { ThinkingBlock } from './ThinkingBlock'
import { MessageTime, TurnDuration, LiveTimer } from './MessageMeta'
import { AgentHeader } from './AgentHeader'
import { DirectionBadge } from './DirectionBadge'
import { WorkingDots } from './WorkingDots'
import { DeleteButton } from './DeleteButton'
import { MessageDebugPanel } from './MessageDebugPanel'
import { ChangesButton } from './ChangesButton'
import { hasFileChanges } from '@/shared/lib/fileChanges'
import { modelDisplayName } from '@/shared/lib/modelLabel'
import {
  ACTION_CLUSTER,
  META_CLUSTER,
  TURN_FOOTER,
  actionChip,
  actionChipActive,
} from './messageActions'

interface Props {
  message: Message
  agent?: Agent
  // Session this turn belongs to — enables the per-message debug panel (fetches
  // the turn's spend/latency by reply id). Absent in previews with no session.
  sessionId?: string
  // isLastLive: this is the in-flight assistant bubble (last message while
  // streaming) — drives the live elapsed timer and suppresses retry/delete.
  isLastLive: boolean
  // workedMs: completed-turn wall clock in milliseconds, as MEASURED BY THE
  // SERVER (Message.durationMs). 0 when the backend did not time this turn.
  workedMs: number
  // workedDerived: the value above is a client-side reconstruction from message
  // timestamps, not a server measurement — only true for messages persisted
  // before durationMs existed. Renders the duration as approximate.
  workedDerived: boolean
  toolsHidden: boolean
  onToggleTools: (id: string) => void
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
  onDelete?: (id: string) => void
  onRetry?: (id: string) => void
  // Rate this assistant turn: rating +1 (up) / -1 (down) / 0 (clear). Absent →
  // the thumbs are not shown (e.g. the live in-flight bubble).
  onFeedback?: (id: string, rating: number) => void
  // Open this agent's settings page — fired when its header (avatar/name) is clicked.
  onOpenAgent?: (id: string) => void
  // "→ <name>" direction cue when this reply is addressed to a specific participant
  // in a multi-participant thread (generic participant model). Undefined = no cue.
  recipientLabel?: string
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
export const AssistantTurn = memo(function AssistantTurn({
  message: m,
  agent,
  sessionId,
  isLastLive,
  workedMs,
  workedDerived,
  toolsHidden,
  onToggleTools,
  onOpenFile,
  onOpenArtifact,
  onDelete,
  onRetry,
  onFeedback,
  onOpenAgent,
  recipientLabel,
}: Props) {
  // The full trace fetched on demand, replacing the server-trimmed one. Keyed
  // implicitly by this bubble's identity; a session switch remounts the row.
  const [fullSteps, setFullSteps] = useState<string | null>(null)
  const [loadingFull, setLoadingFull] = useState(false)
  const [fullError, setFullError] = useState(false)
  // useMemo, not a bare call: `steps` is a JSON.parse of a string that can run
  // to hundreds of KB on a worker turn, and this component re-renders on every
  // streaming delta of the LIVE turn. Memoizing also gives TurnSteps a stable
  // array reference so its own memo actually holds.
  const steps = useMemo(() => parseSteps(fullSteps ?? m.steps), [fullSteps, m.steps])
  // Whether the server cut any payload in this turn's trace.
  const truncated = useMemo(() => !fullSteps && steps.some(stepTruncated), [fullSteps, steps])
  const loadFullSteps = async () => {
    if (!sessionId || loadingFull) return
    setLoadingFull(true)
    setFullError(false)
    try {
      setFullSteps(await api.getMessageSteps(sessionId, m.id))
    } catch {
      setFullError(true)
    } finally {
      setLoadingFull(false)
    }
  }
  const stopNote = stopReasonLabel(m.stopReason)
  const rating = m.feedback?.rating ?? 0
  const toolCount = steps.filter(
    (st) => st.kind === 'tool' || st.kind === 'diff' || st.kind === 'todo',
  ).length
  // A turn that ended in an error step can be retried (resends the triggering user
  // message). Not offered while still streaming.
  const canRetry = !!onRetry && steps.some((st) => st.kind === 'error') && !isLastLive

  // Manual read-aloud (TTS) of this reply's prose. Local "speaking" state drives
  // the play/stop icon; it resets when the utterance ends. Offered only when the
  // browser supports speech synthesis, the turn is finished, and it has text.
  const [speaking, setSpeaking] = useState(false)
  const canSpeak = ttsAvailable() && !isLastLive && m.text.trim().length > 0
  const toggleSpeak = () => {
    if (speaking) {
      stopSpeaking()
      setSpeaking(false)
      return
    }
    setSpeaking(true)
    speak(m.text, () => setSpeaking(false))
  }
  // Stop this bubble's speech if it unmounts mid-utterance.
  useEffect(
    () => () => {
      if (speaking) stopSpeaking()
    },
    [speaking],
  )

  // Did this turn mutate any file? Drives the bulk file-changes chip. Cheap
  // probe rather than the full extraction — the modal does that work when
  // opened. Skipped on the live bubble: the trace grows with every delta, and
  // the diffs are worth browsing only once the turn has settled.
  const mutatedFiles = useMemo(() => hasFileChanges(steps), [steps])
  const showChanges = mutatedFiles && !isLastLive

  // Whether the in-bubble action row has anything to show at all (a live bubble
  // offers none of them, so it stays clean while streaming).
  const hasActions =
    showChanges ||
    canSpeak ||
    (!!onFeedback && !isLastLive) ||
    canRetry ||
    (!!onDelete && !isLastLive)

  return (
    <div className="group flex flex-col gap-1">
      <div className="flex w-full justify-start">
        <div className="w-full min-w-0 rounded-2xl bg-[color-mix(in_srgb,var(--color-surface-2)_65%,var(--color-bg))] px-4 py-3 text-[var(--color-text)]">
          {/* Top-left affordance: per-message debug/cost/performance panel. */}
          <div className="flex items-center gap-1.5">
            {sessionId && !isLastLive && <MessageDebugPanel sessionId={sessionId} turnId={m.id} />}
            <AgentHeader agent={agent} onOpenAgent={onOpenAgent} />
            <DirectionBadge label={recipientLabel} />
          </div>
          {m.reasoningContent && <ThinkingBlock text={m.reasoningContent} />}
          {/* Per-message toggle to hide/show the tool-activity trace. */}
          {steps.length > 0 && (
            <div className="mb-1.5 flex items-center gap-2">
              <button
                onClick={() => onToggleTools(m.id)}
                title={toolsHidden ? 'Araç adımlarını göster' : 'Araç adımlarını gizle'}
                className="flex items-center gap-1 text-[10px] text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
              >
                <span>🔧</span>
                <span>{toolCount > 0 ? `${toolCount} araç` : `${steps.length} adım`}</span>
                <span className="opacity-70">{toolsHidden ? '▸ göster' : '▾ gizle'}</span>
              </button>
              {/* The transcript arrives with long tool payloads cut server-side
                  so opening a session stays cheap. This pulls THIS turn's full
                  trace on demand. Needs a sessionId to address the message. */}
              {truncated && sessionId && (
                <button
                  onClick={loadFullSteps}
                  disabled={loadingFull}
                  title="Bu turun kırpılmış araç çıktılarının tamamını sunucudan getir"
                  className="flex items-center gap-1 text-[10px] text-[var(--color-text-dim)] underline decoration-dotted underline-offset-2 transition hover:text-[var(--color-accent)] disabled:opacity-50"
                >
                  {loadingFull ? '⏳ getiriliyor…' : '⤓ tam iz'}
                </button>
              )}
              {fullError && (
                <span className="text-[10px] text-[var(--color-danger)]">tam iz alınamadı</span>
              )}
            </div>
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
            <div
              className={
                steps.length > 0 || m.text.trim() || m.reasoningContent ? 'mt-1.5' : undefined
              }
            >
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
      {/* Footer BELOW the bubble: passive meta (time, duration, model, token spend)
          on the LEFT, action chips (read-aloud, rating, retry, delete) on the
          RIGHT. The chips are visible at rest, not hover-only — see
          messageActions.ts. */}
      <div className={TURN_FOOTER}>
        <div className={META_CLUSTER}>
          <MessageTime unixSec={m.createdAt} />
          {isLastLive ? (
            <LiveTimer startUnixSec={m.createdAt} />
          ) : (
            <TurnDuration ms={workedMs} derived={workedDerived} />
          )}
          {/* Per-turn metadata: model + token usage (the per-turn cost). */}
          {(m.model || m.usage) && (
            <span
              className="flex items-center gap-1.5 text-[10px] text-[var(--color-text-dim)]"
              title="Bu turun modeli ve token tüketimi"
            >
              {/* Named, not mono: this is the model people talk about, while the
                  numbers beside it stay mono. The exact id lives in the title. */}
              {m.model && (
                <span className="opacity-80" title={m.model}>
                  {modelDisplayName(m.model)}
                </span>
              )}
              {m.usage && (
                <span className="font-mono">
                  ↑{fmtTok(m.usage.in)} ↓{fmtTok(m.usage.out)}
                  {!!m.usage.cacheRead && (
                    <span className="opacity-60"> ⚡{fmtTok(m.usage.cacheRead)}</span>
                  )}
                </span>
              )}
            </span>
          )}
        </div>
        {hasActions && (
          <div className={ACTION_CLUSTER}>
            {/* Every file this turn changed, browsable in one popup (with a
                whole-session tab) instead of hunting the inline diff cards. */}
            {showChanges && (
              <ChangesButton
                sessionId={sessionId}
                msgId={m.id}
                steps={steps}
                onOpenFile={onOpenFile}
              />
            )}
            {/* Read this reply aloud (TTS). Toggles play/stop; strips code/tables.
                The adjacent slider sets the GLOBAL read-aloud volume (all bubbles). */}
            {canSpeak && (
              <div className="flex items-center gap-1.5">
                <button
                  onClick={toggleSpeak}
                  title={speaking ? 'Okumayı durdur' : 'Yanıtı sesli oku (kod atlanır)'}
                  aria-label={speaking ? 'Okumayı durdur' : 'Yanıtı sesli oku'}
                  className={speaking ? actionChipActive() : actionChip()}
                >
                  {speaking ? <Square size={13} /> : <Volume2 size={13} />}
                </button>
                <TtsVolumeSlider />
              </div>
            )}
            {/* Feedback thumbs (not on the live bubble). */}
            {onFeedback && !isLastLive && (
              <>
                <button
                  onClick={() => onFeedback(m.id, rating === 1 ? 0 : 1)}
                  title="Bu yanıt iyi"
                  aria-label="Bu yanıt iyi"
                  aria-pressed={rating === 1}
                  className={rating === 1 ? actionChipActive('positive') : actionChip('positive')}
                >
                  <ThumbsUp size={13} />
                </button>
                <button
                  onClick={() => onFeedback(m.id, rating === -1 ? 0 : -1)}
                  title="Bu yanıt kötü"
                  aria-label="Bu yanıt kötü"
                  aria-pressed={rating === -1}
                  className={rating === -1 ? actionChipActive('danger') : actionChip('danger')}
                >
                  <ThumbsDown size={13} />
                </button>
              </>
            )}
            {canRetry && (
              <button
                onClick={() => onRetry!(m.id)}
                title="Bu turu yeniden dene"
                className={actionChipActive('danger', 'font-semibold')}
              >
                <RotateCcw size={13} />
                Yeniden dene
              </button>
            )}
            {onDelete && !isLastLive && <DeleteButton onClick={() => onDelete(m.id)} />}
          </div>
        )}
      </div>
    </div>
  )
})
