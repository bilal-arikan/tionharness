package insight

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/textutil"
	"github.com/bilal-arikan/tionharness/internal/view"
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

	// ExcludeKinds lists Session.Kind values the scan must not look at. nil means
	// the default exclusion — db.MachineTranscriptKinds(), the transcripts the
	// system itself writes (today: the insight run session). Without it a scan
	// analyses its OWN output: every run would spend LLM calls on the previous
	// run's report, burn the MaxSessions budget on them, and feed findings back
	// into findings. Pass an empty (non-nil) slice to scan everything.
	//
	// Modelled as a general kind filter rather than a hardcoded `kind ==
	// "insight"` check so any future machine-written kind is excluded by adding
	// it to the db set, in one place, together with the matching unread rule.
	ExcludeKinds []string
}

// excludedKinds resolves the scope's kind exclusion set, applying the default
// when the caller left it nil.
func (s ScanScope) excludedKinds() map[string]bool {
	kinds := s.ExcludeKinds
	if kinds == nil {
		kinds = db.MachineTranscriptKinds()
	}
	out := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		out[k] = true
	}
	return out
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
	// sink, when set, receives one event per completed analysis (see sink.go).
	sink AnalysisSink
	// sessionFilter returns true for sessions that should be excluded before
	// scan budgets, transcript loading, and analyzer calls are consumed.
	sessionFilter func(context.Context, db.Session) (bool, error)
}

// SetSessionFilter installs a caller-owned session exclusion policy.
func (s *Scanner) SetSessionFilter(filter func(context.Context, db.Session) (bool, error)) {
	s.sessionFilter = filter
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
		sessionFilter: func(ctx context.Context, session db.Session) (bool, error) {
			agent, err := database.GetAgent(ctx, session.AgentID)
			if err != nil {
				return false, err
			}
			return agent.System, nil
		},
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
	excluded := scope.excludedKinds()
	var tasks []analysisTask
enumerate:
	for _, sess := range sessions {
		// Out of scope entirely — not counted in res.Sessions (which reports what
		// the scan considered) and not counted as skipped either.
		if excluded[sess.Kind] {
			continue
		}
		if s.sessionFilter != nil {
			exclude, filterErr := s.sessionFilter(ctx, sess)
			if filterErr != nil {
				return res, filterErr
			}
			if exclude {
				continue
			}
		}
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
			if s.ledger.NeedsScan(l.ID, sess.ID, sess.UpdatedAt, fp, l.Version()) {
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
				if rErr := s.record(l, sess, fp, 0, "clean"); rErr != nil {
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
				lens: l, sess: sess, fp: fp, transcript: s.buildSlice(l, sess, msgs, events),
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
			started := time.Now()
			found, aErr := s.analyzer.Analyze(ctx, AnalysisRequest{Lens: t.lens, SessionID: t.sess.ID, Transcript: t.transcript})
			outcomes[i] = analysisOutcome{task: t, found: found, err: aErr}
			// Live observability, in COMPLETION order (not task order): the caller
			// renders each finished pair as its own card while the scan is still running.
			if s.sink != nil {
				s.sink(AnalysisEvent{
					LensID:       t.lens.ID,
					SessionID:    t.sess.ID,
					SessionTitle: t.sess.Title,
					Findings:     len(found),
					Err:          aErr,
					Duration:     time.Since(started),
				})
			}
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
		if rErr := s.record(t.lens, t.sess, t.fp, len(oc.found), "clean"); rErr != nil {
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

// record upserts a ledger entry for a scanned (lens,session) pair. It takes the
// whole Lens (not just its id) so the entry pins the lens version this result was
// produced with — editing the lens later makes the pair due again.
func (s *Scanner) record(l Lens, sess db.Session, fp string, findingCount int, status string) error {
	return s.ledger.Record(LedgerEntry{
		LensID:          l.ID,
		SessionID:       sess.ID,
		SeenUpdatedAt:   sess.UpdatedAt,
		SeenFingerprint: fp,
		SeenLensVersion: l.Version(),
		ScannedAt:       s.now(),
		FindingCount:    findingCount,
		Status:          status,
	})
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
		for _, st := range view.DecodeSteps(m.Steps) {
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
		if e.Type == db.DebugCacheBreak {
			// A cache break's CAUSE decides which lens should look: an unstable
			// prefix is a context-ordering problem, a TTL cooldown is a cadence
			// problem, and they need opposite advice. The bare `cache_break` count
			// cannot tell them apart, so index the attributed cause as its own
			// signal ("cache_break:ttl-or-server-eviction") — the prefilter is a
			// flat name→count map, so a compound name is the whole mechanism needed.
			if e.Name != "" {
				sig.DebugEvents[db.DebugCacheBreak+":"+e.Name]++
			}
			// Waste is only MEASURED on native Anthropic (OpenRouter folds the cold
			// prefix into plain input and reports no write). Kept as a secondary
			// signal — a lens that must fire on every provider should prefilter on
			// the cause tag above, not on this.
			if e.WasteUSD > 0 {
				sig.DebugEvents["cooling_waste"]++
			}
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
// guardrail/recovery debug events. Faz 1 is error-centric (the shipped
// tool-errors lens); scope-driven slicing arrives with more lenses.
//
// The budget is applied per RECORD, not per byte (view.CapLines), and what did
// not fit is COUNTED. A byte-sliced transcript ends mid-line, so the last error
// arrives truncated into something that still reads like a complete record —
// and the analyzer has no way to tell it was cut, nor how much it is missing.
// Steps get first claim on the budget: they are the primary evidence, and the
// debug events largely restate them.
// ScopeCache is the lens `scope:` value that adds the prompt-cache section to the
// slice (cache_break events with their attributed cause + cost, and the epoch
// lifecycle events that explain whether a break was a DELIBERATE adopt). Without
// it the slice stays error-centric, so a lens that does not care about caching is
// not charged tokens for events it will not use.
const ScopeCache = "cache"

// hasScope reports whether a lens requested a slice surface.
func hasScope(l Lens, want string) bool {
	for _, s := range l.Scope {
		if strings.EqualFold(strings.TrimSpace(s), want) {
			return true
		}
	}
	return false
}

func (s *Scanner) buildSlice(lens Lens, sess db.Session, msgs []db.Message, events []db.DebugEvent) string {
	var stepLines []string
	for _, m := range msgs {
		for _, st := range view.DecodeSteps(m.Steps) {
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
			stepLines = append(stepLines, line)
		}
	}

	var eventLines []string
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
			eventLines = append(eventLines, line)
		}
	}

	// Prompt-cache surface (opt-in via `scope: [cache]`). Without this a lens that
	// prefilters on cache_break was handed a slice containing NO cache evidence at
	// all — it could only guess. Each line carries the attributed cause, the cold
	// prefix size and the measured waste; the interleaved epoch events tell the
	// analyzer whether a break sat next to a DELIBERATE adopt (expected) or stood
	// alone (the signal worth a finding — _Docs/57).
	var cacheLines []string
	if hasScope(lens, ScopeCache) {
		for _, e := range events {
			switch e.Type {
			case db.DebugCacheBreak:
				line := "- [cache_break] at=" + msTime(e.Time)
				if e.Name != "" {
					line += " cause=" + e.Name
				}
				if cold := e.CacheWrite + e.In; cold > 0 {
					line += fmt.Sprintf(" coldTokens=%d", cold)
				}
				if e.WasteUSD > 0 {
					line += fmt.Sprintf(" wasteUsd=%.4f", e.WasteUSD)
					if e.WasteEstimated {
						line += "(est)"
					}
				}
				if e.Detail != "" {
					line += ": " + truncate(oneLine(e.Detail), 200)
				}
				cacheLines = append(cacheLines, line)
			case db.DebugEpoch:
				line := "- [epoch] at=" + msTime(e.Time)
				if e.Name != "" {
					line += " " + e.Name
				}
				cacheLines = append(cacheLines, line)
			}
		}
	}

	header := fmt.Sprintf("SESSION %s — %q\n\n## Error / recovery steps\n", sess.ID, sess.Title)
	const eventsHeader = "\n## Debug events (errors/repairs/guardrails)\n"
	const cacheHeader = "\n## Prompt-cache events (breaks + epoch lifecycle, oldest→newest)\n"

	budget := s.sliceCap - len(header) - len(eventsHeader)
	if len(cacheLines) > 0 {
		budget -= len(cacheHeader)
	}
	steps, stepsDropped := view.CapLines(stepLines, budget)
	spent := 0
	for _, ln := range steps {
		spent += len(ln) + 1
	}
	evts, evtsDropped := view.CapLines(eventLines, budget-spent)
	for _, ln := range evts {
		spent += len(ln) + 1
	}
	cache, cacheDropped := view.CapLines(cacheLines, budget-spent)

	var b strings.Builder
	b.WriteString(header)
	for _, ln := range steps {
		b.WriteString(ln + "\n")
	}
	if stepsDropped > 0 {
		fmt.Fprintf(&b, "…(%d more error/recovery step(s) omitted for size)\n", stepsDropped)
	}
	b.WriteString(eventsHeader)
	for _, ln := range evts {
		b.WriteString(ln + "\n")
	}
	if evtsDropped > 0 {
		fmt.Fprintf(&b, "…(%d more debug event(s) omitted for size)\n", evtsDropped)
	}
	if len(cacheLines) > 0 {
		b.WriteString(cacheHeader)
		for _, ln := range cache {
			b.WriteString(ln + "\n")
		}
		if cacheDropped > 0 {
			fmt.Fprintf(&b, "…(%d more cache event(s) omitted for size)\n", cacheDropped)
		}
	}
	return b.String()
}

// msTime renders a debug event's unix-millisecond stamp as a UTC clock label. The
// cadence lenses reason about the GAP between events (a warm prefix cools after
// an hour of silence), so the slice must carry when each one happened.
func msTime(ms int64) string {
	if ms <= 0 {
		return "?"
	}
	return time.UnixMilli(ms).UTC().Format("2006-01-02 15:04:05Z")
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

// truncate cuts s to at most n bytes on a rune boundary — see textutil for why
// a raw byte slice is fatal on the codex prompt path.
func truncate(s string, n int) string { return textutil.TruncBytesEllipsis(s, n) }
