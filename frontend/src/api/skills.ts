import type { Skill, SkillDetail, SkillInput } from '../types'
import { req } from './client'

export const skillApi = {
  listSkills: () => req<Skill[]>('/api/skills'),
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
  setSkillAccess: (slug: string, shared: boolean) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/access`, {
      method: 'PUT',
      body: JSON.stringify({ shared }),
    }),
  revealSkill: (slug: string) =>
    req<{ path: string }>(`/api/skills/${encodeURIComponent(slug)}/reveal`, { method: 'POST' }),
}
