import { ClipboardList } from 'lucide-react'
import type { PendingAsk } from './AskPrompt'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { ScrollableCard } from '@/shared/components'
import { ComposerCard } from './ComposerCard'

interface Props {
  ask: PendingAsk
  onAnswer: (text: string) => void
}

// PlanPrompt renders the plan-approval card shown while a claude-cli turn is
// paused on ExitPlanMode: the proposed plan (rendered as markdown) plus Approve /
// Reject buttons. Approving lets the CLI leave plan mode and proceed; rejecting
// returns control to the model so it can revise. Each delivers the answer over the
// same channel as ask_user, unblocking the CLI permission-prompt tool.
export function PlanPrompt({ ask, onAnswer }: Props) {
  const options = ask.options?.length ? ask.options : ['Planı onayla', 'Reddet']
  return (
    <ComposerCard tone="plan" className="px-3 py-2.5">
      <div className="mb-2 flex items-start gap-2 text-sm text-[var(--color-text)]">
        <ClipboardList size={16} className="mt-0.5 shrink-0 text-[var(--color-success)]" />
        <span className="min-w-0 flex-1 font-medium">Ajan bir plan sunuyor. Onaylıyor musun?</span>
      </div>
      {ask.cmd && (
        <ScrollableCard
          maxH="max-h-[55vh]"
          className="mb-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm"
        >
          <Markdown>{ask.cmd}</Markdown>
        </ScrollableCard>
      )}
      <div className="flex flex-wrap gap-1.5">
        {options.map((opt, i) => {
          const reject = /reddet|reject|deny/i.test(opt)
          return (
            <button
              key={i}
              onClick={() => onAnswer(opt)}
              className={
                reject
                  ? 'rounded-full border border-[var(--color-danger)]/60 px-3 py-1 text-xs text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10'
                  : 'rounded-full bg-[var(--color-success)] px-3 py-1 text-xs font-medium text-[var(--color-on-success)] hover:opacity-90'
              }
            >
              {opt}
            </button>
          )
        })}
      </div>
    </ComposerCard>
  )
}
