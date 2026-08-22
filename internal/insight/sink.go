package insight

import "time"

// AnalysisEvent reports ONE completed (lens, session) analysis. It is emitted the
// moment the analyzer returns — before phase 3 persists anything — so a caller can
// render the scan as live activity instead of a single report at the end.
//
// Findings is the analyzer's raw count for this pair (pre-dedupe): the store may
// upsert fewer of them into an already-known finding.
type AnalysisEvent struct {
	LensID       string
	SessionID    string
	SessionTitle string
	Findings     int
	Err          error
	Duration     time.Duration
}

// AnalysisSink receives one AnalysisEvent per analyzed pair, in COMPLETION order.
// It is called from the scanner's phase-2 worker goroutines, so an implementation
// must be safe for concurrent use.
type AnalysisSink func(AnalysisEvent)

// SetAnalysisSink installs the live per-analysis callback. Deliberately a setter
// rather than a NewScanner parameter: the sink is optional observability, and the
// constructor's signature is shared by every call site (including tests).
func (s *Scanner) SetAnalysisSink(fn AnalysisSink) { s.sink = fn }
