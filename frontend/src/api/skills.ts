import type { Skill, SkillDetail, SkillInput } from '../types'
import { req } from './client'

// SkillImportResult mirrors the backend skills.ImportResult (SK-IMP): the new
// slug, which CC frontmatter fields were carried over, the bundled files copied,
// and warnings about unsupported CC features that were dropped.
export interface SkillImportResult {
  slug: string
  name: string
  mappedFields: string[]
  files: string[]
  warnings: string[]
}

export interface SkillImportResponse {
  result: SkillImportResult
  skill: SkillDetail
}

export interface SkillImportInput {
  source: 'local' | 'github'
  path?: string // local directory (source=local)
  url?: string // github.com folder URL (source=github)
  slug?: string
  shared?: boolean
}

export const skillApi = {
  listSkills: () => req<Skill[]>('/api/skills'),
  importSkill: (input: SkillImportInput) =>
    req<SkillImportResponse>('/api/skills/import', { method: 'POST', body: JSON.stringify(input) }),
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
  setSkillAutoSummary: (slug: string, autoSummary: boolean) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/auto-summary`, {
      method: 'PUT',
      body: JSON.stringify({ autoSummary }),
    }),
  setSkillNameOnly: (slug: string, nameOnly: boolean) =>
    req<Skill>(`/api/skills/${encodeURIComponent(slug)}/name-only`, {
      method: 'PUT',
      body: JSON.stringify({ nameOnly }),
    }),
  revealSkill: (slug: string) =>
    req<{ path: string }>(`/api/skills/${encodeURIComponent(slug)}/reveal`, { method: 'POST' }),
}
