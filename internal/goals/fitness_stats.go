package goals

import (
	"fmt"
	"slices"
)

// Robust per-bucket statistics for the configuration-version breakdown
// (_Docs/83 §4.2). A version bucket's mean cost is easily dominated by a
// handful of heavy sessions (one long coordinator run, one big card), so a
// "+75%" delta between two small buckets says more about the job mix than
// about the configuration. The bucket therefore carries the median and a
// trimmed mean next to the mean, a work-normalised secondary metric (cost per
// tool call), and an insufficient-data gate the UI honours before it draws a
// delta badge.

const (
	// MinBucketSessions is the smallest number of costed sessions a version
	// bucket needs before its delta against another bucket is shown.
	MinBucketSessions = 50
	// MaxTop3CostShare is the largest share of a bucket's total cost its three
	// most expensive sessions may hold before the bucket counts as dominated
	// by outliers.
	MaxTop3CostShare = 0.40
	// TrimFraction is cut from EACH end of the sorted samples for the trimmed
	// mean (10% trimmed mean).
	TrimFraction = 0.10
)

// Distribution is the shape of a per-session metric over one group.
type Distribution struct {
	N           int     `json:"n"`
	Mean        float64 `json:"mean"`
	Median      float64 `json:"median"`
	TrimmedMean float64 `json:"trimmedMean"`
	// Top3Share is the share of the samples' sum held by the three largest.
	Top3Share float64 `json:"top3Share"`
}

// BucketStats is the cost picture of one group of sessions, independent of
// which metric the goal optimises: it drives the insufficient-data gate and
// the cost-per-tool-call secondary metric.
type BucketStats struct {
	Cost Distribution `json:"cost"`
	// ToolCalls is the lifetime tool-call total of the costed sessions.
	ToolCalls int `json:"toolCalls"`
	// CostPerToolCall is total cost / ToolCalls; nil when no tool call ran.
	CostPerToolCall *float64 `json:"costPerToolCall,omitempty"`
	// Insufficient is true when the bucket is too small or too skewed for a
	// version-vs-version delta to mean anything; Reasons says why.
	Insufficient bool     `json:"insufficient"`
	Reasons      []string `json:"reasons,omitempty"`
}

// distributionOf computes mean, median, trimmed mean and top-3 share. It
// returns ok=false for an empty sample.
func distributionOf(samples []float64) (Distribution, bool) {
	n := len(samples)
	if n == 0 {
		return Distribution{}, false
	}
	s := append([]float64(nil), samples...)
	slices.Sort(s)
	var sum float64
	for _, v := range s {
		sum += v
	}
	d := Distribution{N: n, Mean: sum / float64(n)}
	if n%2 == 1 {
		d.Median = s[n/2]
	} else {
		d.Median = (s[n/2-1] + s[n/2]) / 2
	}
	cut := int(float64(n) * TrimFraction)
	kept := s[cut : n-cut]
	var ksum float64
	for _, v := range kept {
		ksum += v
	}
	d.TrimmedMean = ksum / float64(len(kept))
	if sum > 0 {
		var top float64
		for i := n - 1; i >= 0 && i >= n-3; i-- {
			top += s[i]
		}
		d.Top3Share = top / sum
	}
	return d, true
}

// bucketStats computes the cost picture over rows (sessions with tokens only:
// a session that never reached the model has no cost to describe).
func bucketStats(rows []SessionRow, in FitnessInputs) BucketStats {
	var costs []float64
	var total float64
	var st BucketStats
	for _, s := range rows {
		u, ok := in.Usage[s.ID]
		if !ok || u.Tokens == 0 {
			continue
		}
		costs = append(costs, u.CostUSD)
		total += u.CostUSD
		st.ToolCalls += s.ToolCalls
	}
	if d, ok := distributionOf(costs); ok {
		st.Cost = d
	}
	if st.ToolCalls > 0 {
		v := total / float64(st.ToolCalls)
		st.CostPerToolCall = &v
	}
	if st.Cost.N < MinBucketSessions {
		st.Insufficient = true
		st.Reasons = append(st.Reasons, fmt.Sprintf("n=%d < %d", st.Cost.N, MinBucketSessions))
	}
	if st.Cost.Top3Share > MaxTop3CostShare {
		st.Insufficient = true
		st.Reasons = append(st.Reasons, fmt.Sprintf("top-3 sessions hold %.0f%% of cost (> %.0f%%)", st.Cost.Top3Share*100, MaxTop3CostShare*100))
	}
	return st
}

// isProviderFailure reports a session whose run failed before the model did
// any work: a failed run state with zero tokens and zero tool calls — a
// provider/auth/startup failure (e.g. an unauthenticated CLI), not a
// configuration outcome. Such sessions are counted separately so a login
// problem cannot break a configuration's error-rate guardrail.
func isProviderFailure(s SessionRow, in FitnessInputs) bool {
	if !isFailedRunState(s.RunState) || s.ToolCalls > 0 {
		return false
	}
	u, ok := in.Usage[s.ID]
	return !ok || u.Tokens == 0
}

func countProviderFailures(rows []SessionRow, in FitnessInputs) int {
	n := 0
	for _, s := range rows {
		if isProviderFailure(s, in) {
			n++
		}
	}
	return n
}
