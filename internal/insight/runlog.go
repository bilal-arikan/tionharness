package insight

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// Scan-run observability. Every scan now ALSO opens a read-only session
// (Kind=="insight", SourceID==RunRecord.ID) so the run reads like any other
// transcript; this append-only log stays alongside it as the compact, queryable
// rollup — when it ran, how long it took, what it covered and produced — read via
// GET /api/insight/runs. The insight kind is hidden from the default sessions
// view, so the old "noise in the chat list" problem does not come back.

var runsRelPath = filepath.Join("insight", "runs.jsonl")

// maxRunRecords bounds the run log so repeated scans can't grow it forever. When
// an append pushes the file past this, the oldest records are dropped (newest
// kept). Generous — a nightly scan for a year is ~365 lines.
const maxRunRecords = 1000

// RunRecord is one scan run's rollup, stamped with wall-clock time + duration by
// the caller (the runtime, which owns the real clock).
type RunRecord struct {
	// ID is this run's stable identity, also carried as SourceID on the run's
	// read-only session, so the two records point at each other. Empty on records
	// written before the session pairing existed (backward compatible).
	ID string `json:"id,omitempty"`
	// SessionID is the run's read-only transcript session (Kind=="insight").
	// Empty when the session could not be created, or on legacy records.
	SessionID   string   `json:"sessionId,omitempty"`
	At          int64    `json:"at"`         // unix seconds when the run finished
	DurationMs  int64    `json:"durationMs"` // wall-clock duration
	Trigger     string   `json:"trigger"`    // "manual" | "auto" | "agent" | ""
	LensIDs     []string `json:"lensIds,omitempty"`
	Sessions    int      `json:"sessions"`
	Analyzed    int      `json:"analyzed"`
	Skipped     int      `json:"skipped"`
	Prefiltered int      `json:"prefiltered"`
	Findings    int      `json:"findings"`
	Errors      int      `json:"errors"`
}

// AppendRun appends one run record to <root>/insight/runs.jsonl (created on
// demand), then trims the file to the newest maxRunRecords so repeated scans
// can't grow it without bound. A blank root is a no-op.
func AppendRun(root string, rec RunRecord) error {
	if root == "" {
		return nil
	}
	path := filepath.Join(root, runsRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	b, err := json.Marshal(rec)
	if err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return trimRuns(path)
}

// trimRuns rewrites the run log to the newest maxRunRecords when it exceeds that,
// so the append-only file stays bounded. A cheap no-op while under the cap.
func trimRuns(path string) error {
	recs, err := readAllRuns(path) // oldest-first
	if err != nil || len(recs) <= maxRunRecords {
		return err
	}
	recs = recs[len(recs)-maxRunRecords:]
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for i := range recs {
		if err := enc.Encode(recs[i]); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readAllRuns reads every record in file order (oldest-first). Missing file → nil.
func readAllRuns(path string) ([]RunRecord, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var all []RunRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var rec RunRecord
		if json.Unmarshal(sc.Bytes(), &rec) == nil {
			all = append(all, rec)
		}
	}
	return all, sc.Err()
}

// ReadRuns returns the most recent run records NEWEST-first, capped at limit
// (<=0 means a default of 50). A missing file yields an empty slice.
func ReadRuns(root string, limit int) ([]RunRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	all, err := readAllRuns(filepath.Join(root, runsRelPath)) // oldest-first
	if err != nil {
		return nil, err
	}
	// Newest-first, capped.
	out := make([]RunRecord, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i])
	}
	return out, nil
}
