package decider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"
)

// Record is one decision as the ledger keeps it: enough to measure agreement,
// latency and spend per site, and deliberately nothing of the state that was
// judged (the ledger is a metrics log, not a transcript).
type Record struct {
	// At is when the decision finished, in unix milliseconds.
	At   int64  `json:"at"`
	Site string `json:"site"`
	Mode Mode   `json:"mode"`
	// Model is the requested model id.
	Model       string  `json:"model,omitempty"`
	LatencyMs   int64   `json:"latencyMs,omitempty"`
	InputTokens int     `json:"inputTokens,omitempty"`
	CostUSD     float64 `json:"costUsd,omitempty"`
	// Outcome is the decider's verdict in the site's own vocabulary ("stalled",
	// "ask", "arm 2", "pass"); empty when the call failed.
	Outcome string `json:"outcome,omitempty"`
	// Strength is the probability or confidence behind Outcome.
	Strength float64 `json:"strength,omitempty"`
	// Baseline is the verdict of the site's existing logic, in the same
	// vocabulary as Outcome. Set in shadow mode (and wherever both exist).
	Baseline string `json:"baseline,omitempty"`
	// Applied is set when the decider's verdict drove behaviour.
	Applied bool `json:"applied,omitempty"`
	// Error is a short error class ("timeout", "http_401"), never raw text.
	Error string `json:"error,omitempty"`
	// Ref correlates the record with a session or flow run id.
	Ref string `json:"ref,omitempty"`
}

// Compared reports whether both the decider and the site's existing logic
// produced a verdict, so the record counts toward agreement.
func (r Record) Compared() bool {
	return r.Error == "" && r.Outcome != "" && r.Baseline != ""
}

// Agrees reports whether a compared record's two verdicts match.
func (r Record) Agrees() bool {
	return r.Compared() && r.Outcome == r.Baseline
}

// SiteStats aggregates a site's records over a window.
type SiteStats struct {
	Site     string  `json:"site"`
	Calls    int     `json:"calls"`
	Errors   int     `json:"errors"`
	Shadow   int     `json:"shadow"`
	Compared int     `json:"compared"`
	Agreed   int     `json:"agreed"`
	Applied  int     `json:"applied"`
	P50Ms    int64   `json:"p50Ms"`
	P95Ms    int64   `json:"p95Ms"`
	CostUSD  float64 `json:"costUsd"`
	LastAt   int64   `json:"lastAt,omitempty"`
}

// Ledger is an append-only JSONL log of decisions plus an in-memory ring of the
// most recent records for the settings screen. Writes are best-effort: a disk
// failure is logged and the decision proceeds.
type Ledger struct {
	mu       sync.Mutex
	path     string
	ring     []Record // oldest first
	capacity int
	maxBytes int64
	logger   *slog.Logger
}

const (
	ledgerFileName = "ledger.jsonl"
	ledgerCapacity = 5000
	// ledgerMaxBytes rotates the file: the current one becomes ledger.1.jsonl
	// (replacing the previous rotation), so disk use stays bounded at ~2x this.
	ledgerMaxBytes = 4 << 20
)

// OpenLedger opens (or creates) the ledger in dir and loads its most recent
// records into memory. dir == "" gives an in-memory-only ledger (tests).
func OpenLedger(dir string, logger *slog.Logger) *Ledger {
	if logger == nil {
		logger = slog.Default()
	}
	l := &Ledger{capacity: ledgerCapacity, maxBytes: ledgerMaxBytes, logger: logger}
	if dir == "" {
		return l
	}
	l.path = filepath.Join(dir, ledgerFileName)
	l.ring = readLedgerTail(l.path, l.capacity)
	return l
}

func readLedgerTail(path string, capacity int) []Record {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []Record
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var rec Record
		if json.Unmarshal(sc.Bytes(), &rec) == nil && rec.Site != "" {
			out = append(out, rec)
		}
	}
	if len(out) > capacity {
		out = out[len(out)-capacity:]
	}
	return out
}

// Append records one decision.
func (l *Ledger) Append(rec Record) {
	if l == nil {
		return
	}
	if rec.At == 0 {
		rec.At = time.Now().UnixMilli()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ring = append(l.ring, rec)
	if len(l.ring) > l.capacity {
		l.ring = slices.Delete(l.ring, 0, len(l.ring)-l.capacity)
	}
	if l.path == "" {
		return
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	if err := l.appendLine(append(line, '\n')); err != nil {
		l.logger.Warn("decider ledger write failed", "path", l.path, "error", err)
	}
}

func (l *Ledger) appendLine(line []byte) error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	if fi, err := os.Stat(l.path); err == nil && fi.Size()+int64(len(line)) > l.maxBytes {
		_ = os.Rename(l.path, rotatedLedgerPath(l.path))
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

func rotatedLedgerPath(path string) string {
	return filepath.Join(filepath.Dir(path), "ledger.1.jsonl")
}

// Recent returns up to n records, newest first.
func (l *Ledger) Recent(n int) []Record {
	if l == nil || n <= 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n > len(l.ring) {
		n = len(l.ring)
	}
	out := make([]Record, 0, n)
	for i := len(l.ring) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, l.ring[i])
	}
	return out
}

// Stats aggregates the in-memory records at or after since, one entry per
// site that has any, sorted by site id.
func (l *Ledger) Stats(since time.Time) []SiteStats {
	if l == nil {
		return nil
	}
	cutoff := since.UnixMilli()
	l.mu.Lock()
	recs := make([]Record, 0, len(l.ring))
	for _, r := range l.ring {
		if r.At >= cutoff {
			recs = append(recs, r)
		}
	}
	l.mu.Unlock()

	bySite := map[string]*SiteStats{}
	latencies := map[string][]int64{}
	for _, r := range recs {
		st := bySite[r.Site]
		if st == nil {
			st = &SiteStats{Site: r.Site}
			bySite[r.Site] = st
		}
		st.Calls++
		st.CostUSD += r.CostUSD
		st.LastAt = max(st.LastAt, r.At)
		if r.Error != "" {
			st.Errors++
			continue
		}
		if r.Mode == ModeShadow {
			st.Shadow++
		}
		if r.Compared() {
			st.Compared++
			if r.Agrees() {
				st.Agreed++
			}
		}
		if r.Applied {
			st.Applied++
		}
		if r.LatencyMs > 0 {
			latencies[r.Site] = append(latencies[r.Site], r.LatencyMs)
		}
	}
	out := make([]SiteStats, 0, len(bySite))
	for site, st := range bySite {
		st.P50Ms = percentile(latencies[site], 0.50)
		st.P95Ms = percentile(latencies[site], 0.95)
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Site < out[j].Site })
	return out
}

// percentile returns the nearest-rank percentile of vals (0 when empty).
func percentile(vals []int64, p float64) int64 {
	if len(vals) == 0 {
		return 0
	}
	s := slices.Clone(vals)
	slices.Sort(s)
	idx := int(float64(len(s))*p+0.999999) - 1
	idx = min(max(idx, 0), len(s)-1)
	return s[idx]
}
