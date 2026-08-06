package view

import (
	"fmt"
	"strings"
	"time"
)

// Worker is one entry of a coordinator's fleet, flattened out of the runtime's
// live state.
//
// The projection takes this rather than internal/agent's WorkerInfo because
// agent depends on tools, which depends on this package — importing it would
// close a cycle. The caller maps; this file stays a pure renderer.
type Worker struct {
	SessionID string
	AgentName string
	// Running is a live turn of this worker's own.
	Running bool
	// Delegating marks a SUB-COORDINATOR that is "running" only in the sense that
	// the workers IT spawned are. It has no turn in flight and has not reported.
	Delegating bool
	// Summary is the first line of a finished worker's reply.
	Summary string
	// StartedAt is when a running worker's current turn began (unix SECONDS).
	// Zero means genuinely unknown, and the projection then omits the duration
	// rather than printing a fabricated one.
	StartedAt int64
}

// WorkersInput is everything the worker-fleet projection reads.
type WorkersInput struct {
	Workers []Worker
	// Now is the clock used for elapsed time. Zero means time.Now().
	Now time.Time
}

// workersMaxListed caps how many workers the block spells out. This projection
// is a PUSH channel — it is regenerated into every coordinator turn's prompt —
// so an uncapped list would let a wide fleet quietly dominate the context. The
// dropped ones are still COUNTED in the summary line, so the coordinator's
// arithmetic ("is anyone still running?") stays correct.
const workersMaxListed = 20

// ProjectWorkers renders a coordinator's live fleet.
//
// Unlike the other projections in this package this one is rendered in ENGLISH:
// its only consumer is a coordinator's system prompt, where every neighbouring
// block (autonomous boot reminder, shell capability, epoch note) is English too.
// Mixing languages inside one prompt reads worse than either choice.
//
// The wording is load-bearing, not decorative. It exists to defeat a specific
// failure: a coordinator that read a coalesced notification stream and concluded
// a finished worker was still running would wait forever (_Docs/47). Hence the
// "trust THIS over the notifications in history" framing, the explicit
// DELEGATING state, and the closing nudge when nothing is left running.
func ProjectWorkers(in WorkersInput, level Level, lens Lens) (View, error) {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	running, finished := 0, 0
	pass, fail := 0, 0
	for _, w := range in.Workers {
		if w.Running || w.Delegating {
			running++
		} else {
			finished++
			switch parseVerdict(w.Summary) {
			case verdictPass:
				pass++
			case verdictFail:
				fail++
			}
		}
	}

	v := View{
		Ref:    Ref{Kind: KindWorkers, ID: WorkersRefID},
		Level:  level,
		Lens:   lens,
		AsOf:   now,
		Source: fmt.Sprintf("%d/%d", running, finished),
	}
	v.Header = "# Worker status (live, authoritative)\n" +
		"Regenerated every turn from real session state; trust THIS over the notifications in history."

	if level == LevelTiny || len(in.Workers) == 0 {
		v.Body = workerSummaryLine(running, finished, pass, fail)
		v.finalize()
		return v, nil
	}

	// Running workers first: they are what a coordinator is deciding about.
	listed := in.Workers
	if len(listed) > workersMaxListed {
		dropped := len(in.Workers) - workersMaxListed
		listed = prioritiseRunning(listed, workersMaxListed)
		v.Elided, v.ElidedUnit = dropped, "worker"
		// English, and stated as "finished" rather than a bare count: the cap only
		// ever drops finished workers (running ones are kept first), and saying so
		// is what stops the coordinator wondering whether a hidden entry might
		// still be the one it is waiting for.
		v.ElidedNote = fmt.Sprintf(
			"(%d more finished worker(s) not listed; all of them are included in the counts above.)",
			dropped)
	}

	var l lines
	for _, w := range listed {
		l.add("%s", workerLine(w, now))
	}
	l.add("%s", workerSummaryLine(running, finished, pass, fail))
	v.Body = l.String()
	v.finalize()
	return v, nil
}

// Verdict markers a validator worker emits as the FIRST line of its report (see
// the subagent-validator prompt). Reading this contracted marker is a rule-based
// L1 signal — deterministic, no LLM — not fragile parsing of free prose: only the
// exact "VERDICT: PASS|FAIL" contract is recognised, anything else yields none.
const (
	verdictPass = "PASS"
	verdictFail = "FAIL"
)

// parseVerdict extracts a validator's PASS/FAIL from the first line of its reply,
// or "" when the line is not a verdict marker (a non-validator worker, or a
// validator that broke its contract — neither is guessed at).
func parseVerdict(summary string) string {
	line := summary
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	const prefix = "VERDICT:"
	if !strings.HasPrefix(strings.ToUpper(line), prefix) {
		return ""
	}
	rest := strings.ToUpper(strings.TrimSpace(line[len(prefix):]))
	switch {
	case strings.HasPrefix(rest, verdictPass):
		return verdictPass
	case strings.HasPrefix(rest, verdictFail):
		return verdictFail
	default:
		return ""
	}
}

// workerLine renders one worker: who, what state, how long, and what it said.
func workerLine(w Worker, now time.Time) string {
	status := "finished"
	switch {
	case w.Delegating:
		// Not a live turn of its own: it is waiting on the workers it spawned.
		// Spelling that out stops the coordinator reading "RUNNING" as "about to
		// answer" and, worse, reading "finished" as "its result is in".
		status = "DELEGATING (its own workers are running; it has not reported yet)"
	case w.Running:
		status = "RUNNING"
		// Elapsed time is what turns "RUNNING" into a decision: a worker 14
		// minutes in is a different situation from one that just started.
		if started := tsSec(w.StartedAt); !started.IsZero() {
			status += " for " + dur(now.Sub(started))
		}
	}

	// A finished validator's verdict is the signal a coordinator acts on, so it is
	// hoisted into a scannable badge ahead of the free-text summary rather than left
	// buried in it. Running workers have no verdict yet.
	badge := ""
	if !w.Running && !w.Delegating {
		switch parseVerdict(w.Summary) {
		case verdictPass:
			badge = " ✅ PASS"
		case verdictFail:
			badge = " ❌ FAIL"
		}
	}

	line := fmt.Sprintf("- %s [%s]%s (%s)", w.AgentName, status, badge, w.SessionID)
	if w.Summary != "" {
		line += " — " + clip(w.Summary, 200)
	}
	return line
}

// workerSummaryLine is the arithmetic the coordinator acts on. It counts the
// WHOLE fleet, including workers the list elided. When any finished worker carried
// a validator verdict, a PASS/FAIL tally is appended so the coordinator can scan
// outcomes without re-reading each line — and a FAIL is called out as needing a
// re-task, since a failed verdict is work that is NOT done.
func workerSummaryLine(running, finished, pass, fail int) string {
	s := fmt.Sprintf("Summary: %d running, %d finished.", running, finished)
	if pass > 0 || fail > 0 {
		s += fmt.Sprintf(" Verdicts: %d PASS, %d FAIL.", pass, fail)
		if fail > 0 {
			s += " A FAIL is unfinished work — re-task its implementer; do not commit or conclude on it."
		}
	}
	if running == 0 {
		s += " ALL workers are finished — there is NO running worker to wait for;" +
			" spawn the remaining steps or conclude."
	}
	return s
}

// prioritiseRunning keeps the first max workers, preferring the ones still in
// flight. Dropping a running worker to make room for a finished one would hide
// exactly the entry the coordinator needs.
func prioritiseRunning(ws []Worker, max int) []Worker {
	out := make([]Worker, 0, max)
	for _, w := range ws {
		if w.Running || w.Delegating {
			out = append(out, w)
			if len(out) == max {
				return out
			}
		}
	}
	for _, w := range ws {
		if !w.Running && !w.Delegating {
			out = append(out, w)
			if len(out) == max {
				return out
			}
		}
	}
	return out
}
