package insight

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// Analyzer turns a prepared per-session analysis request into findings. The
// production implementation calls a cheap model (call_llm / claude-cli) with the
// lens prompt + the session slice; unit tests supply a deterministic fake. This
// is the ONLY LLM seam in the scanner, so the rest of the pipeline (enumeration,
// incremental skipping, prefilter, dedupe) is fully testable without a model.
type Analyzer interface {
	Analyze(ctx context.Context, req AnalysisRequest) ([]Finding, error)
}

// AnalysisRequest is the prepared input for one (lens, session) analysis. The
// caller has already applied the prefilter and built a compact Transcript slice
// (errors/tools/debug), so the analyzer only has to reason and emit findings.
type AnalysisRequest struct {
	Lens       Lens
	SessionID  string
	Transcript string
}

// ScanScope selects what a scan covers.
type ScanScope struct {
	LensIDs         []string // empty = all enabled lenses; explicit ids run even if disabled
	AgentID         string   // "" = all agents
	IncludeArchived bool     // include archived sessions (default: skip them)
	MaxSessions     int      // 0 = no cap (budget guardrail)
	MaxAnalyzed     int      // 0 = no cap; hard ceiling on analyzer (LLM) calls this run (cost budget)
	Concurrency     int      // 0 = default; how many analyzer calls run in parallel
	SinceUnix       int64    // 0 = no age limit; skip sessions last active before this unix time
}

// defaultScanConcurrency bounds how many analyzer (LLM) calls run at once when the
// scope leaves it unset. Modest so a scan doesn't hammer the provider, but enough
// to cut a large first scan from many minutes to a fraction.
const defaultScanConcurrency = 4

// ScanResult is the rollup of one scan run.
type ScanResult struct {
	Sessions    int      `json:"sessions"`    // sessions considered
	Analyzed    int      `json:"analyzed"`    // (lens,session) pairs sent to the analyzer
	Skipped     int      `json:"skipped"`     // pairs skipped by the ledger (unchanged)
	Prefiltered int      `json:"prefiltered"` // pairs dropped by the prefilter (no LLM)
	Findings    int      `json:"findings"`    // findings emitted (post-dedupe upserts)
	Errors      []string `json:"errors,omitempty"`

	// Produced holds the findings emitted THIS run (post-metadata, pre-persist),
	// exposed so a caller can route fresh findings onward — e.g. promoting
	// lessons-mining findings into the lessons store — without re-listing the
	// store (which would re-feed already-known findings on every scan). Not part
	// of the API rollup shape; consumed in-process only.
	Produced []Finding `json:"-"`
}

// Scanner runs the retrospective pipeline over a workspace's sessions.
type Scanner struct {
	db       *db.DB
	reg      *Registry
	ledger   *Ledger
	findings *FindingStore
	analyzer Analyzer
	now      func() int64
	sliceCap int
}

// NewScanner wires the pipeline. now is injectable for deterministic tests; when
// nil it defaults to wall-clock unix seconds.
func NewScanner(database *db.DB, reg *Registry, ledger *Ledger, findings *FindingStore, analyzer Analyzer, now func() int64) *Scanner {
	if now == nil {
		now = func() int64 { return time.Now().Unix() }
	}
	return &Scanner{
		db: database, reg: reg, ledger: ledger, findings: findings,
		analyzer: analyzer, now: now, sliceCap: 8000,
	}
}

// analysisTask is one prepared (lens, session) unit whose slow LLM analysis can
// run concurrently with others; the transcript is built serially in phase 1.
type analysisTask struct {
	lens       Lens
	sess       db.Session
	fp         string
	transcript string
}

// analysisOutcome pairs a task with its analyzer result, applied serially in
// phase 3 so all store/ledger/counter mutation stays single-threaded.
type analysisOutcome struct {
	task  analysisTask
	found []Finding
	err   error
}

// Scan runs one pass in three phases so the slow part parallelizes safely:
//
//  1. SERIAL enumerate: skip (lens,session) pairs the ledger already covers,
//     prefilter the rest with cheap signals (recording clean-prefiltered pairs),
//     and build the analysis task list — bounded by MaxSessions and MaxAnalyzed.
//  2. CONCURRENT analyze: run the analyzer (one LLM call per task) through a
//     bounded worker pool. This is the only parallel phase; nothing here mutates
//     shared state beyond the per-index outcome slot.
//  3. SERIAL apply: dedupe findings into the store, record each pair in the
//     ledger, and tally the rollup. An analyzer error leaves that pair
//     UN-recorded so the next scan retries it.
func (s *Scanner) Scan(ctx context.Context, scope ScanScope) (ScanResult, error) {
	var res ScanResult
	lenses := s.selectLenses(scope.LensIDs)
	if len(lenses) == 0 {
		return res, nil
	}
	sessions, err := s.db.ListSessions(ctx, scope.AgentID)
	if err != nil {
		return res, err
	}

	// ---- Phase 1: enumerate + prefilter (serial) ----
	var tasks []analysisTask
enumerate:
	for _, sess := range sessions {
		if scope.MaxSessions > 0 && res.Sessions >= scope.MaxSessions {
			break
		}
		if !scope.IncludeArchived && sess.State == "archived" {
			continue
		}
		// Age filter: skip sessions whose last activity predates the cutoff, so a
		// scan can focus on recent history instead of re-surfacing findings from
		// long-old sessions (whose issues may be stale or already fixed).
		if scope.SinceUnix > 0 && sess.UpdatedAt > 0 && sess.UpdatedAt < scope.SinceUnix {
			continue
		}
		res.Sessions++
		fp := Fingerprint(sess.MessageCount, sess.SummaryMsgCount)

		var due []Lens
		for _, l := range lenses {
			if s.ledger.NeedsScan(l.ID, sess.ID, sess.UpdatedAt, fp) {
				due = append(due, l)
			} else {
				res.Skipped++
			}
		}
		if len(due) == 0 {
			continue
		}

		msgs, mErr := s.db.ListMessages(ctx, sess.ID)
		if mErr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: list messages: %v", sess.ID, mErr))
			continue
		}
		events, _ := s.db.ReadDebugEvents(ctx, sess.ID, "", 0) // best-effort: debug journal may be off
		sig := extractSignals(msgs, events)

		for _, l := range due {
			if !l.Prefilter.Match(sig) {
				res.Prefiltered++
				if rErr := s.record(l.ID, sess, fp, 0, "clean"); rErr != nil {
					res.Errors = append(res.Errors, rErr.Error())
				}
				continue
			}
			// Cost budget: stop QUEUEING analyzer calls once the cap is hit. Pairs not
			// queued stay un-recorded → picked up by the next scan.
			if scope.MaxAnalyzed > 0 && len(tasks) >= scope.MaxAnalyzed {
				break enumerate
			}
			tasks = append(tasks, analysisTask{
				lens: l, sess: sess, fp: fp, transcript: s.buildSlice(sess, msgs, events),
			})
		}
	}

	// ---- Phase 2: analyze (concurrent, bounded) ----
	outcomes := make([]analysisOutcome, len(tasks))
	conc := scope.Concurrency
	if conc <= 0 {
		conc = defaultScanConcurrency
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, t analysisTask) {
			defer wg.Done()
			defer func() { <-sem }()
			found, aErr := s.analyzer.Analyze(ctx, AnalysisRequest{Lens: t.lens, SessionID: t.sess.ID, Transcript: t.transcript})
			outcomes[i] = analysisOutcome{task: t, found: found, err: aErr}
		}(i, t)
	}
	wg.Wait()

	// ---- Phase 3: apply (serial) ----
	for _, oc := range outcomes {
		t := oc.task
		if oc.err != nil {
			// Leave this pair un-recorded → retried on the next scan.
			res.Errors = append(res.Errors, fmt.Sprintf("%s/%s: analyze: %v", t.lens.ID, t.sess.ID, oc.err))
			continue
		}
		now := s.now()
		for _, f := range oc.found {
			f.LensID = t.lens.ID
			f.Channel = t.lens.Channel
			if f.FirstSeen == 0 {
				f.FirstSeen = now
			}
			f.LastSeen = now
			if len(f.EvidenceSessionIDs) == 0 {
				f.EvidenceSessionIDs = []string{t.sess.ID}
			}
			if _, uErr := s.findings.Upsert(f); uErr != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: upsert finding: %v", t.lens.ID, uErr))
				continue
			}
			res.Findings++
			res.Produced = append(res.Produced, f)
		}
		if rErr := s.record(t.lens.ID, t.sess, t.fp, len(oc.found), "clean"); rErr != nil {
			res.Errors = append(res.Errors, rErr.Error())
		}
		res.Analyzed++
	}
	return res, nil
}

// selectLenses resolves the scope's lens ids. Empty selects all ENABLED lenses;
// an explicit id runs even if the lens is disabled (the user asked for it).
func (s *Scanner) selectLenses(ids []string) []Lens {
	if len(ids) == 0 {
		return s.reg.Enabled()
	}
	out := make([]Lens, 0, len(ids))
	for _, id := range ids {
		if l, ok := s.reg.Get(id); ok {
			out = append(out, l)
		}
	}
	return out
}

// record upserts a ledger entry for a scanned (lens,session) pair.
func (s *Scanner) record(lensID string, sess db.Session, fp string, findingCount int, status string) error {
	return s.ledger.Record(LedgerEntry{
		LensID:          lensID,
		SessionID:       sess.ID,
		SeenUpdatedAt:   sess.UpdatedAt,
		SeenFingerprint: fp,
		ScannedAt:       s.now(),
		FindingCount:    findingCount,
		Status:          status,
	})
}

// rawStep is the minimal shape of an agent.TurnStep needed for signal extraction.
// Decoding into this local type keeps the insight package decoupled from the
// heavy agent package (and avoids any import cycle) — only kind/reason/isError
// and a little text are read.
type rawStep struct {
	Kind    string `json:"kind"`
	Reason  string `json:"reason,omitempty"`
	IsError bool   `json:"isError,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Text    string `json:"text,omitempty"`
	Output  string `json:"output,omitempty"`
}

func parseSteps(raw string) []rawStep {
	if raw == "" || raw == "[]" {
		return nil
	}
	var steps []rawStep
	if json.Unmarshal([]byte(raw), &steps) != nil {
		return nil
	}
	return steps
}

// extractSignals derives the cheap, LLM-free SessionSignals a prefilter matches
// against: step kinds (a failed tool step also counts as an "error" signal),
// debug event types (an errored event also counts as "error"), and the session's
// llm_call token total.
func extractSignals(msgs []db.Message, events []db.DebugEvent) SessionSignals {
	sig := SessionSignals{
		StepKinds:   map[string]int{},
		DebugEvents: map[string]int{},
		Tools:       map[string]int{},
	}
	for _, m := range msgs {
		for _, st := range parseSteps(m.Steps) {
			if st.Kind != "" {
				sig.StepKinds[st.Kind]++
			}
			if st.Tool != "" {
				sig.Tools[st.Tool]++ // count by tool name so a lens can target a tool
			}
			if st.IsError {
				sig.StepKinds["error"]++ // a failed tool is an error signal too
			}
		}
	}
	for _, e := range events {
		if e.Type != "" {
			sig.DebugEvents[e.Type]++
		}
		if e.Type == db.DebugTool && e.Name != "" {
			sig.Tools[e.Name]++
		}
		if e.Err {
			sig.DebugEvents["error"]++
		}
		if e.Type == db.DebugLLMCall {
			sig.TokenTotal += e.In + e.Out
		}
	}
	return sig
}

// buildSlice assembles the compact, error-focused transcript fed to the analyzer:
// the session's error/recovery steps (and failed tools) plus the error/repair/
// guardrail/recovery debug events, truncated to sliceCap. Faz 1 is error-centric
// (the shipped tool-errors lens); scope-driven slicing arrives with more lenses.
func (s *Scanner) buildSlice(sess db.Session, msgs []db.Message, events []db.DebugEvent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SESSION %s — %q\n\n## Error / recovery steps\n", sess.ID, sess.Title)
	for _, m := range msgs {
		for _, st := range parseSteps(m.Steps) {
			if st.Kind != "error" && st.Kind != "recovery" && !st.IsError {
				continue
			}
			line := "- [" + st.Kind + "]"
			if st.Tool != "" {
				line += " tool=" + st.Tool
			}
			if st.Reason != "" {
				line += " reason=" + st.Reason
			}
			if txt := firstNonEmpty(st.Text, st.Output); txt != "" {
				line += ": " + truncate(oneLine(txt), 300)
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n## Debug events (errors/repairs/guardrails)\n")
	for _, e := range events {
		switch e.Type {
		case db.DebugError, db.DebugRepair, db.DebugGuardrail, db.DebugRecovery:
			line := "- [" + e.Type + "]"
			if e.Name != "" {
				line += " " + e.Name
			}
			if e.Detail != "" {
				line += ": " + truncate(oneLine(e.Detail), 300)
			}
			b.WriteString(line + "\n")
		}
	}
	out := b.String()
	if len(out) > s.sliceCap {
		out = out[:s.sliceCap] + "\n…(truncated)"
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
