// parseJudgeLabel splits a judge-mode branch trace label into the arm it routed
// to and the judgement note the engine appended (internal/orchestration/judge.go):
//
//   "A feature request (judge 0.93)"        → arm "A feature request", note "judge 0.93"
//   "default (judge unsure, 0.41)"          → arm "default", note "judge unsure, 0.41"
//   "default (judge error: decider is off)" → arm "default", note "judge error: decider is off"
//   "no match (judge unsure, 0.30)"         → arm "no match", note "judge unsure, 0.30"
//
// A label without a note is returned whole.
export function parseJudgeLabel(label: string): { arm: string; note: string } {
  const m = /^(.*) \(((?:judge|no options)[^]*)\)$/.exec(label)
  return m ? { arm: m[1], note: m[2] } : { arm: label, note: '' }
}
