import type { WorkspacePromptMeta } from '@/types'

type PromptMeta = Record<string, WorkspacePromptMeta | undefined>

export function isSystemOwnedPrompt(key: string, promptMeta: PromptMeta): boolean {
  return !!promptMeta[key]?.ownedBySystemKey
}

export function changedEditablePrompts(
  promptKeys: string[],
  draft: Record<string, string>,
  original: Record<string, string>,
  promptMeta: PromptMeta,
): Record<string, string> {
  const changed: Record<string, string> = {}
  for (const key of promptKeys) {
    // System-owned prompts are display-only. Their effective text is edited on
    // the Agents screen and must never create a stale workspace override here.
    if (isSystemOwnedPrompt(key, promptMeta)) continue
    if (draft[key] !== original[key]) changed[key] = draft[key]
  }
  return changed
}
