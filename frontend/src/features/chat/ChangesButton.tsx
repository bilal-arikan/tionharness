import { useMemo, useState } from 'react'
import { FileDiff } from 'lucide-react'
import type { TurnStep } from '@/types'
import { extractFileChanges, changeTotals } from '@/shared/lib/fileChanges'
import { ChangesModal } from './ChangesModal'
import { actionChip } from './messageActions'
import { useTranslation } from 'react-i18next'

const MINUS_SIGN = String.fromCharCode(0x2212)
const AT_LEAST = String.fromCharCode(0x2265)

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
  const { t } = useTranslation('chat')
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
        title={t('changesButton.title', { count: files })}
        className={actionChip()}
      >
        <FileDiff size={13} />
        <span>{t('changesButton.files', { count: files })}</span>
        {added > 0 && <span className="text-[var(--color-success)]">+{added}</span>}
        {removed > 0 && (
          <span className="text-[var(--color-danger)]">
            {MINUS_SIGN}
            {removed}
          </span>
        )}
        {partial && <span className="opacity-60">{AT_LEAST}</span>}
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
