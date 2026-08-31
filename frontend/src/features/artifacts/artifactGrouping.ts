// Grouping/paging primitives for the artifacts screen, split out of
// ArtifactsPanel so the panel file stays about wiring. Pure data + stateless
// helpers only.
import type { Artifact, ArtifactKind } from '@/types'
import { compareText } from '@/shared/lib/intl'

// Label for the bucket holding artifacts with no `group` set; always rendered last.
export const UNGROUPED = 'Grupsuz'

// Page size for the server-side "load more" paging (the API caps a page at 100;
// 50 keeps each fetch snappy while staying well under the cap).
export const ARTIFACTS_PAGE_SIZE = 50

// Group key for one artifact: its `group` field, or the ungrouped bucket.
export function artifactGroupKey(a: Artifact): string {
  return a.group?.trim() || UNGROUPED
}

// Identity of one artifact row (module-scope so the drag hook's lookup map is stable).
export function artifactId(a: Artifact): string {
  return a.id
}

// Order groups: ungrouped bucket first, then named groups alphabetically (tr).
export function sortArtifactGroups(a: string, b: string): number {
  if (a === UNGROUPED) return -1
  if (b === UNGROUPED) return 1
  return compareText(a, b)
}

// Origin facet values offered by the list filter ('all' = no facet).
export type OriginFilter = 'all' | 'chat' | 'manual' | 'agent' | 'tool' | 'plan'

// Draft holds the editable fields while creating or editing an artifact.
export interface Draft {
  title: string
  kind: ArtifactKind
  language: string
  content: string
  // Organisation bucket the artifact belongs to. Empty string = ungrouped.
  // Persisted through the dedicated group endpoint (not the content/meta patch).
  group: string
}

// Seed a draft from the persisted artifact it edits.
export function draftFromArtifact(a: Artifact): Draft {
  return {
    title: a.title,
    kind: a.kind,
    language: a.language,
    content: a.content,
    group: a.group ?? '',
  }
}
