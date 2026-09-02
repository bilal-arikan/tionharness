// Curator (Rota F3) — the LLM-free weekly pass: last report + run now.
import { req } from './client'
import type { CuratorReport } from '@/types/curator'

export const curatorApi = {
  // 404 when the curator has not run yet in this workspace.
  curatorReport: (): Promise<CuratorReport> => req<CuratorReport>('/api/curator/report'),
  runCurator: (apply = true): Promise<CuratorReport> =>
    req<CuratorReport>(`/api/curator/run?apply=${apply ? 'true' : 'false'}`, { method: 'POST' }),
}
