import { useEffect, useRef, useState } from 'react'
import { MessageCircleQuestion, Send, X } from 'lucide-react'
import { api } from '@/api'
import type { BtwResponse } from '@/types'
import { ActionButton } from './ActionButton'
import { BTN_PRIMARY } from './buttonStyles'

interface Props {
  sessionId: string
  // The agent that answers (the composer's selected agent).
  agentId: string
  onClose: () => void
}

// BtwPanel is the "/btw" side chat: a quick, read-only consultation asked while
// the agent is busy with the main task.
//
// What makes it a SIDE chat (and not just another message):
//   - The exchange is NOT added to the conversation history, so it does not grow
//     the token cost of a long session.
//   - The agent still sees the full current context (the code it read, the
//     decisions it made) — the question is answered against it server-side.
//   - The agent gets NO tools here: it cannot run commands or edit files.
//
// The answer therefore lives only in this panel: it is shown until the panel is
// closed and is never persisted. That is intentional, not a missing feature.
export function BtwPanel({ sessionId, agentId, onClose }: Props) {
  const [question, setQuestion] = useState('')
  const [answer, setAnswer] = useState<BtwResponse | null>(null)
  const [asking, setAsking] = useState(false)
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const ask = () => {
    const q = question.trim()
    if (!q || asking) return
    setAsking(true)
    setError('')
    setAnswer(null)
    api
      .btw(sessionId, q, agentId)
      .then(setAnswer)
      .catch((e: unknown) => setError((e as Error).message))
      .finally(() => setAsking(false))
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      ask()
    }
  }

  return (
    <div
      data-testid="btw-panel"
      className="absolute bottom-full right-3 z-20 mb-2 w-[min(30rem,calc(100vw-2rem))] rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-3 shadow-2xl md:right-6"
    >
      <div className="mb-2 flex items-center gap-2">
        <MessageCircleQuestion size={16} className="text-[var(--color-accent)]" />
        <span className="text-sm font-medium">Btw — yan soru</span>
        <span className="text-xs text-[var(--color-text-dim)]">geçmişe yazılmaz</span>
        <div className="flex-1" />
        <button
          type="button"
          onClick={onClose}
          title="Kapat"
          aria-label="Kapat"
          data-testid="btw-close"
          className="text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
        >
          <X size={16} />
        </button>
      </div>

      <p className="mb-2 text-xs leading-4 text-[var(--color-text-dim)]">
        Ajan bu soruyu mevcut konuşmanın tam bağlamıyla yanıtlar, ama soru ve cevap sohbet geçmişine{' '}
        <strong>eklenmez</strong> ve ana görev kesilmez. Yan sohbette araç kullanımı yoktur (komut
        çalıştıramaz, dosya düzenleyemez).
      </p>

      <textarea
        ref={inputRef}
        value={question}
        onChange={(e) => setQuestion(e.target.value)}
        onKeyDown={onKeyDown}
        rows={2}
        placeholder="Yan soru yaz — Enter ile sor"
        data-testid="btw-input"
        aria-label="Yan soru"
        className="max-h-[8rem] w-full resize-none overflow-y-auto rounded-xl border border-[var(--color-border)] bg-transparent px-2.5 py-2 text-sm leading-5 outline-none transition-colors focus:border-[var(--color-accent)] placeholder:text-[var(--color-text-dim)]"
      />

      <div className="mt-2 flex items-center gap-2">
        <div className="flex-1" />
        <ActionButton
          onClick={ask}
          disabled={!question.trim() || asking}
          testId="btw-ask"
          icon={Send}
          label={asking ? 'Soruluyor…' : 'Sor'}
          className={BTN_PRIMARY}
        />
      </div>

      {error && (
        <div
          data-testid="btw-error"
          className="mt-2 rounded-xl border border-[var(--color-danger)] px-2.5 py-2 text-xs text-[var(--color-danger)]"
        >
          {error}
        </div>
      )}

      {answer && (
        <div
          data-testid="btw-answer"
          className="mt-2 max-h-[18rem] overflow-y-auto whitespace-pre-wrap rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-2 text-sm leading-5"
        >
          {answer.answer}
        </div>
      )}
    </div>
  )
}
