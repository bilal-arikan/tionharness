import { Scale } from 'lucide-react'

interface Props {
  question: string
  onQuestion: (question: string) => void
  hint: string
}

// JudgeFields is the extra input of a branch/loop node in the "judge" match mode:
// an optional question telling the decision model what to look at, plus a note
// on how the mode behaves when the model is unsure or unavailable.
export function JudgeFields({ question, onQuestion, hint }: Props) {
  return (
    <div className="space-y-1 rounded border border-[var(--color-border)] p-2">
      <label className="block">
        <span className="mb-1 flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
          <Scale size={12} /> Karar sorusu (isteğe bağlı)
        </span>
        <input
          value={question}
          onChange={(e) => onQuestion(e.target.value)}
          placeholder="ör. Bu ne tür bir istek?"
          className="w-full rounded bg-[var(--color-surface-2)] px-2 py-1 text-xs outline-none"
        />
      </label>
      <p className="text-[11px] text-[var(--color-text-dim)]">{hint}</p>
    </div>
  )
}
