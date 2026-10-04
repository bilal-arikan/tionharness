// Shared badges for the Notes screen: kind, confidence and scope/reach.
import { useTranslation } from 'react-i18next'
import { Badge } from '@/shared/components'
import type { Note, NoteConfidence, NoteKind } from '@/types'
import { confidenceTone, kindTone, reachOf } from './notesHelpers'

export function KindBadge({ kind }: { kind: NoteKind }) {
  const { t } = useTranslation('notes')
  return <Badge tone={kindTone(kind)}>{t(`kind.${kind}`)}</Badge>
}

export function ConfidenceBadge({ confidence }: { confidence: NoteConfidence }) {
  const { t } = useTranslation('notes')
  return <Badge tone={confidenceTone(confidence)}>{t(`confidence.${confidence}`)}</Badge>
}

// ScopeBadge spells the reach: the scope word plus how many agents / project
// roots a scoped note names ("agent · 2").
export function ScopeBadge({ note }: { note: Note }) {
  const { t } = useTranslation('notes')
  const reach = reachOf(note)
  return (
    <Badge tone="muted">
      {t(`scope.${note.scope}`)}
      {reach.length > 0 ? ` · ${reach.length}` : ''}
    </Badge>
  )
}
