package decider

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Role says why a ledger record exists.
type Role string

const (
	// RolePrimary (empty) is the answer the authority used (or would use).
	RolePrimary Role = ""
	// RoleChallenger is a challenger model's answer to the same question,
	// kept only to measure agreement with the primary answer.
	RoleChallenger Role = "challenger"
)

// Record is one decision as the ledger keeps it: enough to measure agreement,
// latency and spend per authority and per model, and deliberately nothing of
// the state that was judged (the ledger is a metrics log, not a transcript).
type Record struct {
	DebugID string `json:"debugId,omitempty"`
	// At is when the decision finished, in unix milliseconds.
	At        int64  `json:"at"`
	Authority string `json:"authority"`
	Mode      Mode   `json:"mode"`
	Role      Role   `json:"role,omitempty"`
	// Instance is the decision model (settings entry) that answered.
	Instance string `json:"instance,omitempty"`
	// Model is the requested service model id.
	Model string `json:"model,omitempty"`
	// Fallback is set when the primary model failed and the fallback answered.
	Fallback    bool    `json:"fallback,omitempty"`
	LatencyMs   int64   `json:"latencyMs,omitempty"`
	InputTokens int     `json:"inputTokens,omitempty"`
	CostUSD     float64 `json:"costUsd,omitempty"`
	// Outcome is the verdict in the authority's own vocabulary ("stalled",
	// "ask", "arm2", "pass"); empty when the call failed.
	Outcome string `json:"outcome,omitempty"`
	// Strength is the probability or confidence behind Outcome.
	Strength float64 `json:"strength,omitempty"`
	// Baseline is the verdict Outcome is compared with: the authority's
	// existing logic in shadow mode, the primary model's verdict on a
	// challenger record.
	Baseline string `json:"baseline,omitempty"`
	// Applied is set when the verdict drove behaviour.
	Applied bool `json:"applied,omitempty"`
	// Error is a short error class ("timeout", "http_401"), never raw text.
	Error string `json:"error,omitempty"`
	// Ref correlates the record with a session, flow run or trajectory id.
	Ref string `json:"ref,omitempty"`
}

// Compared reports whether both verdicts exist, so the record counts toward
// agreement.
func (r Record) Compared() bool {
	return r.Error == "" && r.Outcome != "" && r.Baseline != ""
}

// Agrees reports whether a compared record's two verdicts match.
func (r Record) Agrees() bool {
	return r.Compared() && r.Outcome == r.Baseline
}

// AuthorityStats aggregates an authority's records over a window.
type AuthorityStats struct {
	Authority string `json:"authority"`
	// Primary answers.
	Calls     int     `json:"calls"`
	Errors    int     `json:"errors"`
	Shadow    int     `json:"shadow"`
	Compared  int     `json:"compared"`
	Agreed    int     `json:"agreed"`
	Applied   int     `json:"applied"`
	Fallbacks int     `json:"fallbacks"`
	P50Ms     int64   `json:"p50Ms"`
	P95Ms     int64   `json:"p95Ms"`
	CostUSD   float64 `json:"costUsd"`
	LastAt    int64   `json:"lastAt,omitempty"`
	// Challenger comparisons.
	ChallengerCalls    int     `json:"challengerCalls"`
	ChallengerErrors   int     `json:"challengerErrors"`
	ChallengerCompared int     `json:"challengerCompared"`
	ChallengerAgreed   int     `json:"challengerAgreed"`
	ChallengerP50Ms    int64   `json:"challengerP50Ms"`
	ChallengerCostUSD  float64 `json:"challengerCostUsd"`
	// Challenger is the model of the most recent challenger record.
	Challenger string `json:"challenger,omitempty"`
}

// ModelStats aggregates one decision model's calls over a window, whatever
// authority or role they served.
type ModelStats struct {
	Instance string  `json:"instance"`
	Calls    int     `json:"calls"`
	Errors   int     `json:"errors"`
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
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if rec.Authority == "" {
			// Lines written before authorities were named so called them sites.
			var legacy struct {
				Site string `json:"site"`
			}
			_ = json.Unmarshal(sc.Bytes(), &legacy)
			rec.Authority = legacy.Site
		}
		if rec.Authority != "" {
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
	return appendJSONL(l.path, l.maxBytes, line, func() error {
		// Keep ledger rotation best-effort even when the destination is blocked.
		_ = os.Rename(l.path, rotatedLedgerPath(l.path))
		return nil
	})
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

// since returns the in-memory records at or after t, oldest first.
func (l *Ledger) since(t time.Time) []Record {
	cutoff := t.UnixMilli()
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Record, 0, len(l.ring))
	for _, r := range l.ring {
		if r.At >= cutoff {
			out = append(out, r)
		}
	}
	return out
}

// Stats aggregates the records at or after since, one entry per authority that
// has any, sorted by authority id.
func (l *Ledger) Stats(since time.Time) []AuthorityStats {
	if l == nil {
		return nil
	}
	byAuth := map[string]*AuthorityStats{}
	latencies := map[string][]int64{}
	challengerLatencies := map[string][]int64{}
	for _, r := range l.since(since) {
		st := byAuth[r.Authority]
		if st == nil {
			st = &AuthorityStats{Authority: r.Authority}
			byAuth[r.Authority] = st
		}
		if r.Role == RoleChallenger {
			st.ChallengerCalls++
			st.ChallengerCostUSD += r.CostUSD
			st.Challenger = r.Instance
			if r.Error != "" {
				st.ChallengerErrors++
				continue
			}
			if r.Compared() {
				st.ChallengerCompared++
				if r.Agrees() {
					st.ChallengerAgreed++
				}
			}
			if r.LatencyMs > 0 {
				challengerLatencies[r.Authority] = append(challengerLatencies[r.Authority], r.LatencyMs)
			}
			continue
		}
		st.Calls++
		st.CostUSD += r.CostUSD
		st.LastAt = max(st.LastAt, r.At)
		if r.Error != "" {
			st.Errors++
			continue
		}
		if r.Fallback {
			st.Fallbacks++
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
			latencies[r.Authority] = append(latencies[r.Authority], r.LatencyMs)
		}
	}
	out := make([]AuthorityStats, 0, len(byAuth))
	for id, st := range byAuth {
		st.P50Ms = percentile(latencies[id], 0.50)
		st.P95Ms = percentile(latencies[id], 0.95)
		st.ChallengerP50Ms = percentile(challengerLatencies[id], 0.50)
		out = append(out, *st)
	}
	slices.SortFunc(out, func(a, b AuthorityStats) int {
		return cmp.Compare(a.Authority, b.Authority)
	})
	return out
}

// ModelStats aggregates the records at or after since per decision model,
// sorted by model id. Records without a model (a call that never reached one)
// are left out.
func (l *Ledger) ModelStats(since time.Time) []ModelStats {
	if l == nil {
		return nil
	}
	byModel := map[string]*ModelStats{}
	latencies := map[string][]int64{}
	for _, r := range l.since(since) {
		if r.Instance == "" {
			continue
		}
		st := byModel[r.Instance]
		if st == nil {
			st = &ModelStats{Instance: r.Instance}
			byModel[r.Instance] = st
		}
		st.Calls++
		st.CostUSD += r.CostUSD
		st.LastAt = max(st.LastAt, r.At)
		if r.Error != "" {
			st.Errors++
			continue
		}
		if r.LatencyMs > 0 {
			latencies[r.Instance] = append(latencies[r.Instance], r.LatencyMs)
		}
	}
	out := make([]ModelStats, 0, len(byModel))
	for id, st := range byModel {
		st.P50Ms = percentile(latencies[id], 0.50)
		st.P95Ms = percentile(latencies[id], 0.95)
		out = append(out, *st)
	}
	slices.SortFunc(out, func(a, b ModelStats) int {
		return cmp.Compare(a.Instance, b.Instance)
	})
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
