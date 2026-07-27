package insight

// Findings lifecycle maintenance. Unlike lessons (which age out purely by time,
// default 2 days), findings only ever grew — dismissed/verified items lingered
// forever and an
// "applied" fix was never confirmed. Maintain closes both gaps: it auto-verifies
// applied findings that stopped recurring, and prunes long-resolved ones.

const (
	// DefaultAutoVerifyAge: an APPLIED finding whose last sighting is older than
	// this (i.e. it has not recurred in any scan since) is considered fixed →
	// verified. A regressed finding is never auto-verified (it demonstrably came
	// back). Overridable per workspace via Settings.AutoVerifyDays.
	DefaultAutoVerifyAge = int64(14 * 24 * 60 * 60) // 14 days

	// DefaultPruneAge: a DISMISSED or VERIFIED finding untouched for this long is
	// deleted, so the store does not accumulate resolved noise indefinitely.
	// Overridable per workspace via Settings.PruneDays.
	DefaultPruneAge = int64(45 * 24 * 60 * 60) // 45 days
)

// MaintainResult reports what a maintenance pass changed.
type MaintainResult struct {
	AutoVerified int `json:"autoVerified"`
	Pruned       int `json:"pruned"`
}

// Maintain runs one lifecycle pass at time now (unix seconds; injected so the
// pass stays deterministic and unit-testable):
//   - APPLIED findings not seen for autoVerifyAge and not regressed → VERIFIED.
//   - DISMISSED/VERIFIED findings not seen for pruneAge → removed.
//
// autoVerifyAge/pruneAge are in seconds; a value <= 0 falls back to the built-in
// default. It rewrites the store only when something changed.
func (s *FindingStore) Maintain(now, autoVerifyAge, pruneAge int64) (MaintainResult, error) {
	if autoVerifyAge <= 0 {
		autoVerifyAge = DefaultAutoVerifyAge
	}
	if pruneAge <= 0 {
		pruneAge = DefaultPruneAge
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var res MaintainResult
	kept := s.items[:0:0]
	changed := false
	for _, f := range s.items {
		// Auto-verify applied fixes that stopped recurring.
		if f.Status == StatusApplied && !f.Regressed && f.LastSeen > 0 && now-f.LastSeen > autoVerifyAge {
			f.Status = StatusVerified
			f.VerifiedAt = now
			res.AutoVerified++
			changed = true
		}
		// Prune long-resolved findings.
		if (f.Status == StatusDismissed || f.Status == StatusVerified) && f.LastSeen > 0 && now-f.LastSeen > pruneAge {
			res.Pruned++
			changed = true
			continue // drop
		}
		kept = append(kept, f)
	}
	if !changed {
		return res, nil
	}
	s.items = kept
	if err := writeFindings(s.path, s.items); err != nil {
		return MaintainResult{}, err
	}
	return res, nil
}
