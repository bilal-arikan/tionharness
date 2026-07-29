// Client-side counterpart of db.FlowRun's lineage helpers (RootOf / IsRootRun).
//
// A root run carries NO lineage fields at all — the backend omits them — so
// "rootRunId is absent" means "this run IS the root", not "unknown". Every
// consumer that addresses the tree endpoint (/api/flow-runs/{id}/tree) or the
// tree event scope (subscribeFlowTree) has to resolve that encoding, and each
// place that re-derives it inline is a place it can be got wrong. Decode it here.

import type { FlowRun } from '@/types'

// flowRunRootOf returns the id of the top of this run's tree. A root run reports
// its own id.
export function flowRunRootOf(run: FlowRun): string {
  return run.rootRunId || run.id
}

// isRootFlowRun reports whether nothing launched this run — the parent link, not
// the root link, is what decides it: a root's rootRunId is empty AND so is its
// parentRunId, but only the parent link stays empty for exactly the roots.
export function isRootFlowRun(run: FlowRun): boolean {
  return !run.parentRunId
}
