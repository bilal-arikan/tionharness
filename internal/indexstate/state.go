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
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
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
	StartedAt time.Time `json:"startedAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

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
// the host filesystem does — case-insensitively on Windows, where the same
// repository reached as C:\Repo and c:\repo must not mint two ledger entries and
// two concurrent index runs.
func NewKey(tool, root string) Key {
	root = filepath.ToSlash(filepath.Clean(strings.TrimSpace(root)))
	if runtime.GOOS == "windows" {
		root = strings.ToLower(root)
	}
	return Key{Tool: strings.TrimSpace(tool), Root: root}
}

// Manager is the ledger. The zero value is not usable; call New.
type Manager struct {
	mu      sync.Mutex
	entries map[Key]*Entry
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
func (m *Manager) Begin(tool, root, action string) (Entry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entryLocked(tool, root)
	if e.Phase == PhaseIndexing {
		return *e, false
	}
	e.Phase, e.Action = PhaseIndexing, action
	e.Error = ""
	e.StartedAt = m.now()
	e.UpdatedAt = e.StartedAt
	return *e, true
}

// Succeed closes a run that finished, recording what the new store was built
// with. Only a run that Begin claimed may close an entry, so a late goroutine
// from a superseded run cannot mark a newer one ready.
func (m *Manager) Succeed(tool, root, embedding, toolVersion string) Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entryLocked(tool, root)
	e.Phase = PhaseReady
	e.Embedding, e.ToolVersion = embedding, toolVersion
	e.Error = ""
	e.UpdatedAt = m.now()
	return *e
}

// Fail closes a run that failed. The reason is REQUIRED: an entry that landed in
// PhaseFailed with no explanation is the silent failure this package exists to
// prevent, so an empty reason is replaced with an explicit placeholder rather
// than left blank.
func (m *Manager) Fail(tool, root, reason string) Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.entryLocked(tool, root)
	e.Phase = PhaseFailed
	if reason = strings.TrimSpace(reason); reason == "" {
		reason = "index run failed without a reported reason"
	}
	e.Error = reason
	e.UpdatedAt = m.now()
	return *e
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
