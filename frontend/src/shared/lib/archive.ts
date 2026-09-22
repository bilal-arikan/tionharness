// Pure helpers for lists that carry an `archived` flag (tasks, agents, skills,
// automations, artifacts). The archive view shows exactly the archived items;
// the live view exactly the rest — never a mix.

export interface Archivable {
  archived?: boolean
}

/** Items on the requested side of the archive. */
export function archiveSide<T extends Archivable>(items: T[], showArchived: boolean): T[] {
  return items.filter((it) => !!it.archived === showArchived)
}

/** Number of archived items, for the toggle / banner counts. */
export function archivedCount<T extends Archivable>(items: T[]): number {
  return items.reduce((n, it) => n + (it.archived ? 1 : 0), 0)
}
