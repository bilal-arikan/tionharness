// Package insight is the retrospective session-scanning subsystem (_Docs/60).
// It scans past sessions through pluggable "lenses", skips sessions it has
// already scanned (incremental ledger), and routes findings to one of two
// channels: app-fix (bugs in TionHarness itself) or workspace-opt (changes the
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
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
)

// Channel is where a finding is routed once produced.
type Channel string

const (
	// ChannelAppFix marks a finding whose root cause is in the TionHarness
	// application itself. It is reported (never auto-applied, never coder-spawned)
	// so the user can use it to develop TionHarness.
	ChannelAppFix Channel = "app-fix"
	// ChannelWorkspaceOpt marks a finding fixable inside the workspace without
	// touching app code (skill pruning, blocked tools, context tuning, ...).
	ChannelWorkspaceOpt Channel = "workspace-opt"
	// ChannelRecipeOpt marks a recipe-optimizer proposal (Rota F4): a measured,
	// conditional change to one coordinator recipe. Suggestion-only in v1 — the
	// user (or a later opt-in applier) edits the recipe.
	ChannelRecipeOpt Channel = "recipe-opt"
)

// Valid reports whether c is one of the known channels.
func (c Channel) Valid() bool {
	return c == ChannelAppFix || c == ChannelWorkspaceOpt || c == ChannelRecipeOpt
}

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
	// AnswerOnly is an in-memory grouped-analysis marker proving that a lens
	// returned an explicit empty findings array. Scanner consumes it before store.
	AnswerOnly  bool    `json:"-"`
	ID          string  `json:"id"`
	LensID      string  `json:"lensId"`
	Channel     Channel `json:"channel"`
	Signature   string  `json:"sig"` // dedupe key (lens-defined shape)
	Title       string  `json:"title"`
	RootCause   string  `json:"rootCause,omitempty"`
	ProposedFix string  `json:"proposedFix,omitempty"`
	FilePointer string  `json:"filePointer,omitempty"` // e.g. internal/providers/claudecli.go
	Severity    string  `json:"severity,omitempty"`    // low|med|high
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
	// AppliedEntity is the workspace entity that was actually mutated to apply
	// this finding (e.g. skill/tionharness-tool-discovery). It is the evidence
	// StatusApplied requires: without it "applied" is an unbacked claim, so the
	// store refuses the transition (ErrAppliedNeedsEvidence).
	AppliedEntity *AppliedEntity `json:"appliedEntity,omitempty"`
	// Proposal is the structured recipe change behind a recipe-opt finding
	// (Rota F4); nil on the other channels.
	Proposal *RecipeProposal `json:"proposal,omitempty"`
	// LastRunID is the scan run (insight.RunRecord.ID) that most recently produced
	// or re-confirmed this finding. It is what makes "show me what the run that
	// just finished surfaced" answerable: without it a consumer triggered by one
	// scan can only ask for status:new and gets the whole untriaged backlog.
	LastRunID string `json:"lastRunId,omitempty"`
}

// RecipeProposal is what the recipe optimizer proposes: one action on one
// target of one recipe version, with the metric that justifies it.
type RecipeProposal struct {
	Slug    string `json:"slug"`
	Version string `json:"version,omitempty"`
	// Action: prune_phase | make_optional | prune_watcher | change_profile |
	// add_gate | bind_watcher | split_phase | merge_phase | rollback_version.
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Value  string `json:"value,omitempty"`
	// Removes names what an addition drops to stay within the growth budget.
	Removes  string `json:"removes,omitempty"`
	Evidence string `json:"evidence"`
}

// AppliedEntity names the workspace entity a finding was applied to. Both fields
// are mandatory — a type without an id (or the reverse) is not evidence.
type AppliedEntity struct {
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
}

// Valid reports whether e carries both halves of the evidence.
func (e *AppliedEntity) Valid() bool {
	return e != nil && strings.TrimSpace(e.EntityType) != "" && strings.TrimSpace(e.EntityID) != ""
}

// String renders the evidence as entityType/entityId.
func (e *AppliedEntity) String() string {
	if e == nil {
		return ""
	}
	return e.EntityType + "/" + e.EntityID
}

// ErrAppliedNeedsEvidence is returned when a caller tries to move a finding to
// StatusApplied without naming the entity it changed. Closing a finding without
// having touched anything is what "accepted" and "dismissed" are for; silently
// downgrading the request would hide the mistake.
var ErrAppliedNeedsEvidence = errors.New("applied requires evidence: entityType+entityId")

// ClosedStatus reports whether a finding is in a terminal/closed state, so a
// fresh recurrence of it counts as a regression rather than normal accumulation
// — and so downstream consumers (lesson promotion) can skip what the user has
// already applied or dismissed.
func ClosedStatus(s FindingStatus) bool {
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
				return s.mergeInto(i, f)
			}
		}
		// Lens-independent pass: one root cause is usually visible to several
		// lenses, each slugging it with its own signature, which used to produce a
		// separate card per lens. Match on the canonical TOPIC (tool + error shape)
		// within the same channel so those land on one card.
		if topic := canonTopic(f.LensID, f.Signature); topic != "" && f.Channel != "" {
			for i := range s.items {
				if s.items[i].Channel != f.Channel {
					continue
				}
				if canonTopic(s.items[i].LensID, s.items[i].Signature) == topic {
					return s.mergeInto(i, f)
				}
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

// mergeInto folds f into the stored finding at index i and persists the store.
// The caller holds s.mu and has already decided the two denote the same finding.
func (s *FindingStore) mergeInto(i int, f Finding) (Finding, error) {
	s.items[i].Occurrences++
	// A closed finding that recurs is a REGRESSION: flag it (and stamp when)
	// so a "fixed"/"ignored" issue coming back is surfaced, not buried under
	// a silently-incrementing counter on a card that still reads as done.
	if ClosedStatus(s.items[i].Status) && !s.items[i].Regressed {
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
	// A recurrence belongs to the run that saw it again, so a run-scoped listing
	// includes findings this run re-confirmed, not only brand-new signatures.
	if f.LastRunID != "" {
		s.items[i].LastRunID = f.LastRunID
	}
	s.items[i].EvidenceSessionIDs = mergeStrings(s.items[i].EvidenceSessionIDs, f.EvidenceSessionIDs)
	merged := s.items[i]
	if err := writeFindings(s.path, s.items); err != nil {
		return Finding{}, err
	}
	return merged, nil
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
// applied is the evidence entity, mandatory for StatusApplied (see
// ErrAppliedNeedsEvidence) and ignored otherwise. Returns false when the id is
// absent.
func (s *FindingStore) SetStatus(id string, status FindingStatus, at int64, applied *AppliedEntity) (bool, error) {
	updated, err := s.SetStatusMany([]string{id}, status, at, applied)
	if err != nil {
		return false, err
	}
	return len(updated) > 0, nil
}

// SetStatusMany applies one status transition to several findings in a single
// pass and ONE store rewrite (the per-id loop used to rewrite findings.jsonl
// once per id). Returns the ids that actually existed, in store order; ids that
// are absent are simply missing from the result, so the caller can report them.
//
// Moving to StatusApplied without valid evidence fails with
// ErrAppliedNeedsEvidence and writes nothing — the transition is rejected, not
// downgraded.
func (s *FindingStore) SetStatusMany(ids []string, status FindingStatus, at int64, applied *AppliedEntity) ([]string, error) {
	if status == StatusApplied && !applied.Valid() {
		return nil, ErrAppliedNeedsEvidence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id != "" {
			want[id] = true
		}
	}
	var updated []string
	for i := range s.items {
		if !want[s.items[i].ID] {
			continue
		}
		s.items[i].Status = status
		// The user is making a fresh decision → acknowledge any prior regression.
		s.items[i].Regressed = false
		s.items[i].RegressedAt = 0
		switch status {
		case StatusApplied:
			s.items[i].AppliedAt = at
			s.items[i].AppliedEntity = applied
		case StatusVerified:
			s.items[i].VerifiedAt = at
		}
		updated = append(updated, s.items[i].ID)
	}
	if len(updated) == 0 {
		return nil, nil
	}
	if err := writeFindings(s.path, s.items); err != nil {
		return nil, err
	}
	return updated, nil
}

// Delete removes one finding by id, rewriting the store. Returns false when the
// id is absent (a no-op, not an error). Unlike Dismiss (a status), Delete drops
// the row entirely — used by the board-style triage UI's per-card/bulk delete.
func (s *FindingStore) Delete(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.items {
		if s.items[i].ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			if err := writeFindings(s.path, s.items); err != nil {
				return false, err
			}
			return true, nil
		}
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
