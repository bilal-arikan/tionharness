// Package insight is the retrospective session-scanning subsystem (_Docs/60).
// It scans past sessions through pluggable "lenses", skips sessions it has
// already scanned (incremental ledger), and routes findings to one of two
// channels: app-fix (bugs in TionSwarm itself) or workspace-opt (changes the
// user can apply inside the workspace).
//
// This file holds the canonical Finding model and its file-backed, signature-
// deduplicated store — the fleet-wide sibling of db.Lesson (internal/db/
// store_lessons.go): a repeat of the same shape bumps a counter instead of
// piling up duplicate rows.
package insight

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/google/uuid"
)

// Channel is where a finding is routed once produced.
type Channel string

const (
	// ChannelAppFix marks a finding whose root cause is in the TionSwarm
	// application itself. It is reported (never auto-applied, never coder-spawned)
	// so the user can use it to develop TionSwarm.
	ChannelAppFix Channel = "app-fix"
	// ChannelWorkspaceOpt marks a finding fixable inside the workspace without
	// touching app code (skill pruning, blocked tools, context tuning, ...).
	ChannelWorkspaceOpt Channel = "workspace-opt"
)

// Valid reports whether c is one of the two known channels.
func (c Channel) Valid() bool { return c == ChannelAppFix || c == ChannelWorkspaceOpt }

// FindingStatus is a finding's position in its review lifecycle (_Docs/60 §5).
type FindingStatus string

const (
	StatusNew       FindingStatus = "new"
	StatusTriaged   FindingStatus = "triaged"
	StatusAccepted  FindingStatus = "accepted"
	StatusApplied   FindingStatus = "applied"
	StatusVerified  FindingStatus = "verified"
	StatusDismissed FindingStatus = "dismissed"
)

// ValidStatus reports whether s is a known lifecycle status.
func ValidStatus(s FindingStatus) bool {
	switch s {
	case StatusNew, StatusTriaged, StatusAccepted, StatusApplied, StatusVerified, StatusDismissed:
		return true
	}
	return false
}

// Finding is one cross-session insight produced by a lens. Repeats of the same
// failure/opportunity shape are deduplicated by Signature: a repeat bumps
// Occurrences and records the new evidence session, so a recurring pattern
// accumulates weight rather than fragmenting into duplicate rows.
type Finding struct {
	ID          string        `json:"id"`
	LensID      string        `json:"lensId"`
	Channel     Channel       `json:"channel"`
	Signature   string        `json:"sig"` // dedupe key (lens-defined shape)
	Title       string        `json:"title"`
	RootCause   string        `json:"rootCause,omitempty"`
	ProposedFix string        `json:"proposedFix,omitempty"`
	FilePointer string        `json:"filePointer,omitempty"` // e.g. internal/providers/claudecli.go
	Severity    string        `json:"severity,omitempty"`    // low|med|high
	// EvidenceSessionIDs are the sessions this finding was observed in (dedup-merged).
	EvidenceSessionIDs []string      `json:"evidenceSessionIds,omitempty"`
	Occurrences        int           `json:"occurrences"`
	Status             FindingStatus `json:"status"`
	FirstSeen          int64         `json:"firstSeen"` // unix seconds
	LastSeen           int64         `json:"lastSeen"`  // unix seconds
	AppliedAt          int64         `json:"appliedAt,omitempty"`
	VerifiedAt         int64         `json:"verifiedAt,omitempty"`
	// Regressed is set when a finding that had been CLOSED (dismissed/applied/
	// verified) recurs in a later scan — i.e. something you thought you were done
	// with came back. It is the signal that a "fixed" issue is not actually fixed.
	Regressed   bool  `json:"regressed,omitempty"`
	RegressedAt int64 `json:"regressedAt,omitempty"` // unix seconds of the recurrence
}

// closedStatus reports whether a finding is in a terminal/closed state, so a
// fresh recurrence of it counts as a regression rather than normal accumulation.
func closedStatus(s FindingStatus) bool {
	switch s {
	case StatusDismissed, StatusApplied, StatusVerified:
		return true
	default:
		return false
	}
}

// FindingStore is the workspace-scoped findings.jsonl sidecar, guarded by its
// own mutex and written atomically (tmp+rename) like the lessons store.
type FindingStore struct {
	mu    sync.Mutex
	path  string
	items []Finding
}

// findingsRelPath is the store-root-relative location of the findings file.
var findingsRelPath = filepath.Join("insight", "findings.jsonl")

// OpenFindingStore loads (or lazily creates) the findings store rooted at the
// given store root (db.Root()). A missing file yields an empty store.
func OpenFindingStore(root string) (*FindingStore, error) {
	path := filepath.Join(root, findingsRelPath)
	items, err := readFindings(path)
	if err != nil {
		return nil, err
	}
	return &FindingStore{path: path, items: items}, nil
}

// Upsert records f, deduplicating by Signature: a repeat bumps Occurrences,
// refreshes LastSeen and the mutable text fields (newest wording wins), and
// merges the evidence session. A new signature is appended with a fresh ID.
// The caller supplies FirstSeen/LastSeen (unix seconds) so the store stays
// deterministic and unit-testable. Returns the stored finding.
func (s *FindingStore) Upsert(f Finding) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if f.Signature != "" {
		canon := canonSig(f.Signature)
		for i := range s.items {
			// Match on the canonical signature (case/punctuation-insensitive) so
			// formatting variants of the same shape merge instead of duplicating.
			if s.items[i].LensID == f.LensID && canonSig(s.items[i].Signature) == canon {
				s.items[i].Occurrences++
				// A closed finding that recurs is a REGRESSION: flag it (and stamp when)
				// so a "fixed"/"ignored" issue coming back is surfaced, not buried under
				// a silently-incrementing counter on a card that still reads as done.
				if closedStatus(s.items[i].Status) && !s.items[i].Regressed {
					s.items[i].Regressed = true
					s.items[i].RegressedAt = f.LastSeen
				}
				if f.LastSeen > 0 {
					s.items[i].LastSeen = f.LastSeen
				}
				if f.Title != "" {
					s.items[i].Title = f.Title
				}
				if f.RootCause != "" {
					s.items[i].RootCause = f.RootCause
				}
				if f.ProposedFix != "" {
					s.items[i].ProposedFix = f.ProposedFix
				}
				if f.Severity != "" {
					s.items[i].Severity = f.Severity
				}
				s.items[i].EvidenceSessionIDs = mergeStrings(s.items[i].EvidenceSessionIDs, f.EvidenceSessionIDs)
				merged := s.items[i]
				if err := writeFindings(s.path, s.items); err != nil {
					return Finding{}, err
				}
				return merged, nil
			}
		}
	}

	if f.ID == "" {
		f.ID = "FND-" + uuid.NewString()[:8]
	}
	if f.Occurrences <= 0 {
		f.Occurrences = 1
	}
	if f.Status == "" {
		f.Status = StatusNew
	}
	s.items = append(s.items, f)
	if err := writeFindings(s.path, s.items); err != nil {
		return Finding{}, err
	}
	return f, nil
}

// List returns findings ranked by PriorityScore (severity + recurrence, regressed
// first, resolved last), tie-broken by recency. Optional filters: a non-empty
// lensID or channel narrows the result; zero values match everything.
func (s *FindingStore) List(lensID string, channel Channel) []Finding {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Finding, 0, len(s.items))
	for _, f := range s.items {
		if lensID != "" && f.LensID != lensID {
			continue
		}
		if channel != "" && f.Channel != channel {
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := out[i].PriorityScore(), out[j].PriorityScore(); pi != pj {
			return pi > pj
		}
		return out[i].LastSeen > out[j].LastSeen
	})
	return out
}

// SetStatus updates one finding's lifecycle status by id, stamping AppliedAt/
// VerifiedAt from the supplied unix-seconds `at` when moving into those states.
// Returns false when the id is absent.
func (s *FindingStore) SetStatus(id string, status FindingStatus, at int64) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID != id {
			continue
		}
		s.items[i].Status = status
		// The user is making a fresh decision → acknowledge any prior regression.
		s.items[i].Regressed = false
		s.items[i].RegressedAt = 0
		switch status {
		case StatusApplied:
			s.items[i].AppliedAt = at
		case StatusVerified:
			s.items[i].VerifiedAt = at
		}
		if err := writeFindings(s.path, s.items); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// mergeStrings appends items of b not already in a, preserving order.
func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if v != "" && !seen[v] {
			a = append(a, v)
			seen[v] = true
		}
	}
	return a
}

func readFindings(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var fnd Finding
		if json.Unmarshal(sc.Bytes(), &fnd) == nil && fnd.ID != "" {
			out = append(out, fnd)
		}
	}
	return out, sc.Err()
}

func writeFindings(path string, items []Finding) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
