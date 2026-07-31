import { useMemo, useState } from 'react'
import { FileDiff } from 'lucide-react'
import type { TurnStep } from '@/types'
import { extractFileChanges, changeTotals } from '@/shared/lib/fileChanges'
import { ChangesModal } from './ChangesModal'
import { actionChip } from './messageActions'

interface Props {
  sessionId?: string
  msgId: string
  steps: TurnStep[]
  onOpenFile?: (path: string) => void
}

// ChangesButton is the footer chip under an assistant bubble that opens the bulk
// file-changes browser. It renders nothing when the turn changed no files, so it
// never adds noise to a conversational reply.
//
// The modal is mounted only while open: its work (parsing every patch in the
// session) must not be paid by a transcript that merely scrolled past.
export function ChangesButton({ sessionId, msgId, steps, onOpenFile }: Props) {
  const [open, setOpen] = useState(false)
  const changes = useMemo(() => extractFileChanges(steps, { msgId }), [steps, msgId])
  if (changes.length === 0) return null

  const { files, added, removed } = changeTotals(changes)
  // A server-trimmed step means the counts below are a floor, not a total; the
  // modal refetches the untrimmed trace on open and corrects them there.
  const partial = changes.some((c) => c.truncated)

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title={`Bu turda değişen ${files} dosyanın farklarını topluca gör`}
        className={actionChip()}
      >
        <FileDiff size={13} />
        <span>{files} dosya</span>
        {added > 0 && <span className="text-[var(--color-success)]">+{added}</span>}
        {removed > 0 && <span className="text-[var(--color-danger)]">−{removed}</span>}
        {partial && <span className="opacity-60">≥</span>}
      </button>
      {open && (
        <ChangesModal
          sessionId={sessionId}
          msgId={msgId}
          turnSteps={steps}
          turnTruncated={partial}
          onClose={() => setOpen(false)}
          onOpenFile={onOpenFile}
        />
      )}
    </>
  )
}
