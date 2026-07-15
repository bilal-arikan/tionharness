// Client-side finding helpers for the Insight cockpit: priority scoring,
// filtering, lexical clustering and summary counts. Priority + clustering mirror
// the Go implementations (internal/insight/priority.go, cluster.go) so the panel
// can filter/collapse instantly without a server round-trip.
import type { InsightFinding } from '@/types'

export function severityRank(s?: string): number {
  switch (s) {
    case 'high':
      return 3
    case 'med':
    case 'medium':
      return 2
    case 'low':
      return 1
    default:
      return 0
  }
}

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

export interface FindingCluster {
  representative: InsightFinding
  members: InsightFinding[]
}

const STOPWORDS = new Set([
  'the', 'and', 'but', 'for', 'with', 'that', 'this', 'into', 'from', 'not', 'was', 'were',
  'are', 'its', 'has', 'had', 'when', 'which', 'instead', 'than', 'then', 'even', 'though',
  'tool', 'tools', 'agent', 'model', 'call', 'calls', 'called', 'using', 'use', 'used',
])

function tokenize(s: string): Set<string> {
  const out = new Set<string>()
  for (const w of s.toLowerCase().split(/[^a-z0-9]+/)) {
    if (w.length >= 3 && !STOPWORDS.has(w)) out.add(w)
  }
  return out
}

function jaccard(a: Set<string>, b: Set<string>): number {
  if (a.size === 0 && b.size === 0) return 0
  let inter = 0
  for (const w of a) if (b.has(w)) inter++
  const union = a.size + b.size - inter
  return union === 0 ? 0 : inter / union
}

const CLUSTER_THRESHOLD = 0.5

// clusterFindings mirrors internal/insight/cluster.go: greedy single-pass lexical
// grouping over title+rootCause. Input should be priority-sorted so each cluster's
// representative is the most urgent member.
export function clusterFindings(findings: InsightFinding[]): FindingCluster[] {
  const clusters: FindingCluster[] = []
  const repToks: Set<string>[] = []
  for (const f of findings) {
    const toks = tokenize(`${f.title} ${f.rootCause ?? ''}`)
    let placed = false
    for (let c = 0; c < clusters.length; c++) {
      if (jaccard(toks, repToks[c]) >= CLUSTER_THRESHOLD) {
        clusters[c].members.push(f)
        placed = true
        break
      }
    }
    if (!placed) {
      clusters.push({ representative: f, members: [f] })
      repToks.push(toks)
    }
  }
  return clusters
}

export interface FindingSummary {
  total: number
  appFix: number
  workspaceOpt: number
  high: number
  regressed: number
  open: number
}

export function summarize(findings: InsightFinding[]): FindingSummary {
  const s: FindingSummary = { total: findings.length, appFix: 0, workspaceOpt: 0, high: 0, regressed: 0, open: 0 }
  for (const f of findings) {
    if (f.channel === 'app-fix') s.appFix++
    else if (f.channel === 'workspace-opt') s.workspaceOpt++
    if (f.severity === 'high') s.high++
    if (f.regressed) s.regressed++
    const st = f.status || 'new'
    if (st !== 'dismissed' && st !== 'verified') s.open++
  }
  return s
}
