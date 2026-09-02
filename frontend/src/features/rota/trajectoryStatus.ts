// Trajectory status → badge tone / Turkish label, shared by the Rota screen,
// the trajectory list and the chat header strip (kept out of the component
// files for react-refresh).
import type { BadgeTone } from '@/shared/components'
import type { TrajectoryStatus } from '@/types/trajectory'

export const STATUS_TONE: Record<TrajectoryStatus, BadgeTone> = {
  planned: 'muted',
  running: 'accent',
  waiting: 'warning',
  done: 'success',
  failed: 'danger',
  abandoned: 'muted',
}

export const STATUS_LABEL: Record<TrajectoryStatus, string> = {
  planned: 'planlandı',
  running: 'sürüyor',
  waiting: 'bekliyor',
  done: 'bitti',
  failed: 'başarısız',
  abandoned: 'terk edildi',
}
