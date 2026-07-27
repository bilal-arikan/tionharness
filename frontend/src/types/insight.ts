// Retrospective session scanning (Insight, _Docs/60) — mirrors the Go models in
// internal/insight.

export type InsightChannel = 'app-fix' | 'workspace-opt'

export interface LensPrefilter {
  requiresAny?: string[]
  requiresAll?: string[]
  excludes?: string[]
  minCount?: Record<string, number>
  minTokens?: number
}

export interface InsightLens {
  id: string
  name: string
  description: string
  channel: InsightChannel
  enabled: boolean
  model?: string
  scope?: string[]
  prefilter: LensPrefilter
  path: string
}

export interface InsightFinding {
  id: string
  lensId: string
  channel: InsightChannel
  sig: string
  title: string
  rootCause?: string
  proposedFix?: string
  filePointer?: string
  severity?: string
  evidenceSessionIds?: string[]
  occurrences: number
  status: string
  firstSeen: number
  lastSeen: number
  appliedAt?: number
  verifiedAt?: number
  /** Set when a closed (dismissed/applied/verified) finding recurred — a "fixed" issue came back. */
  regressed?: boolean
  regressedAt?: number
}

export interface InsightSettings {
  appFixRepoPath?: string
  maxSessions?: number
  /** Hard ceiling on analyzer (LLM) calls per scan — the real cost driver. 0 = no cap. */
  maxAnalyzed?: number
  /** Only scan sessions active within the last N days. 0 = all history. */
  scanSinceDays?: number
  /** Standard 5-field cron expression driving automatic scans; empty = manual only. */
  autoScanCron?: string
  /** Agent whose own provider/model runs scans; empty = fall back to the first agent. */
  autoScanAgentId?: string
  /** APPLIED finding not seen for N days (and not regressed) → auto-verified. 0 = default (14). */
  autoVerifyDays?: number
  /** DISMISSED/VERIFIED finding untouched for N days → deleted. 0 = default (45). */
  pruneDays?: number
}

export interface InsightScanResult {
  sessions: number
  analyzed: number
  skipped: number
  prefiltered: number
  findings: number
  errors?: string[]
}

/** One fleet-merged app-fix finding plus the workspaces it surfaced in. */
export interface FleetFinding extends InsightFinding {
  workspaces: string[]
}

/** One scan-run log record (observability; not a session). */
export interface InsightRun {
  at: number
  durationMs: number
  trigger?: string
  lensIds?: string[]
  sessions: number
  analyzed: number
  skipped: number
  prefiltered: number
  findings: number
  errors: number
}
