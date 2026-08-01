package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
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
func (r *Runtime) activeSubtreeWorkers(ctx context.Context, coordSessionID string) int {
	ws, err := r.ListSubtreeWorkers(ctx, coordSessionID)
	if err != nil {
		// Unknown rather than zero: treating an unreadable tree as "all done" would
		// let a coordinator conclude on top of workers that are still running.
		r.logger.Warn("coordination: cannot inspect subtree", "coordinator", coordSessionID, "error", err)
		return 0
	}
	n := 0
	for _, w := range ws {
		if w.Running {
			n++
		}
	}
	return n
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
		if w.Running {
			status = "running"
			running++
		} else {
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
// producing. Instead the parent gets a "delegating" progress note, and the node
// closes its task later via report_to_coordinator (or the settle backstop).
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
	if !r.coordSlotFor(sess.ID).hasWorkers() && r.activeSubtreeWorkers(ctx, sess.ID) == 0 {
		// It never delegated (or everything already finished and it synthesized in
		// this same turn): the turn genuinely is the result, report it as usual.
		return false
	}
	r.setOwesReport(ctx, sess.ID, true)
	return true
}

// notifyDelegating tells a coordinator that one of its workers has fanned the work
// out further and is NOT finished — the interim signal that replaces the premature
// completion notification. Deliberately not a <task-notification>: the coordinator
// prompt teaches that only those close a task.
func (r *Runtime) notifyDelegating(coordSessionID, workerSessionID, agentName string, subWorkers int) {
	note := fmt.Sprintf("<task-progress>\n<task-id>%s</task-id>\n<agent>%s</agent>\n<status>delegating</status>\n"+
		"<detail>This worker is a sub-coordinator and has %d worker(s) of its own running. It is NOT finished — "+
		"it will send a <task-notification> when its whole branch is done. Do not treat this as a result and do not wait idly on it; "+
		"work on your other tracks.</detail>\n</task-progress>",
		workerSessionID, agentName, subWorkers)
	r.NotifyCoordinator(coordSessionID, note)
}

// ReportToCoordinator closes a sub-coordinator's task upstream with its own
// synthesis (the report_to_coordinator tool). This is the ONLY thing that reports
// a mid-level node as done — see the file header.
func (r *Runtime) ReportToCoordinator(ctx context.Context, sessionID, status, summary string) error {
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session %s not found: %w", sessionID, err)
	}
	if sess.CoordinatorSessionID == "" {
		return fmt.Errorf("this session has no coordinator to report to")
	}
	if running := r.activeSubtreeWorkers(ctx, sessionID); running > 0 && status == turnStatusCompleted {
		return fmt.Errorf("cannot report \"completed\" while %d of your own workers are still running: wait for their notifications and synthesize them first, or stop them and report \"incomplete\"", running)
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
	note := formatTaskNotification(sessionID, r.agentName(sess.AgentID), status, summary, 0, 0)
	r.NotifyCoordinator(sess.CoordinatorSessionID, note)
	r.logger.Info("coordination: sub-coordinator reported up",
		"session", sessionID, "coordinator", sess.CoordinatorSessionID,
		"status", status, "closedPendingReport", claimed)
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
	if r.activeSubtreeWorkers(ctx, sessionID) > 0 {
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
	note := formatTaskNotification(sessionID, r.agentName(sess.AgentID), turnStatusIncomplete, summary, 0, 0)
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
		if v, ok := r.workerCancels.Load(w.SessionID); ok {
			ctl := v.(*workerCtl)
			ctl.stopped.Store(true)
			ctl.cancel()
			stopped++
		}
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
		if running := r.activeSubtreeWorkers(ctx, sessionID); running > 0 {
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
