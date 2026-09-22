// Archive / unarchive for the entities that mirror the kanban card's archive:
// agents, artifacts, automations, goals and skills. Every one of them exposes the
// same pair on the backend (internal/api/archive_routes.go), so one call covers
// them all:
//
//   POST /api/{kind}/{id}/archive     → put away (hidden from the default
//                                       lists and the Map, reversible)
//   POST /api/{kind}/{id}/unarchive   → restore
import { req } from './client'

export type ArchivableKind = 'agents' | 'artifacts' | 'automations' | 'goals' | 'skills'

export const archiveApi = {
  setArchived: (kind: ArchivableKind, id: string, archived: boolean) =>
    req<{ id: string; archived: boolean }>(
      `/api/${kind}/${encodeURIComponent(id)}/${archived ? 'archive' : 'unarchive'}`,
      { method: 'POST' },
    ),
}
