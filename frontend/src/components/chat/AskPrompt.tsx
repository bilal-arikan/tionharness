import { useState } from 'react'
import { MessageCircleQuestion } from 'lucide-react'
import { Button } from '../common'

export interface PendingAsk {
  question: string
  options?: string[]
  // 'permission' renders the approval card (PermissionPrompt) instead of the
  // plain question card; tool/risk describe the gated tool.
  kind?: 'ask' | 'permission'
  tool?: string
  risk?: string
  // cmd is the representative argument of the gated call (e.g. the shell command)
  // shown on the permission card so the user sees what is being approved.
  cmd?: string
}

interface Props {
  ask: PendingAsk
  onAnswer: (text: string) => void
}

// AskPrompt renders the agent's clarifying question while the turn is paused on
// the ask_user tool: the question, optional one-click suggested answers, and a
// free-text field. Submitting any of them delivers the answer and unblocks the
// agent (the turn resumes over the still-open SSE stream).
export function AskPrompt({ ask, onAnswer }: Props) {
  const [text, setText] = useState('')

  const submit = (value: string) => {
    const v = value.trim()
    if (!v) return
    onAnswer(v)
  }

  return (
    <div className="mx-3 mb-2 rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] px-3 py-2.5">
      <div className="mb-2 flex items-start gap-2 text-sm text-[var(--color-text)]">
        <MessageCircleQuestion size={16} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
        <span className="min-w-0 flex-1 font-medium">{ask.question}</span>
      </div>

      {!!ask.options?.length && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {ask.options.map((opt, i) => (
            <button
              key={i}
              onClick={() => submit(opt)}
              className="rounded-full border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-1 text-xs text-[var(--color-text)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
            >
              {opt}
            </button>
          ))}
        </div>
      )}

      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit(text)
        }}
        className="flex gap-2"
      >
        <input
          autoFocus
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Yanıtını yaz…"
          className="min-w-0 flex-1 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]"
        />
        <Button type="submit" disabled={!text.trim()}>
          Gönder
        </Button>
      </form>
    </div>
  )
}
