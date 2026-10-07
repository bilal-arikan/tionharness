// Package indexstate is the process-wide lifecycle ledger for the search
// indexes TionHarness builds on the user's behalf — zvec-grep's vector store and
// codebase-memory's graph — keyed by (tool, root).
//
// It exists because the capability prompt tells the agent "never create, rebuild
// or drop an index yourself; TionHarness manages them", and that claim has to be
// backed by something. A one-shot best-effort create is not management: it
// cannot tell a finished index from a failed one, cannot notice that the
// embedding model changed under it, and reports nothing to the user.
//
// The package is deliberately free of any dependency on internal/agent: the
// agent runtime DRIVES the transitions and the HTTP API READS them, so the
// ledger has to sit below both. It holds state only — running a CLI is the
// caller's job (see internal/agent/indexmanager.go).
package indexstate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/fspath"
)

// Phase is where one (tool, root) index sits in its lifecycle.
//
// The set is closed and ordered by knowledge, not by time: Missing means "no
// index and nothing running", Failed means "a run finished badly and we know
// why". A failed run NEVER decays back to Ready — that is the whole point of
// tracking a phase instead of a boolean.
type Phase string

const (
	// PhaseMissing: no index covers this root, and no run is in flight.
	PhaseMissing Phase = "missing"
	// PhaseIndexing: a create/refresh/rebuild run is in flight right now.
	PhaseIndexing Phase = "indexing"
	// PhaseReady: an index exists and matches the current embedding model and
	// tool version.
	PhaseReady Phase = "ready"
	// PhaseStale: an index exists but has fallen behind the working tree; a
	// refresh brings it back without discarding the store.
	PhaseStale Phase = "stale"
	// PhaseFailed: the last run failed. The reason is kept in Entry.Error and
	// logged by the caller. An index in this phase is never reported as usable.
	PhaseFailed Phase = "failed"
)

// Action is the kind of run that moved, or is moving, an entry.
//
// Drop is listed here but is NOT reachable from any automatic path: see
// Manager.Drop, which requires an explicit user confirmation token.
const (
	// ActionCreate builds an index where none existed.
	ActionCreate = "create"
	// ActionRefresh re-embeds a stale index in place, keeping the store.
	ActionRefresh = "refresh"
	// ActionRebuild discards and re-creates the store. Required when the
	// embedding model changes (vectors from different models are not comparable)
	// or the tool version changes its on-disk format.
	ActionRebuild = "rebuild"
	// ActionDrop deletes an index. User-confirmed only.
	ActionDrop = "drop"
)

// Entry is one index's recorded lifecycle state.
//
// Embedding and ToolVersion are what the CURRENT store was built with, read back
// from the tool's own manifest where it has one. They are the rebuild trigger:
// when either differs from what the host would use today, the existing vectors
// cannot be compared against new queries and a refresh would silently mix them.
type Entry struct {
	Tool  string `json:"tool"`
	Root  string `json:"root"`
	Phase Phase  `json:"phase"`
	// Action is the run that produced this phase ("" before the first run).
	Action string `json:"action,omitempty"`
	// Embedding is the model the store was built with, e.g.
	// "local/potion-code-16m-v2".
	Embedding string `json:"embedding,omitempty"`
	// ToolVersion is the indexing tool's version at build time.
	ToolVersion string `json:"toolVersion,omitempty"`
	// Error explains PhaseFailed. Never cleared by anything except a later run.
	Error string `json:"error,omitempty"`
	// StartedAt / UpdatedAt bound the last run.
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Run is the claim token of the last run Begin granted on this entry (0 before
	// the first one). It is unique across the ledger, so a Succeed or Fail
	// carrying an older token — a superseded run, or a run whose entry was
	// Forgotten and re-claimed — can be told apart from the run that owns the
	// entry now.
	Run uint64 `json:"run,omitempty"`
}

// ErrStaleClaim is returned by Succeed and Fail when the caller does not hold
// the entry's current claim: the entry is not in PhaseIndexing, was never
// claimed, was Forgotten, or was re-claimed by a newer run. The entry is left
// untouched — a late report must never overwrite what a newer run recorded, nor
// resurrect a failed run as ready.
var ErrStaleClaim = errors.New("index run does not hold the current claim")

// Usable reports whether a search against this index would return meaningful
// results. Stale counts: a lagging index still answers, it just misses the newest
// edits. Failed and Missing never count, which is what keeps a broken index from
// being advertised to the model as working.
func (e Entry) Usable() bool {
	return e.Phase == PhaseReady || e.Phase == PhaseStale
}

// Key identifies one index: the tool that owns it and the root it covers.
type Key struct {
	Tool string
	Root string
}

// NewKey normalises a (tool, root) pair into a map key. Root is compared the way
// the host filesystem does — case-insensitively on Windows and macOS, where the
// same repository reached as C:\Repo and c:\repo (or /Users/x/Repo and
// /users/x/repo) must not mint two ledger entries and two concurrent index runs.
func NewKey(tool, root string) Key {
	root = filepath.ToSlash(fspath.Key(strings.TrimSpace(root)))
	return Key{Tool: strings.TrimSpace(tool), Root: root}
}

// Manager is the ledger. The zero value is not usable; call New.
type Manager struct {
	mu      sync.Mutex
	entries map[Key]*Entry
	// lastRun is the most recently issued claim token. Tokens are never reused,
	// including across Forget, so a stale token cannot collide with a live one.
	lastRun uint64
	// now is the clock, injectable so tests can assert timestamps.
	now func() time.Time
}

// New returns an empty ledger.
func New() *Manager {
	return &Manager{entries: make(map[Key]*Entry), now: time.Now}
}

// SetClock replaces the ledger's clock. Test seam.
func (m *Manager) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}

// Get returns the recorded entry for one index and whether it has ever been
// seen. An unseen index is NOT reported as Missing: "we have no record" and "we
// looked and there is nothing there" are different answers, and only the caller
// that inspected the disk may assert the second one.
func (m *Manager) Get(tool, root string) (Entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[NewKey(tool, root)]
	if !ok {
		return Entry{}, false
	}
	return *e, true
}

// List returns every recorded entry, ordered by tool then root so the API and
// the UI get a stable list instead of Go's randomised map order.
func (m *Manager) List() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Entry, 0, len(m.entries))
	for _, e := range m.entries {
		out = append(out, *e)
	}
	sortEntries(out)
	return out
}

// Observe records what an inspection of the disk found, WITHOUT starting
// anything: phase, and the embedding/version the existing store was built with.
//
// It refuses to overwrite PhaseIndexing: a run in flight owns the entry, and a
// concurrent observer reading a half-written manifest must not declare it ready.
func (m *Manager) Observe(tool, root string, phase Phase, embedding, toolVersion string) Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entryLocked(tool, root)
	if e.Phase == PhaseIndexing {
		return *e
	}
	e.Phase = phase
	e.Embedding, e.ToolVersion = embedding, toolVersion
	if phase != PhaseFailed {
		e.Error = ""
	}
	e.UpdatedAt = m.now()
	return *e
}

// Begin claims an index for a run and moves it to PhaseIndexing.
//
// It returns false when a run is already in flight for this (tool, root) — the
// process-wide single-run lock, which matters because two workspace runtimes
// pointed at the same repository share one daemon and one store. The caller must
// not start its CLI when this returns false.
//
// A granted claim is identified by the returned Entry.Run token; the run must
// hand that token back to Succeed or Fail to close the entry.
func (m *Manager) Begin(tool, root, action string) (Entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entryLocked(tool, root)
	if e.Phase == PhaseIndexing {
		return *e, false
	}
	m.lastRun++
	e.Phase, e.Action, e.Run = PhaseIndexing, action, m.lastRun
	e.Error = ""
	e.StartedAt = m.now()
	e.UpdatedAt = e.StartedAt
	return *e, true
}

// Succeed closes a run that finished, recording what the new store was built
// with. Only the run that Begin claimed may close an entry: run must be the
// Entry.Run token Begin returned, and the entry must still be in PhaseIndexing
// under that token. Anything else — a never-claimed key, a late goroutine from
// a superseded run, a report after the run already closed — is rejected with
// ErrStaleClaim and leaves the entry unchanged.
func (m *Manager) Succeed(tool, root string, run uint64, embedding, toolVersion string) (Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, err := m.claimedLocked(tool, root, run)
	if err != nil {
		return Entry{}, err
	}
	e.Phase = PhaseReady
	e.Embedding, e.ToolVersion = embedding, toolVersion
	e.Error = ""
	e.UpdatedAt = m.now()
	return *e, nil
}

// Fail closes a run that failed. It enforces the same claim check as Succeed.
//
// The reason is REQUIRED: an entry that landed in PhaseFailed with no
// explanation is the silent failure this package exists to prevent, so an empty
// reason is replaced with an explicit placeholder rather than left blank.
func (m *Manager) Fail(tool, root string, run uint64, reason string) (Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, err := m.claimedLocked(tool, root, run)
	if err != nil {
		return Entry{}, err
	}
	e.Phase = PhaseFailed
	if reason = strings.TrimSpace(reason); reason == "" {
		reason = "index run failed without a reported reason"
	}
	e.Error = reason
	e.UpdatedAt = m.now()
	return *e, nil
}

// claimedLocked returns the entry for a key only when run holds its current
// claim. It never creates an entry: a close for a key the ledger has no record
// of is by definition unclaimed. Caller holds mu.
func (m *Manager) claimedLocked(tool, root string, run uint64) (*Entry, error) {
	k := NewKey(tool, root)
	e, ok := m.entries[k]
	switch {
	case !ok:
		return nil, fmt.Errorf("%w: %s %s has no ledger entry", ErrStaleClaim, k.Tool, k.Root)
	case run == 0 || e.Run != run:
		return nil, fmt.Errorf("%w: %s %s is claimed by run %d, not %d", ErrStaleClaim, k.Tool, k.Root, e.Run, run)
	case e.Phase != PhaseIndexing:
		return nil, fmt.Errorf("%w: %s %s run %d already closed as %s", ErrStaleClaim, k.Tool, k.Root, run, e.Phase)
	}
	return e, nil
}

// Forget removes an entry from the ledger. This is bookkeeping only — it does
// NOT touch the index on disk. Deleting a store is Drop's job.
func (m *Manager) Forget(tool, root string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, NewKey(tool, root))
}

// entryLocked returns the entry for a key, creating it in PhaseMissing on first
// touch. Caller holds mu.
func (m *Manager) entryLocked(tool, root string) *Entry {
	k := NewKey(tool, root)
	e, ok := m.entries[k]
	if !ok {
		e = &Entry{Tool: k.Tool, Root: filepath.Clean(strings.TrimSpace(root)), Phase: PhaseMissing}
		m.entries[k] = e
	}
	return e
}
