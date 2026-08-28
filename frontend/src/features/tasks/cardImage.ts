// Board-card image preview selection. Pure — no React, no network — so the
// "which image does this card show" rule is testable on its own.
import type { Artifact } from '@/types'

// pickCardImage returns the image artifact a board card previews, or null when
// the card has none.
//
// "Most recently added" is read off the artifactIds ORDER, not artifact
// createdAt: attaching a file appends its new id to the end of the card's list
// (TaskBoard.attachFilesToTask) and linking an existing artifact from the editor
// appends too, so the tail of the list is always what the user attached LAST to
// THIS card. createdAt would instead resurface an image uploaded long ago that
// was only just linked, which is not what the card should show.
//
// Ids that do not resolve are skipped: the artifact was deleted elsewhere, the
// same stale-ref tolerance the card's artifact chips already have.
export function pickCardImage(
  artifactIds: string[] | undefined,
  byId: ReadonlyMap<string, Artifact>,
): Artifact | null {
  if (!artifactIds) return null
  for (let i = artifactIds.length - 1; i >= 0; i--) {
    const a = byId.get(artifactIds[i])
    if (a && a.kind === 'image') return a
  }
  return null
}
