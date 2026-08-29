package insight

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Ledger is the incremental scan state: per (lensId, sessionId) it records what
// was last scanned, so a session is never re-scanned unless it actually changed
// (_Docs/60 §3). It is deliberately decoupled from db — callers pass the two
// change signals (updatedAt + a cheap content fingerprint) as primitives, which
// keeps the ledger trivially unit-testable.
//
// Storage is an append-only JSONL at <store>/insight/ledger.jsonl: Record appends
// one line (O(1), crash-safe), and load keeps the LAST entry per key. Compact
// rewrites the file to drop superseded lines when it grows.
type Ledger struct {
	mu   sync.Mutex
	path string
	// entries: lensID -> sessionID -> latest entry.
	entries map[string]map[string]LedgerEntry
}

// LedgerEntry is one scan record. SeenUpdatedAt is the session's UpdatedAt at
// scan time (primary change signal); SeenFingerprint is the content signature at
// scan time (so a metadata-only UpdatedAt bump does not force a re-scan).
// SeenLensVersion is Lens.Version() at scan time — the second change signal, so
// improving a lens re-scans sessions whose content never changed.
type LedgerEntry struct {
	LensID          string `json:"lensId"`
	SessionID       string `json:"sessionId"`
	SeenUpdatedAt   int64  `json:"seenUpdatedAt"`
	SeenFingerprint string `json:"seenFingerprint"`
	SeenLensVersion string `json:"seenLensVersion,omitempty"`
	ScannedAt       int64  `json:"scannedAt"`
	FindingCount    int    `json:"findingCount"`
	Status          string `json:"status"` // clean | error
}

var ledgerRelPath = filepath.Join("insight", "ledger.jsonl")

// Fingerprint is a cheap, header-only content signature of a session: message
// count plus the compaction watermark. It changes when a turn is appended or the
// conversation is compacted, but NOT when only metadata (title/tags/pin) is
// edited — which is exactly what lets NeedsScan skip a metadata-only UpdatedAt
// bump. No session.jsonl read is required. Can later be strengthened with the
// last message id without changing the ledger contract.
func Fingerprint(msgCount, summaryMsgCount int) string {
	return fmt.Sprintf("%d:%d", msgCount, summaryMsgCount)
}

// OpenLedger loads the ledger rooted at the store root (db.Root()). A missing
// file yields an empty ledger.
func OpenLedger(root string) (*Ledger, error) {
	l := &Ledger{
		path:    filepath.Join(root, ledgerRelPath),
		entries: map[string]map[string]LedgerEntry{},
	}
	f, err := os.Open(l.path)
	if os.IsNotExist(err) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		var e LedgerEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.LensID != "" && e.SessionID != "" {
			l.put(e) // last line wins
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return l, nil
}

// NeedsScan reports whether (lensID, session) must be scanned. It is pure (no
// mutation). There are TWO change signals (_Docs/60 §3):
//   - no ledger entry        -> scan (first time / new lens backfill)
//   - fingerprint changed    -> scan (a turn was appended / conversation compacted)
//   - lens version changed   -> scan (the lens prompt/prefilter/scope/model was
//     edited, so the same content can now yield a different answer)
//   - both unchanged         -> skip (a bare UpdatedAt bump is metadata-only)
//
// UpdatedAt is deliberately NOT a trigger: it has second granularity (a fast
// content edit within the same second would be missed) and it also bumps on
// metadata-only edits (which must be skipped). The fingerprint handles both. The
// recorded SeenUpdatedAt is kept for observability only.
//
// Backward compatibility: entries written before the version field carry an EMPTY
// SeenLensVersion. Those are GRANDFATHERED — treated as up to date, not re-scanned.
// The alternative (re-scan once) would invalidate every existing ledger row on the
// first scan after upgrading and re-surface findings for issues already fixed —
// exactly the blast radius Reset(deep) warns about. A user who does want the old
// rows re-analyzed still has that escape hatch.
func (l *Ledger) NeedsScan(lensID, sessionID string, updatedAt int64, fingerprint, lensVersion string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	byLens, ok := l.entries[lensID]
	if !ok {
		return true
	}
	rec, ok := byLens[sessionID]
	if !ok {
		return true
	}
	if rec.SeenFingerprint != fingerprint {
		return true
	}
	return rec.SeenLensVersion != "" && rec.SeenLensVersion != lensVersion
}

// Record upserts an entry in memory and appends it to the ledger file.
func (l *Ledger) Record(e LedgerEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.put(e)
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return enc.Encode(e)
}

// Compact rewrites the file to the latest entry per key, dropping superseded
// append lines. Call periodically (e.g. when the file grows large).
func (l *Ledger) Compact() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, byLens := range l.entries {
		for _, e := range byLens {
			if err := enc.Encode(e); err != nil {
				_ = f.Close()
				return err
			}
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// put stores e as the latest entry for its key (caller holds l.mu).
func (l *Ledger) put(e LedgerEntry) {
	if l.entries[e.LensID] == nil {
		l.entries[e.LensID] = map[string]LedgerEntry{}
	}
	l.entries[e.LensID][e.SessionID] = e
}
