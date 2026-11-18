import type { Skill, ToolVisibility } from '@/types'

// skillVisibility resolves a skill's 4-way tier, preferring the backend-computed
// `visibility` and falling back to the raw frontmatter flags for older payloads.
export function skillVisibility(sk: Skill): ToolVisibility {
  if (sk.visibility) return sk.visibility
  if (sk.autoSummary === false) return 'hidden'
  if (sk.nameOnly) return 'name-only'
  if (sk.summaryOnly) return 'summary'
  return 'full'
}
