import { useState } from 'react'
import { MessageCircleQuestion } from 'lucide-react'
import { Button, ScrollableCard } from '@/shared/components'
import { ComposerCard } from './ComposerCard'

export interface PendingAsk {
  question: string
  options?: string[]
  // Multi-question ask: each entry is its own question with optional suggested
  // answers. When present, AskPrompt renders one field per question and submits
  // all answers together (as a JSON array).
  questions?: { question: string; options?: string[] }[]
  // 'permission' renders the approval card (PermissionPrompt) and 'plan' the
  // plan-approval card (PlanPrompt) instead of the plain question card; tool/risk
  // describe the gated tool.
  kind?: 'ask' | 'permission' | 'plan'
  tool?: string
  risk?: string
  // cmd is the representative argument of the gated call (e.g. the shell command)
  // shown on the permission card, or the plan markdown on the plan card, so the
  // user sees what is being approved.
  cmd?: string
  // interactionId is the server-side resolve-once id (Faz 2): the answer is
  // POSTed to /sessions/{id}/interactions/{interactionId}/answer, so the first
  // window to reply wins (CAS) and every other window's card closes on the
  // broadcast interaction_resolved. Absent only on legacy/local-only prompts.
  interactionId?: string
}

interface Props {
  ask: PendingAsk
  onAnswer: (text: string) => void
}

const neutralOptionClass =
  'border-[var(--color-border)] bg-[var(--color-bg)] text-[var(--color-text)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]'

function askOptionClass(option: string) {
  switch (option.trim()) {
    case 'Onayla':
      return 'border-[color-mix(in_srgb,var(--color-success)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-success)_12%,transparent)] text-[var(--color-success)] hover:border-[var(--color-success)] hover:bg-[color-mix(in_srgb,var(--color-success)_18%,transparent)]'
    case 'İptal':
      return 'border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] text-[var(--color-danger)] hover:border-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_18%,transparent)]'
    default:
      return neutralOptionClass
  }
}

// AskPrompt renders the agent's clarifying question while the turn is paused on
// the ask_user tool: the question, optional one-click suggested answers, and a
// free-text field. Submitting any of them delivers the answer and unblocks the
// agent (the turn resumes over the still-open SSE stream). When the agent asked
// SEVERAL questions at once (ask.questions), a combined multi-field form is shown
// instead and every answer is submitted together.
export function AskPrompt({ ask, onAnswer }: Props) {
  if (ask.questions?.length) return <MultiAskPrompt questions={ask.questions} onAnswer={onAnswer} />
  return <SingleAskPrompt ask={ask} onAnswer={onAnswer} />
}

// SingleAskPrompt is the original one-question card.
function SingleAskPrompt({ ask, onAnswer }: Props) {
  const [text, setText] = useState('')

  const submit = (value: string) => {
    const v = value.trim()
    if (!v) return
    onAnswer(v)
  }

  return (
    <ComposerCard tone="ask" className="px-3 py-2.5">
      <ScrollableCard>
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
                className={`rounded-full border px-3 py-1 text-xs ${askOptionClass(opt)}`}
              >
                {opt}
              </button>
            ))}
          </div>
        )}
      </ScrollableCard>

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
    </ComposerCard>
  )
}

// MultiAskPrompt renders several questions at once, each with its own optional
// suggested answers and a free-text field. "Gönder" is enabled only when every
// question has an answer; all answers are delivered together as a JSON array (in
// question order), which the backend folds into one labeled block for the model.
function MultiAskPrompt({
  questions,
  onAnswer,
}: {
  questions: { question: string; options?: string[] }[]
  onAnswer: (text: string) => void
}) {
  const [answers, setAnswers] = useState<string[]>(() => questions.map(() => ''))
  const setAt = (i: number, v: string) =>
    setAnswers((prev) => prev.map((a, j) => (j === i ? v : a)))
  const allAnswered = answers.every((a) => a.trim().length > 0)

  const submit = () => {
    if (!allAnswered) return
    onAnswer(JSON.stringify(answers.map((a) => a.trim())))
  }

  return (
    <ComposerCard tone="ask" className="px-3 py-2.5">
      <div className="mb-2 flex items-center gap-2 text-sm font-medium text-[var(--color-text)]">
        <MessageCircleQuestion size={16} className="shrink-0 text-[var(--color-accent)]" />
        <span>Birkaç soru ({questions.length})</span>
      </div>

      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="flex flex-col gap-3"
      >
        <ScrollableCard className="flex flex-col gap-3 pr-1">
          {questions.map((q, i) => (
            <div
              key={i}
              className="flex flex-col gap-1.5 border-t border-[var(--color-border)] pt-2 first:border-t-0 first:pt-0"
            >
              <div className="flex items-start gap-2 text-sm text-[var(--color-text)]">
                <span className="mt-0.5 shrink-0 tabular-nums text-[var(--color-accent)]">
                  {i + 1}.
                </span>
                <span className="min-w-0 flex-1">{q.question}</span>
              </div>
              {!!q.options?.length && (
                <div className="flex flex-wrap gap-1.5 pl-6">
                  {q.options.map((opt, oi) => (
                    <button
                      key={oi}
                      type="button"
                      onClick={() => setAt(i, opt)}
                      className={`rounded-full border px-3 py-1 text-xs ${askOptionClass(opt)} ${
                        answers[i] === opt ? 'ring-1 ring-[var(--color-accent)]' : ''
                      }`}
                    >
                      {opt}
                    </button>
                  ))}
                </div>
              )}
              <input
                autoFocus={i === 0}
                value={answers[i]}
                onChange={(e) => setAt(i, e.target.value)}
                placeholder="Yanıtını yaz…"
                className="ml-6 min-w-0 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-accent)]"
              />
            </div>
          ))}
        </ScrollableCard>

        <div className="flex justify-end">
          <Button type="submit" disabled={!allAnswered}>
            Gönder
          </Button>
        </div>
      </form>
    </ComposerCard>
  )
}
