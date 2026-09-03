package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// coordination_tree.go carries the pieces that only exist because a coordinator
// tree can nest arbitrarily deep (see _Docs/47): subtree queries, the deferred
// upward report of a mid-level node, cascading stops, and the self-service
// coordinator-mode toggle.
//
// The one rule everything here protects: a MID-LEVEL node finishing its turn is
// not the same as its task being done. Its turn typically ends the moment it has
// fanned its work out to its own workers, so reporting that turn upward as
// "completed" would tell its coordinator to move on while the whole branch below
// is still running. Hence: withhold the completion notification while the subtree
// is live (deferWorkerReport), let the node close its own task explicitly
// (ReportToCoordinator), and back that up with an automatic report if it goes
// quiet (settleReportBackstop).

// ---- subtree queries ----

// SubtreeWorker is one node of a coordinator's subtree: a WorkerInfo plus its
// position in the tree, so a caller can render or reason about the shape without
// re-walking the parent links.
type SubtreeWorker struct {
	WorkerInfo
	// Depth is relative to the coordinator the query started from (its direct
	// workers are 1).
	Depth int
	// ParentSessionID is the coordinator this node reports to.
	ParentSessionID string
	// IsCoordinator marks a node that drives workers of its own.
	IsCoordinator bool
}

// ListSubtreeWorkers returns EVERY descendant of a coordinator session — its
// direct workers, their workers, and so on — in breadth-first order. The
// coordinator itself is not included.
//
// This is the tree-aware counterpart of ListWorkers, and the difference matters
// for liveness: a mid-level node sits idle between its own turns, so judging a
// branch by its direct child alone reports "finished" while grandchildren are
// still working.
func (r *Runtime) ListSubtreeWorkers(ctx context.Context, coordSessionID string) ([]SubtreeWorker, error) {
	tree, err := r.db.ListCoordinatorTree(ctx, coordSessionID)
	if err != nil {
		return nil, err
	}
	// ListCoordinatorTree normalizes to the ROOT of the tree, which may sit above
	// the coordinator we were asked about. Re-root the walk here so a mid-level
	// caller sees only what is genuinely below it (its coordinator's other branches
	// are none of its business).
	byID := make(map[string]db.Session, len(tree))
	children := map[string][]db.Session{}
	for _, s := range tree {
		byID[s.ID] = s
		if s.CoordinatorSessionID != "" {
			children[s.CoordinatorSessionID] = append(children[s.CoordinatorSessionID], s)
		}
	}
	if _, ok := byID[coordSessionID]; !ok {
		return nil, fmt.Errorf("session %s is not part of the coordinator tree it claims", coordSessionID)
	}
	var out []SubtreeWorker
	type queued struct {
		id    string
		depth int
	}
	queue := []queued{{coordSessionID, 0}}
	seen := map[string]bool{coordSessionID: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, kid := range children[cur.id] {
			if seen[kid.ID] {
				continue // defensive: a hand-edited cycle must not hang the walk
			}
			seen[kid.ID] = true
			out = append(out, SubtreeWorker{
				WorkerInfo:      r.workerInfoFor(ctx, kid),
				Depth:           cur.depth + 1,
				ParentSessionID: kid.CoordinatorSessionID,
				IsCoordinator:   kid.IsCoordinator(),
			})
			queue = append(queue, queued{kid.ID, cur.depth + 1})
		}
	}
	return out, nil
}

// activeSubtreeWorkers counts the descendants of a coordinator whose turn is
// currently running. Used to decide whether a branch is genuinely settled — see
// the file header for why the direct-children count is not enough.
//
// The error is part of the answer and must not be collapsed into a count: an
// unreadable tree means UNKNOWN, and every caller has to treat unknown as "the
// branch may still be live". Returning 0 there would let a coordinator conclude
// on top of workers that are still running — the exact failure this file exists
// to prevent.
func (r *Runtime) activeSubtreeWorkers(ctx context.Context, coordSessionID string) (int, error) {
	ws, err := r.ListSubtreeWorkers(ctx, coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: cannot inspect subtree", "coordinator", coordSessionID, "error", err)
		return 0, err
	}
	n := 0
	for _, w := range ws {
		if w.Running {
			n++
		}
	}
	return n, nil
}

// formatWorkerTree renders a subtree as an indented status list for the
// list_workers(scope="subtree") tool.
func formatWorkerTree(ws []SubtreeWorker) string {
	if len(ws) == 0 {
		return "No workers have been spawned under this coordinator yet."
	}
	running, finished := 0, 0
	var b strings.Builder
	fmt.Fprintf(&b, "%d worker(s) in your subtree (all levels):\n", len(ws))
	for _, w := range ws {
		status := "finished"
		switch {
		case w.Delegating:
			// Live only through its branch (or still owing a report): counted with the
			// running ones, because the coordinator must not conclude on top of it.
			status = "delegating"
			running++
		case w.Running:
			status = "running"
			running++
		default:
			finished++
		}
		indent := strings.Repeat("  ", w.Depth-1)
		role := ""
		if w.IsCoordinator {
			role = " (sub-coordinator)"
		}
		fmt.Fprintf(&b, "%s- %s%s [%s] (%s)", indent, w.AgentName, role, status, w.SessionID)
		if w.Summary != "" {
			fmt.Fprintf(&b, " — %s", w.Summary)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Summary: %d running, %d finished.", running, finished)
	return b.String()
}

// ---- deferred upward report ----

// deferWorkerReport decides whether a finished worker turn should be reported to
// its coordinator right now, or withheld because this worker is itself a
// coordinator whose own branch is still working.
//
// Withholding is the whole point: a sub-coordinator's first turn ends as soon as
// it has spawned its workers, and reporting THAT as "completed" would tell its
// coordinator the subtask is done while the branch below has not even started
// producing. Nothing at all goes up in that moment — the parent is not woken for a
// non-result. The state stays visible in the parent's LIVE worker view instead
// (the pending-report flag makes the node read as "delegating"; see
// subCoordinatorBusy), and the node closes its task later via
// report_to_coordinator (or the settle backstop).
//
// Only a clean turn is deferred. A failed/killed sub-coordinator reports
// immediately — its branch is broken and the parent must be able to react — and
// its subtree is cancelled so nothing keeps burning budget under a node nobody is
// waiting on any more.
func (r *Runtime) deferWorkerReport(ctx context.Context, sess db.Session, status string) bool {
	if !sess.IsCoordinator() {
		return false // a leaf worker's turn IS its result
	}
	if status != turnStatusCompleted {
		r.stopSubtree(ctx, sess.ID, "sub-coordinator turn ended with status "+status)
		return false
	}
	if !r.coordSlotFor(sess.ID).hasWorkers() {
		active, err := r.activeSubtreeWorkers(ctx, sess.ID)
		if err == nil && active == 0 {
			// It never delegated (or everything already finished and it synthesized in
			// this same turn): the turn genuinely is the result, report it as usual.
			return false
		}
		if err != nil {
			// Unknown subtree: withhold the completion notification. A needless
			// "delegating" state costs the coordinator one wasted wait that the settle
			// backstop resolves; reporting completed over a live branch is unrecoverable.
			r.logger.Warn("coordination: withholding worker report, subtree state unknown",
				"session", sess.ID, "error", err)
		}
	}
	r.setOwesReport(ctx, sess.ID, true)
	return true
}

// ---- end-of-turn delivery of the upward report ----

// pendingUpwardReport is the per-turn stash a report_to_coordinator call writes
// into instead of notifying the parent on the spot.
//
// Sending inline woke the parent while the reporting node was still mid-turn: the
// tool call is rarely the last thing a turn does, so the coordinator started
// reading a "finished" branch whose final assistant reply had not been persisted
// yet. The note is therefore held here and flushed from the worker turn's terminal
// path (runWorkerWithCtl), after the reply is on disk.
//
// One stash per WORKER RUN, not per turn attempt: the idle-resume loop builds a
// fresh context per attempt, and a report made in a cut-short attempt must not be
// dropped by the retry.
type pendingUpwardReport struct {
	mu     sync.Mutex
	coord  string
	note   string
	status string // the status the agent reported (turnStatus*), for the fold below
	armed  bool
}

// stash records the note to deliver, overwriting any earlier one. Two
// report_to_coordinator calls in the same turn mean the agent corrected itself:
// the last one wins and only that one is sent.
func (p *pendingUpwardReport) stash(coordSessionID, note, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.coord, p.note, p.status, p.armed = coordSessionID, note, status, true
}

// foldIntoTerminal consumes the stash for a node whose turn IS its result (a
// leaf worker, or a sub-coordinator that never delegated) and returns the status
// its terminal <task-notification> should carry.
//
// Before this fold the parent got BOTH notes: the stashed report_to_coordinator
// summary and, seconds later, the runtime's terminal notification with the same
// "completed" status and the full reply (observed 2026-09-03 on every worker that
// called the tool: two notes 4s apart, one wasted coordinator turn). For such a
// node the terminal note already carries everything — the persisted reply, tool
// count, the all-idle fold — so the explicit report is dropped. What survives is
// the agent's self-assessment: a weaker reported status ("incomplete" / "failed"
// on a turn that ended cleanly) overrides the turn status, because the agent
// knows better than the runtime whether the task is actually done.
//
// Deferred sub-coordinators (deferWorkerReport) are NOT folded: their terminal
// note is withheld, so the stashed report is the only thing that reaches the
// parent and flushUpwardReport must still deliver it.
func (p *pendingUpwardReport) foldIntoTerminal(turnStatus string) string {
	_, _, ok := p.take()
	if !ok {
		return turnStatus
	}
	p.mu.Lock()
	reported := p.status
	p.mu.Unlock()
	if reported != "" && reported != turnStatusCompleted && turnStatus == turnStatusCompleted {
		return reported
	}
	return turnStatus
}

// take hands out the stashed note exactly once, so a second flush (an early return
// plus the deferred backstop) cannot report the same result twice.
func (p *pendingUpwardReport) take() (coordSessionID, note string, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.armed {
		return "", "", false
	}
	p.armed = false
	return p.coord, p.note, true
}

type upwardReportKey struct{}

// withPendingUpwardReport attaches a run's stash to a turn context.
func withPendingUpwardReport(ctx context.Context, p *pendingUpwardReport) context.Context {
	return context.WithValue(ctx, upwardReportKey{}, p)
}

// pendingUpwardReportFrom returns the stash attached to ctx, or nil when the call
// is not running inside a worker turn (a chat turn opened directly on the worker
// session, or a direct runtime call) — those still send immediately, because there
// is no terminal path that would flush for them.
func pendingUpwardReportFrom(ctx context.Context) *pendingUpwardReport {
	p, _ := ctx.Value(upwardReportKey{}).(*pendingUpwardReport)
	return p
}

// flushUpwardReport delivers the note a report_to_coordinator call stashed during
// this run. Called on EVERY terminal path of a worker turn, including failed and
// killed: the tool already claimed the pending-report flag, so a note dropped here
// would leave the coordinator above waiting on a report that can never arrive.
func (r *Runtime) flushUpwardReport(p *pendingUpwardReport, sessionID string) {
	coord, note, ok := p.take()
	if !ok {
		return
	}
	r.logger.Info("coordination: delivering sub-coordinator report at end of turn",
		"session", sessionID, "coordinator", coord)
	r.NotifyCoordinator(coord, note)
}

// ReportToCoordinator closes a sub-coordinator's task upstream with its own
// synthesis (the report_to_coordinator tool). This is the ONLY thing that reports
// a mid-level node as done — see the file header.
//
// The validations run HERE, at call time, so the tool can still refuse a premature
// "completed" and still win the claim race against the settle backstop. Only the
// delivery is deferred to the end of the turn (see pendingUpwardReport).
func (r *Runtime) ReportToCoordinator(ctx context.Context, sessionID, status, summary string) error {
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session %s not found: %w", sessionID, err)
	}
	if sess.CoordinatorSessionID == "" {
		return fmt.Errorf("this session has no coordinator to report to")
	}
	if status == turnStatusCompleted {
		running, err := r.activeSubtreeWorkers(ctx, sessionID)
		if err != nil {
			// Refuse rather than guess: "completed" is the one status that tells the
			// parent it may build on this result.
			return fmt.Errorf("cannot verify whether your own workers are still running (%w); retry, or stop them and report \"incomplete\"", err)
		}
		if running > 0 {
			return fmt.Errorf("cannot report \"completed\" while %d of your own workers are still running: wait for their notifications and synthesize them first, or stop them and report \"incomplete\"", running)
		}
	}
	// Clear the outstanding report BEFORE sending, so an armed settle backstop
	// racing this call finds nothing to claim and stays quiet. The agent's own
	// synthesis is always the better report; the backstop only exists for silence.
	// (A backstop that already fired microseconds earlier still wins the claim — the
	// coordinator then gets the runtime's "incomplete" note followed by this real
	// result, which is recoverable and strictly more informative than dropping it.)
	claimed, err := r.db.ClaimCoordinatorReport(ctx, sessionID)
	if err != nil {
		return err
	}
	note := formatTaskNotification(sessionID, sess.AgentID, r.agentName(sess.AgentID), sess.Model, status, summary, 0, 0)
	deferred := false
	if stash := pendingUpwardReportFrom(ctx); stash != nil {
		stash.stash(sess.CoordinatorSessionID, note, status)
		deferred = true
	} else {
		r.NotifyCoordinator(sess.CoordinatorSessionID, note)
	}
	r.logger.Info("coordination: sub-coordinator reported up",
		"session", sessionID, "coordinator", sess.CoordinatorSessionID,
		"status", status, "closedPendingReport", claimed, "deferredToTurnEnd", deferred)
	return nil
}

// settleReportBackstop auto-reports for a sub-coordinator that owes its parent a
// result but has gone quiet: every worker of its own has finished, it has had its
// turns to synthesize, and it still did not call report_to_coordinator.
//
// Without this the tree can hang on a single node that simply forgot to report —
// its parent waits forever on a branch that is provably idle. The auto-report is
// marked "incomplete", never "completed": the runtime has no way to know the work
// was actually finished, and claiming so would be worse than admitting it does
// not know.
func (r *Runtime) settleReportBackstop(ctx context.Context, sessionID string) {
	if !r.owesReportNow(ctx, sessionID) {
		return
	}
	running, err := r.activeSubtreeWorkers(ctx, sessionID)
	if err != nil {
		// Unknown subtree: do not settle. The node stays owing its report and a later
		// backstop (or the node itself) resolves it once the tree reads again.
		r.logger.Warn("coordination: not settling, subtree state unknown", "session", sessionID, "error", err)
		return
	}
	if running > 0 {
		return // branch still live; nothing to settle yet
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil || sess.CoordinatorSessionID == "" {
		return
	}
	// Take the report atomically. The check above is only a cheap early-out: two
	// backstops can be armed for the same session (one per drain exit) and the
	// agent's own report_to_coordinator can land in the same instant. Whoever loses
	// the claim must stay silent, or the coordinator above sees one task reported
	// twice with conflicting statuses.
	won, err := r.db.ClaimCoordinatorReport(ctx, sessionID)
	if err != nil || !won {
		return
	}
	last := strings.TrimSpace(r.lastAssistantText(ctx, sessionID))
	if last == "" {
		last = "(bu oturumda kaydedilmiş bir yanıt yok)"
	}
	summary := "⚠️ Bu alt-koordinatör tüm worker'ları bittikten sonra `report_to_coordinator` çağırmadı; " +
		"aşağıdaki metin onun son turundan otomatik olarak alındı ve DOĞRULANMIŞ bir sonuç değildir. " +
		"Eksik görünüyorsa `send_to_worker` ile açık bir rapor iste.\n\n" + last
	note := formatTaskNotification(sessionID, sess.AgentID, r.agentName(sess.AgentID), sess.Model, turnStatusIncomplete, summary, 0, 0)
	r.logger.Warn("coordination: sub-coordinator did not report; auto-reporting",
		"session", sessionID, "coordinator", sess.CoordinatorSessionID)
	r.NotifyCoordinator(sess.CoordinatorSessionID, note)
}

// ---- cascading stop ----

// stopSubtree cancels every RUNNING descendant of a session, deepest work
// included. A plain stop_worker on a mid-level node would leave its grandchildren
// running: they would keep spending budget and then notify a node nobody is
// waiting on any more, waking a zombie turn.
//
// Best-effort by design — a worker that finishes between the listing and the
// cancel is simply not running any more, which is the outcome we wanted.
func (r *Runtime) stopSubtree(ctx context.Context, sessionID, reason string) {
	ws, err := r.ListSubtreeWorkers(ctx, sessionID)
	if err != nil {
		r.logger.Warn("coordination: cannot list subtree to stop", "session", sessionID, "error", err)
		return
	}
	stopped := 0
	for _, w := range ws {
		if !w.Running {
			continue
		}
		r.workerQueueMu.Lock()
		if v, ok := r.workerCancels.Load(w.SessionID); ok {
			ctl := v.(*workerCtl)
			ctl.stopped.Store(true)
			ctl.cancel()
			stopped++
		}
		r.workerQueueMu.Unlock()
	}
	if stopped > 0 {
		r.logger.Info("coordination: cascaded stop through subtree",
			"session", sessionID, "stopped", stopped, "reason", reason)
	}
}

// ---- self-service coordinator mode ----

// SetSessionCoordinatorMode turns a session's coordinator capability on or off at
// the agent's own request (the set_coordinator_mode tool). Returns the note the
// agent sees.
//
// Turning it OFF while workers are still running is REFUSED rather than silently
// accepted: the notifications keep arriving either way (they are queued by the
// coordination loop, not by the tool surface), but the agent would no longer have
// the tools to act on them — a coordinator that cannot stop or continue its own
// running workers.
func (r *Runtime) SetSessionCoordinatorMode(ctx context.Context, sessionID string, enabled bool) (string, error) {
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("session %s not found: %w", sessionID, err)
	}
	if sess.IsCoordinator() == enabled {
		if enabled {
			return "Coordinator mode is already on — spawn_worker and the other coordination tools are available to you.", nil
		}
		return "Coordinator mode is already off.", nil
	}
	if !enabled {
		running, err := r.activeSubtreeWorkers(ctx, sessionID)
		if err != nil {
			// Unknown, so refuse: dropping the tools while workers may still be running
			// leaves the agent unable to stop or continue them.
			return "", fmt.Errorf("cannot verify whether your workers are still running (%w); try again", err)
		}
		if running > 0 {
			return "", fmt.Errorf("cannot turn coordinator mode off while %d worker(s) of yours are still running: stop them with stop_worker, or wait for their notifications, then try again", running)
		}
	}
	if enabled {
		// A session may only become a coordinator if its own workers would still fit
		// under the depth limit — otherwise it would get the tools and every
		// spawn_worker call would fail, which reads as a broken tool rather than a
		// deliberate boundary.
		if maxDepth := r.tun.CoordinatorMaxDepth(); maxDepth > 0 && sess.CoordinatorDepth >= maxDepth {
			return "", fmt.Errorf("cannot become a coordinator: you are at depth %d of a coordinator tree and the limit is %d levels, so you could not spawn any workers. Do this work yourself", sess.CoordinatorDepth, maxDepth)
		}
	}
	if err := r.db.SetCoordinatorMode(ctx, sessionID, enabled); err != nil {
		return "", err
	}
	// The system prompt AND the tool schemas are frozen per prompt epoch (see
	// _Docs/57), so without this the agent would be told "coordinator mode is on"
	// and then find no spawn_worker for the rest of the session — the toggle would
	// look like a broken tool. Dropping the snapshot makes the next turn recompose
	// the prefix and the tool set from live state.
	r.RefreshPromptEpoch(ctx, sessionID)
	r.emitCoordinationModeEvent(sessionID, enabled)
	if enabled {
		return "Coordinator mode is ON. The coordinator manual and spawn_worker / send_to_worker / stop_worker / list_workers become available on your NEXT turn (this turn's tool set was frozen when it started). End your turn, then fan the work out.", nil
	}
	return "Coordinator mode is OFF. The coordination tools disappear on your next turn; you are working alone again.", nil
}

// emitCoordinationModeEvent publishes a coordinator-mode flip so every open window
// swaps the role chip and shows/hides the coordination panel — the same
// cross-window sync the REST toggle gets, for the agent-driven path.
func (r *Runtime) emitCoordinationModeEvent(sessionID string, enabled bool) {
	state := "off"
	if enabled {
		state = "on"
	}
	r.publish(events.Event{
		Type:   events.TypeSession,
		Level:  "info",
		Target: map[string]string{"sessionId": sessionID, "op": "role", "coordinatorMode": state},
	})
}

// ---- shared helpers ----

// hasWorkers reports whether this coordinator currently has active workers.
func (s *coordSlot) hasWorkers() bool { return s.workers.Load() > 0 }

// setOwesReport records (or clears) a session's outstanding upward report. The
// flag lives on the SESSION, not on the in-memory slot, because the gap it covers
// is a restart: a mid-level node waiting on its branch looks healthy on disk (its
// last message is its own reply), so orphan recovery never touches it and an
// in-memory flag would simply vanish — leaving its coordinator waiting forever.
func (r *Runtime) setOwesReport(ctx context.Context, sessionID string, pending bool) {
	if err := r.db.SetCoordinatorReportPending(ctx, sessionID, pending); err != nil {
		r.logger.Warn("coordination: cannot persist pending-report flag",
			"session", sessionID, "pending", pending, "error", err)
	}
}

// owesReportNow reports whether an upward report is still outstanding for this
// session. Reads the persisted flag, so it is correct across restarts.
func (r *Runtime) owesReportNow(ctx context.Context, sessionID string) bool {
	s, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return false
	}
	return s.CoordinatorReportPending
}

// RecoverPendingReports re-arms the settle backstop at boot for every mid-level
// node that still owes its coordinator a report. Called after RecoverOrphanedTurns
// so recovered workers have already enqueued their notifications: a node that is
// about to get a turn out of that recovery finds its slot busy and reports for
// itself, and only a genuinely quiet one is auto-reported.
func (r *Runtime) RecoverPendingReports(ctx context.Context) {
	pending, err := r.db.ListPendingCoordinatorReports(ctx)
	if err != nil {
		return
	}
	for _, sess := range pending {
		r.logger.Info("recover: re-arming pending upward report",
			"session", sess.ID, "coordinator", sess.CoordinatorSessionID)
		r.scheduleSettleBackstop(sess.ID)
	}
}

// DefaultCoordinatorSettleGraceSec is how long the settle backstop waits after a
// branch goes quiet before auto-reporting upward, when the setting is unset.
//
// It is a grace period, not a timeout: it gives the node's own reconcile turn room
// to run and call report_to_coordinator properly. Too short and a slow model's
// synthesis turn loses the race and the parent gets a needless "incomplete";
// too long and a genuinely stalled branch keeps its coordinator waiting.
const DefaultCoordinatorSettleGraceSec = 30
