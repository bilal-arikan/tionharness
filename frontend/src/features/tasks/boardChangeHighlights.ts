import type { AppEvent } from '@/types'

const STORAGE_KEY = 'tionharness:board-pending-task-highlights'
const HIGHLIGHT_OPS = new Set(['create', 'update', 'move'])

type PendingByWorkspace = Record<string, string[]>

function readPending(): PendingByWorkspace {
  try {
    const parsed = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return Object.fromEntries(
      Object.entries(parsed).flatMap(([workspaceId, ids]) =>
        Array.isArray(ids) && ids.every((id) => typeof id === 'string') ? [[workspaceId, ids]] : [],
      ),
    )
  } catch {
    return {}
  }
}

function writePending(pending: PendingByWorkspace): void {
  if (Object.keys(pending).length === 0) {
    localStorage.removeItem(STORAGE_KEY)
    return
  }
  localStorage.setItem(STORAGE_KEY, JSON.stringify(pending))
}

export function recordPendingBoardChange(
  event: AppEvent,
  openBoardWorkspaceId: string | null,
): void {
  if (
    event.type !== 'board' ||
    !event.workspaceId ||
    event.workspaceId === openBoardWorkspaceId ||
    !event.target?.taskId ||
    !HIGHLIGHT_OPS.has(event.target.op ?? '')
  ) {
    return
  }

  const pending = readPending()
  pending[event.workspaceId] = [
    ...new Set([...(pending[event.workspaceId] ?? []), event.target.taskId]),
  ]
  writePending(pending)
}

export function consumePendingBoardChanges(workspaceId: string): Set<string> {
  const pending = readPending()
  const ids = new Set(pending[workspaceId] ?? [])
  if (!(workspaceId in pending)) return ids

  delete pending[workspaceId]
  writePending(pending)
  return ids
}
