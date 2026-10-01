package decider

import "time"

// DebugFilter bounds a metadata-only read. Limit applies to events, newest
// first; Summary describes all retained matches, not just the displayed tail.
type DebugFilter struct {
	Since       time.Time
	Authority   string
	Instance    string
	Ref         string
	TraceID     string
	WorkspaceID string
	Limit       int
}

type DebugSummary struct {
	Decisions          int     `json:"decisions"`
	Errors             int     `json:"errors"`
	Skipped            int     `json:"skipped"`
	Attempts           int     `json:"attempts"`
	Fallbacks          int     `json:"fallbacks"`
	Challengers        int     `json:"challengers"`
	Retries            int     `json:"retries"`
	Applied            int     `json:"applied"`
	Compared           int     `json:"compared"`
	Agreed             int     `json:"agreed"`
	ChallengerCompared int     `json:"challengerCompared"`
	ChallengerAgreed   int     `json:"challengerAgreed"`
	Tests              int     `json:"tests"`
	TestErrors         int     `json:"testErrors"`
	Warnings           int     `json:"warnings"`
	Trimmed            int     `json:"trimmed"`
	P50Ms              int64   `json:"p50Ms"`
	P95Ms              int64   `json:"p95Ms"`
	CostUSD            float64 `json:"costUsd"`
}

type DebugIssue struct {
	Code  string `json:"code"`
	Count int    `json:"count"`
}

type DebugReport struct {
	Events         []DebugEvent `json:"events"`
	Summary        DebugSummary `json:"summary"`
	Issues         []DebugIssue `json:"issues"`
	RetainedEvents int          `json:"retainedEvents"`
	MatchedEvents  int          `json:"matchedEvents"`
	OldestAt       int64        `json:"oldestAt,omitempty"`
	Truncated      bool         `json:"truncated"`
	Capacity       int          `json:"capacity"`
}

// Debug reports operational evidence. Agreement compares decision policies;
// it is not a correctness or calibration score without independent labels.
func (h *Hub) Debug(filter DebugFilter) DebugReport {
	events := h.debug.snapshot()
	limit := filter.Limit
	if limit <= 0 {
		limit = 500
	}
	limit = min(limit, debugCapacity)
	report := DebugReport{Events: []DebugEvent{}, Issues: []DebugIssue{}, RetainedEvents: len(events), Capacity: debugCapacity}
	if len(events) > 0 {
		report.OldestAt = events[0].At
	}
	var latencies []int64
	challengerSkipped := 0
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if filter.WorkspaceID != "" && e.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if !filter.Since.IsZero() && e.At < filter.Since.UnixMilli() {
			continue
		}
		if filter.Authority != "" && e.Authority != filter.Authority {
			continue
		}
		if filter.Instance != "" && e.Instance != filter.Instance {
			continue
		}
		if filter.Ref != "" && e.Ref != filter.Ref && e.SessionID != filter.Ref && e.TurnID != filter.Ref {
			continue
		}
		if filter.TraceID != "" && e.TraceID != filter.TraceID {
			continue
		}
		report.MatchedEvents++
		if len(report.Events) < limit {
			report.Events = append(report.Events, e)
		}
		s := &report.Summary
		switch e.Stage {
		case "completed":
			if e.Role == "test" {
				s.Tests++
				if e.Error != "" {
					s.TestErrors++
				}
				continue
			}
			if e.Error == "off" {
				continue
			}
			s.Decisions++
			if e.Error != "" {
				s.Errors++
			}
			latencies = append(latencies, e.LatencyMs)
		case "skipped":
			s.Skipped++
			if e.Error == "challenger_busy" {
				challengerSkipped++
			}
		case "transport":
			if e.HTTPAttempt > 1 {
				s.Retries++
			}
		case "attempt":
			s.Attempts++
			s.CostUSD += e.CostUSD
			if e.Role == "fallback" {
				s.Fallbacks++
			}
			if e.Role == "challenger" {
				s.Challengers++
			}
			if len(e.Warnings) > 0 {
				s.Warnings++
			}
			if e.StateTrimmed {
				s.Trimmed++
			}
		case "outcome":
			if e.Applied {
				s.Applied++
			}
			if e.Error == "" && e.Outcome != "" && e.Baseline != "" {
				if e.Role == "challenger" {
					s.ChallengerCompared++
					if e.Outcome == e.Baseline {
						s.ChallengerAgreed++
					}
					continue
				}
				s.Compared++
				if e.Outcome == e.Baseline {
					s.Agreed++
				}
			}
		}
	}
	report.Truncated = report.MatchedEvents > len(report.Events)
	report.Summary.P50Ms = percentile(latencies, .50)
	report.Summary.P95Ms = percentile(latencies, .95)
	add := func(code string, count int) {
		report.Issues = append(report.Issues, DebugIssue{Code: code, Count: count})
	}
	if report.Summary.Compared < 50 {
		add("insufficient_evidence", report.Summary.Compared)
	}
	if report.Summary.Decisions >= 10 && report.Summary.Errors*10 >= report.Summary.Decisions {
		add("high_error_rate", report.Summary.Errors)
	}
	if report.Summary.P95Ms > 1500 {
		add("slow_decisions", int(report.Summary.P95Ms))
	}
	if report.Summary.Warnings > 0 {
		add("degraded_probabilities", report.Summary.Warnings)
	}
	if report.Summary.Trimmed > 0 {
		add("trimmed_context", report.Summary.Trimmed)
	}
	if challengerSkipped > 0 {
		add("challenger_busy", challengerSkipped)
	}
	return report
}
