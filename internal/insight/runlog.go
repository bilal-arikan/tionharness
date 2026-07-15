package insight

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// Scan-run observability. Normal scans no longer create sessions (they were noise
// in the chat list), but a scan should still be auditable: when it ran, how long
// it took, what it covered and produced. This is a lightweight append-only log,
// NOT a session — read via GET /api/insight/runs.

var runsRelPath = filepath.Join("insight", "runs.jsonl")

// RunRecord is one scan run's rollup, stamped with wall-clock time + duration by
// the caller (the runtime, which owns the real clock).
type RunRecord struct {
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
// demand). A blank root is a no-op.
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
	defer f.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// ReadRuns returns the most recent run records NEWEST-first, capped at limit
// (<=0 means a default of 50). A missing file yields an empty slice.
func ReadRuns(root string, limit int) ([]RunRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	path := filepath.Join(root, runsRelPath)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return []RunRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var all []RunRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec RunRecord
		if json.Unmarshal(line, &rec) == nil {
			all = append(all, rec)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// Newest-first, capped.
	out := make([]RunRecord, 0, limit)
	for i := len(all) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, all[i])
	}
	return out, nil
}
