import type { Agent } from '@/types'
import { i18next } from '@/i18n'

// Label appended to an archived agent that is still shown in a picker because
// it is the record's currently-saved value.
export function archivedAgentLabel(): string {
  return i18next.t('agents.archived', { ns: 'sharedUi' })
}

// pickableAgents returns the agents a picker may offer as a NEW selection: an
// archived agent cannot run and the backend refuses it as a schedule /
// automation / task-owner target, so it is hidden. The one exception is the
// currently-saved value (currentId): editing an existing record must still show
// what it points at instead of silently rendering an empty picker.
export function pickableAgents<T extends Pick<Agent, 'id' | 'archived'>>(
  agents: T[],
  currentId?: string,
): T[] {
  return agents.filter((a) => !a.archived || (!!currentId && a.id === currentId))
}
