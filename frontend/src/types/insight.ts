// Retrospective session scanning (Insight, _Docs/60) — mirrors the Go models in
// internal/insight.
import type { SeedDefaultState } from './seed'

export type InsightChannel = 'app-fix' | 'workspace-opt' | 'recipe-opt' | 'evolution'

/** Workspace-evolver proposal (_Docs/83 E2, insight.EvolutionProposal): one
 * measured change to one field of one entity toward a goal. Suggestion-only. */
export interface EvolutionProposal {
  goalId: string
  surface: string
  entityId: string
  field: string
  action: string
  value?: string
  removes?: string
  expectedMetric?: string
  expectedDelta?: number
  sideEffects?: string[]
  evidence: string
  snapshotHash?: string
  lowConfidence?: boolean
  /** change | conflict | escalation */
  kind?: string
}

// Recipe optimizer proposal (Rota F4, insight.RecipeProposal): one measured,
// conditional change to one coordinator recipe. Suggestion-only in v1.
export interface RecipeProposal {
  slug: string
  version?: string
  // prune_phase | make_optional | prune_watcher | change_profile | add_gate |
  // bind_watcher | split_phase | merge_phase | rollback_version
  action: string
  target?: string
  value?: string
  removes?: string
  evidence: string
}

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
  // How this file compares to the lens TionHarness ships (see SeedDefaultState).
  // Absent = user-authored, so there is no default to badge against or restore.
  defaultState?: SeedDefaultState
}

/**
 * Evidence for the "applied" status: the workspace entity that was actually
 * mutated. Both halves are mandatory — the backend rejects a partial one with
 * 400 "applied requires evidence: entityType+entityId".
 */
export interface AppliedEntity {
  entityType: string
  entityId: string
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
  /** Entity the fix was applied to; required for status 'applied', and what auto-verify keys off. */
  appliedEntity?: AppliedEntity
  /** Set when a closed (dismissed/applied/verified) finding recurred — a "fixed" issue came back. */
  regressed?: boolean
  regressedAt?: number
  /** Structured recipe change behind a recipe-opt finding (Rota F4). */
  proposal?: RecipeProposal
  /** Structured change behind an evolution finding (_Docs/83 E2). */
  evolution?: EvolutionProposal
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
  // The read-only 'insight' session this run wrote its transcript into. Empty on
  // legacy records written before per-run sessions existed, so callers must
  // treat it as optional and skip the deep link when absent.
  sessionId?: string
}
