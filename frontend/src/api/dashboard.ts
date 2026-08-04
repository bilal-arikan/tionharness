// Workspace dashboard: counters + chart series + the workspace projection, in
// one call (see internal/api/dashboard.go).
import type { Dashboard } from '@/types'
import { req } from './client'

export const dashboardApi = {
  // getDashboard fetches the overview for a trailing window of `days`
  // (backend clamps to 1..90).
  getDashboard(days = 14): Promise<Dashboard> {
    return req<Dashboard>(`/api/dashboard?days=${days}`)
  },
}
