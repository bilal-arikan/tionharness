package insight

// Priority scoring ranks findings for triage. The retrospective scanner can
// surface dozens of findings at once (a first full scan of a large workspace
// produced ~100); listing them newest-first buries the ones that actually matter.
// PriorityScore gives a single comparable number so List (and the panel/tools)
// lead with high-severity, frequently-recurring and REGRESSED findings, and sink
// resolved/ignored ones.

// severity weights — high issues outrank low ones regardless of recurrence.
const (
	sevWeightHigh = 30
	sevWeightMed  = 20
	sevWeightLow  = 10
)

// occurrenceCap bounds the recurrence contribution so a runaway counter can't
// dominate severity (a low-severity ×50 must not outrank a fresh high).
const occurrenceCap = 20

// regressedBonus floats a regression to the very top: a "fixed"/"ignored" issue
// that came back is the single most actionable signal the scanner produces.
const regressedBonus = 100

// resolvedPenalty sinks dismissed/verified findings below every open one.
const resolvedPenalty = 1000

// appliedPenalty keeps applied findings visible (for regression watch) but below
// untriaged/accepted work.
const appliedPenalty = 50

// PriorityScore is the composite triage rank: severity + capped occurrences, plus
// a big bonus when regressed, minus penalties for closed states. Higher = more
// urgent. Pure and deterministic (no clock), so it is safe to sort by anywhere.
func (f Finding) PriorityScore() int {
	sev := sevWeightLow
	switch f.Severity {
	case "high":
		sev = sevWeightHigh
	case "med", "medium":
		sev = sevWeightMed
	}
	occ := f.Occurrences
	if occ > occurrenceCap {
		occ = occurrenceCap
	}
	score := sev + occ
	if f.Regressed {
		score += regressedBonus
	}
	switch f.Status {
	case StatusDismissed, StatusVerified:
		score -= resolvedPenalty
	case StatusApplied:
		score -= appliedPenalty
	}
	return score
}
