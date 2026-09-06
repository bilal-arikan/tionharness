// Client-side finding helpers for the Insight cockpit: priority scoring,
// filtering and summary counts. Priority mirrors the Go implementation
// (internal/insight/priority.go) so the panel can filter instantly without a
// server round-trip.
import type { InsightFinding } from '@/types'

// priorityScore mirrors internal/insight/priority.go.
export function priorityScore(f: InsightFinding): number {
  const sev = f.severity === 'high' ? 30 : f.severity === 'med' || f.severity === 'medium' ? 20 : 10
  const occ = Math.min(f.occurrences ?? 0, 20)
  let score = sev + occ
  if (f.regressed) score += 100
  if (f.status === 'dismissed' || f.status === 'verified') score -= 1000
  else if (f.status === 'applied') score -= 50
  return score
}

export interface FindingFilter {
  channel?: string
  status?: string
  severity?: string
  lens?: string
  regressedOnly?: boolean
  search?: string
}

export function applyFilter(findings: InsightFinding[], f: FindingFilter): InsightFinding[] {
  const q = (f.search ?? '').trim().toLowerCase()
  return findings.filter((x) => {
    if (f.channel && x.channel !== f.channel) return false
    if (f.status && (x.status || 'new') !== f.status) return false
    if (f.severity && (x.severity || '') !== f.severity) return false
    if (f.lens && x.lensId !== f.lens) return false
    if (f.regressedOnly && !x.regressed) return false
    if (q) {
      const hay = `${x.title} ${x.rootCause ?? ''} ${x.proposedFix ?? ''} ${x.sig}`.toLowerCase()
      if (!hay.includes(q)) return false
    }
    return true
  })
}

export interface FindingSummary {
  total: number
  appFix: number
  workspaceOpt: number
  recipeOpt: number
  evolution: number
  high: number
  regressed: number
  open: number
}

export function summarize(findings: InsightFinding[]): FindingSummary {
  const s: FindingSummary = {
    total: findings.length,
    appFix: 0,
    workspaceOpt: 0,
    recipeOpt: 0,
    evolution: 0,
    high: 0,
    regressed: 0,
    open: 0,
  }
  for (const f of findings) {
    if (f.channel === 'app-fix') s.appFix++
    else if (f.channel === 'workspace-opt') s.workspaceOpt++
    else if (f.channel === 'recipe-opt') s.recipeOpt++
    else if (f.channel === 'evolution') s.evolution++
    if (f.severity === 'high') s.high++
    if (f.regressed) s.regressed++
    const st = f.status || 'new'
    if (st !== 'dismissed' && st !== 'verified') s.open++
  }
  return s
}
