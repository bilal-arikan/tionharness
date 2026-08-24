package insight

import "sort"

// Fleet-level rollup. app-fix findings describe bugs in TionHarness ITSELF, so the
// same bug legitimately surfaces in several workspaces (each scanning its own
// sessions) — with slightly different signatures. Per-workspace stores can't see
// each other, so RollupAppFix merges them into one fleet-wide backlog: dedup by
// canonical signature ACROSS lenses and workspaces, summing weight and recording
// which workspaces hit it.

// FleetFinding is a fleet-merged app-fix finding plus the workspaces it came from.
type FleetFinding struct {
	Finding
	Workspaces []string `json:"workspaces"`
}

// RollupAppFix merges per-workspace app-fix findings into a deduplicated fleet
// backlog. byWorkspace maps a workspace label → its findings (any channel; only
// app-fix are considered). Dedup is by canonical signature across everything, so
// the same TionHarness bug reported four ways collapses to one row carrying the
// combined occurrences, evidence and originating workspaces. Sorted by priority.
func RollupAppFix(byWorkspace map[string][]Finding) []FleetFinding {
	merged := map[string]*FleetFinding{}
	var order []string // preserve first-seen order for stable output before sort
	wsSeen := map[string]map[string]bool{}

	for ws, findings := range byWorkspace {
		for _, f := range findings {
			if f.Channel != ChannelAppFix || f.Signature == "" {
				continue
			}
			key := canonSig(f.Signature)
			m, ok := merged[key]
			if !ok {
				cp := f
				m = &FleetFinding{Finding: cp}
				merged[key] = m
				wsSeen[key] = map[string]bool{}
				order = append(order, key)
			} else {
				m.Occurrences += f.Occurrences
				m.EvidenceSessionIDs = mergeStrings(m.EvidenceSessionIDs, f.EvidenceSessionIDs)
				if severityRank(f.Severity) > severityRank(m.Severity) {
					m.Severity = f.Severity // keep the worst severity reported anywhere
				}
				if f.Regressed {
					m.Regressed = true
				}
				if f.LastSeen > m.LastSeen {
					m.LastSeen = f.LastSeen
				}
			}
			if !wsSeen[key][ws] {
				wsSeen[key][ws] = true
				m.Workspaces = append(m.Workspaces, ws)
			}
		}
	}

	out := make([]FleetFinding, 0, len(order))
	for _, key := range order {
		out = append(out, *merged[key])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := out[i].PriorityScore(), out[j].PriorityScore(); pi != pj {
			return pi > pj
		}
		return out[i].LastSeen > out[j].LastSeen
	})
	return out
}

// severityRank orders severities so the fleet row keeps the worst one seen.
func severityRank(s string) int {
	switch s {
	case "high":
		return 3
	case "med", "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}
