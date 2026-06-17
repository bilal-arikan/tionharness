import type { Skill, SkillDetail } from '../types'
import { req } from './client'

export const skillApi = {
  listSkills: () => req<Skill[]>('/api/skills'),
  getSkill: (slug: string) => req<SkillDetail>(`/api/skills/${encodeURIComponent(slug)}`),
  reloadSkills: () => req<{ ok: boolean }>('/api/skills/reload', { method: 'POST' }),
  setSkillAccess: (slug: string, shared: boolean) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/access`, {
      method: 'PUT',
      body: JSON.stringify({ shared }),
    }),
  revealSkill: (slug: string) =>
    req<{ path: string }>(`/api/skills/${encodeURIComponent(slug)}/reveal`, { method: 'POST' }),
}
