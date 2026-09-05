import type { View } from '@/app/NavRail'
import { VIEW_TITLE } from '@/app/viewRegistry'
import type { ViewRef } from '@/types'

// Where a map node "lives": the screen that owns the entity and, when that
// screen can pre-select it, the id to open it on. The side panel's "open in
// screen" button is built from this; null means the node has no home screen
// beyond the map itself.
export interface ExplorerTarget {
  view: View
  id: string | null
  // Human name of the screen, for the button label.
  label: string
}

const CATEGORY_VIEW: Record<string, View> = {
  sessions: 'chat',
  flows: 'flows',
  agents: 'agents',
  artifacts: 'artifacts',
  automations: 'schedules',
  skills: 'skills',
  insights: 'insights',
}

function target(view: View, id: string | null = null): ExplorerTarget {
  return { view, id, label: VIEW_TITLE[view] }
}

export function screenForRef(ref: ViewRef): ExplorerTarget | null {
  switch (ref.kind) {
    case 'workspace':
      return target('workspace')
    case 'category':
      if (ref.id.startsWith('col:')) return target('board')
      if (ref.id.startsWith('skind:')) return target('chat')
      return CATEGORY_VIEW[ref.id] ? target(CATEGORY_VIEW[ref.id]) : null
    case 'board':
      // A card (sub = task id) opens its editor on the board.
      return target('board', ref.sub ?? null)
    case 'session':
      return target('chat', ref.id)
    case 'flowrun':
      // The Flows screen has no run deep link; open it on its own tab.
      return target('flows')
    case 'schedule':
    case 'automation':
      return target('schedules', ref.id)
    case 'agent':
      return target('agents', ref.id)
    case 'artifact':
      return target('artifacts', ref.id)
    case 'skill':
      return target('skills')
    case 'insight':
      return target('insights')
    case 'budget':
      return target('budget')
    case 'tools':
      // A built-in group pre-selects that group on the Tools screen.
      return target('tools', ref.sub?.startsWith('group:') ? ref.sub.slice('group:'.length) : null)
    case 'logs':
      // Process logs live under Workspace ▸ Logs.
      return target('workspace', 'logs')
    case 'trajectory':
      return target('rota', ref.id)
    default:
      return null
  }
}
