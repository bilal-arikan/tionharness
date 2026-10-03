import type { Skill, SkillDetail, SkillInput, ToolVisibility } from '@/types'
import { req } from './client'

export const skillApi = {
  listSkills: (archived: boolean | 'all' = false) =>
    req<Skill[]>(`/api/skills?archived=${archived}`),
  getSkill: (slug: string) => req<SkillDetail>(`/api/skills/${encodeURIComponent(slug)}`),
  createSkill: (input: SkillInput & { slug?: string }) =>
    req<SkillDetail>('/api/skills', { method: 'POST', body: JSON.stringify(input) }),
  updateSkill: (slug: string, input: SkillInput) =>
    req<SkillDetail>(`/api/skills/${encodeURIComponent(slug)}`, {
      method: 'PUT',
      body: JSON.stringify(input),
    }),
  deleteSkill: (slug: string) =>
    req<{ ok: boolean }>(`/api/skills/${encodeURIComponent(slug)}`, { method: 'DELETE' }),
  reloadSkills: () => req<{ ok: boolean }>('/api/skills/reload', { method: 'POST' }),
  // Overwrite a shipped skill's SKILL.md with its default, discarding local
  // edits. Global tier only (defaultState set); 404 otherwise.
  restoreSkill: (slug: string) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/restore`, { method: 'POST' }),
  setSkillAccess: (slug: string, shared: boolean) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/access`, {
      method: 'PUT',
      body: JSON.stringify({ shared }),
    }),
  // Force a skill into one of the four visibility tiers (full | summary |
  // name-only | hidden) — the skill analogue of a tool's visibility. The single
  // entry point the tier selector drives; maps onto the autoSummary/nameOnly/
  // summaryOnly flags on the backend.
  setSkillVisibility: (slug: string, visibility: ToolVisibility) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/visibility`, {
      method: 'PUT',
      body: JSON.stringify({ visibility }),
    }),
  // Set a skill's `group` (its Skills-UI organisation bucket) without touching any
  // other field. Empty string ungroups it. Drives the bulk "set group" action.
  setSkillGroup: (slug: string, group: string) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/group`, {
      method: 'PUT',
      body: JSON.stringify({ group }),
    }),
}
