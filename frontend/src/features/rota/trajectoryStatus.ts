// Trajectory status → badge tone / Turkish label, shared by the Rota screen,
// the trajectory list and the chat header strip (kept out of the component
// files for react-refresh).
import type { BadgeTone } from '@/shared/components'
import type { TrajectoryStatus } from '@/types/trajectory'
import { i18next } from '@/i18n'

export const STATUS_TONE: Record<TrajectoryStatus, BadgeTone> = {
  planned: 'muted',
  running: 'accent',
  waiting: 'warning',
  done: 'success',
  failed: 'danger',
  abandoned: 'muted',
}

export function trajectoryStatusLabel(status: TrajectoryStatus): string {
  return i18next.t(`status.${status}`, { ns: 'rota', defaultValue: status })
}
