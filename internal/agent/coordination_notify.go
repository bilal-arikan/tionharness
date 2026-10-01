// Coordinator notifications: the task-notification notes a finished worker injects into its coordinator session, the persisted note records behind them, and the worker start/finish events the UI listens to.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// NotifyCoordinator persists a <task-notification> as a user message in the
// coordinator session, then asks the per-session turn queue to run a coordinator
// turn. Safe to call from many workers concurrently: the queue serializes turns
// and coalesces pile-ups. No-op when coordSessionID is empty.
func (r *Runtime) NotifyCoordinator(coordSessionID, note string) {
	// Callers outside runWorker (orphan reclaim, sub-coordinator reports, the settle
	// backstop) did not observe a zero-crossing, so they never fold. Their all-idle
	// transition — if any — is the drain loop's standalone backstop to report.
	if err := r.notifyCoordinator(coordSessionID, note, false, nil, ""); err != nil {
		r.logger.Error("coordination: notification failed", "coordinator", coordSessionID, "error", err)
	}
}

// notifyCoordinator is NotifyCoordinator with the last-worker observation from
// releaseOnce. lastWorker=true means THIS notification's worker took the fleet to
// zero and may therefore carry the folded <coordination-status> note.
func (r *Runtime) notifyCoordinator(coordSessionID, note string, lastWorker bool, steps []TurnStep, terminalWorkerSessionID string) error {
	coordSessionID = strings.TrimSpace(coordSessionID)
	if coordSessionID == "" || strings.TrimSpace(note) == "" {
		return nil
	}
	// Same byte limit as send_message / send_to_worker, applied differently: this
	// path is ONE-WAY (the worker turn that produced the note is already over), so
	// there is nobody to hand a message_too_large error back to. Refusing here
	// would leave the coordinator waiting forever for a worker that has finished,
	// so an oversized note is cut and the cut is stated explicitly instead.
	if capped, cut := capNotification(note, r.tun.AgentMessageMaxBytes()); cut {
		r.logger.Warn("coordination: task-notification capped",
			"coordinator", coordSessionID, "bytes", len(note), "max", r.tun.AgentMessageMaxBytes())
		note = capped
	}
	stepsJSON := ""
	if len(steps) > 0 {
		encoded, err := json.Marshal(steps)
		if err != nil {
			r.logger.Error("coordination: failed to encode worker steps", "coordinator", coordSessionID, "error", err)
			return fmt.Errorf("encode worker steps: %w", err)
		}
		stepsJSON = string(encoded)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	slot := r.coordSlotFor(coordSessionID)
	releaseAdmission, err := acquireCoordinatorAdmission(ctx, slot)
	if err != nil {
		return fmt.Errorf("acquire coordinator admission: %w", err)
	}
	defer releaseAdmission()

	slot.mu.Lock()
	// Fold the all-idle signal into THIS notification when it is the last worker's,
	// so the coordinator gets the final result and "everyone is done" in one turn
	// instead of two.
	//
	// lastWorker comes from the caller's zero-crossing observation (releaseOnce), NOT
	// from re-reading the counter here: by the time this runs another worker may have
	// been spawned or finished, and only the goroutine that actually took the fleet to
	// zero owns this transition. hadWorkers gates it exactly as the drain loop's sweep
	// does; stallHalted is excluded because a halted coordinator gets no auto-turn, so
	// an appended note would only mislead.
	folded := lastWorker && slot.hadWorkers && !slot.ackedIdle && !slot.stallHalted
	if folded {
		slot.ackedIdle = true
		slot.idleFolded = true
		note = attachCoordinationStatus(note)
	}
	slot.mu.Unlock()

	if _, err := r.recordInjectedUserMessage(ctx, db.Message{
		SessionID: coordSessionID,
		Role:      "user",
		Origin:    "worker-note",
		Text:      note,
		Steps:     stepsJSON,
	}); err != nil {
		// The fold is only valid if the note carrying it actually reached history.
		// Give the claim back so a later real notification may claim this transition.
		if folded {
			slot.mu.Lock()
			slot.ackedIdle = false
			slot.idleFolded = false
			slot.mu.Unlock()
		}
		r.logger.Error("coordination: failed to persist task-notification", "coordinator", coordSessionID, "worker", terminalWorkerSessionID, "error", err)
		return fmt.Errorf("persist task-notification: %w", err)
	}
	persistedAt := time.Now()
	if r.coordAfterWorkerNotePersist != nil {
		r.coordAfterWorkerNotePersist(coordSessionID)
	}
	if terminalWorkerSessionID != "" {
		r.observeReport(ReportEvent{
			CoordinatorID: coordSessionID, WorkerID: terminalWorkerSessionID,
			Status: noteTag(note, "status"), LastWorker: lastWorker,
			ToolUses: countToolSteps(steps), NoteBytes: len(note),
		})
	}
	var archiveErr error
	if terminalWorkerSessionID != "" {
		if err := r.db.SetSessionState(context.Background(), terminalWorkerSessionID, "archived"); err != nil {
			archiveErr = fmt.Errorf("archive terminal worker session: %w", err)
			r.logger.Error("coordination: failed to archive terminal worker session", "coordinator", coordSessionID, "worker", terminalWorkerSessionID, "error", err)
		}
	}
	r.enqueueCoordinatorWorkerTurnAt(coordSessionID, folded, persistedAt)
	return archiveErr
}

// recordInjectedUserNote persists a runtime-injected user-role note to a
// coordination session (a worker task-notification, a send_to_worker prompt, a
// coordination status/guard note) AND bridges it live to the session hub via the
// bus. Interactive user messages already reach the hub from the send-queue worker
// (chat_stream publishHub KindUserMessage); these injected ones bypassed it, so a
// window watching the coordinator/worker rendered the assistant reply that
// followed WITHOUT the message it answered — it looked like a duplicate reply
// appearing out of nowhere, and only a page reload restored the real order
// (_Docs/58, _Docs/47). Returns the persisted message so callers can chain.
func (r *Runtime) recordInjectedUserNote(ctx context.Context, sessionID, origin, text string) (db.Message, error) {
	return r.recordInjectedUserMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      "user",
		Origin:    origin,
		Text:      text,
	})
}

// sessionAgentID resolves which agent owns a session, for attributing a note that
// session's agent wrote into ANOTHER session. It returns "" when there is nothing
// to credit (no session id, or the session is gone) — attribution is a display
// nicety, so a failed lookup must degrade to an unattributed note rather than
// fail the delivery that carries the actual work.
func (r *Runtime) sessionAgentID(ctx context.Context, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return ""
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return ""
	}
	return sess.AgentID
}

// recordAgentAuthoredNote persists a runtime-injected user-role turn that was
// WRITTEN BY AN AGENT — a spawn prompt, or a coordinator's follow-up to a worker
// — attributing it to that agent instead of leaving it an anonymous bubble
// (TSK507).
//
// Attribution is what makes the frontend render it as an incoming peer message
// (MessageList's isPeer → PeerTurn, keyed on role "user" + authorKind "agent" +
// authorId) rather than as the human's own turn, so a reader can tell at a glance
// that another agent said this. It is the same participant stamping a peer inbox
// delivery uses; only the delivery path differs.
//
// fromAgentID empty means there is no agent author to credit (a human-initiated
// spawn from the UI): the note stays a plain bubble, which is correct — inventing
// an author would misattribute a human's words to an agent.
//
// fromAgentID is also REQUIRED TO RESOLVE to a real agent before it is stamped.
// It is fed from SpawnOptions.CreatedBy, a provenance field that carries
// non-agent origins too ("automation:<id>" — see automation.go), and the frontend
// resolves authorId against the agent roster to name the sender. Stamping an id
// no agent owns would render an unnamed peer bubble, so an unresolvable author
// degrades to a plain note instead.
func (r *Runtime) recordAgentAuthoredNote(ctx context.Context, sessionID, fromAgentID, toAgentID, text string) (db.Message, error) {
	fromAgentID = strings.TrimSpace(fromAgentID)
	if fromAgentID == "" {
		return r.recordInjectedUserNote(ctx, sessionID, "", text)
	}
	if _, err := r.db.GetAgent(ctx, fromAgentID); err != nil {
		return r.recordInjectedUserNote(ctx, sessionID, "", text)
	}
	return r.recordInjectedUserMessage(ctx, db.Message{
		SessionID:   sessionID,
		Role:        "user",
		Text:        text,
		AuthorKind:  db.AuthorAgent,
		AuthorID:    fromAgentID,
		RecipientID: toAgentID,
	})
}

// recordInjectedUserMessage is the general form of recordInjectedUserNote for
// injected user turns that carry extra participant fields (a peer inbox delivery
// stamps AuthorKind/AuthorID/RecipientID; a spawn/flow opening prompt is a plain
// bubble). It persists m (Role should be "user") AND bridges it live to the hub.
// Returns the persisted message.
func (r *Runtime) recordInjectedUserMessage(ctx context.Context, m db.Message) (db.Message, error) {
	msg, err := r.db.AddMessage(ctx, m)
	if err != nil {
		return db.Message{}, err
	}
	r.emitInjectedUserNote(msg.SessionID, msg)
	return msg, nil
}

// emitInjectedUserNote broadcasts a just-persisted injected user message so the
// bus→hub bridge (bridgeBusToHub) can render it live on every window watching the
// session, mirroring how emitTurnStart / publishAutonomousReply cover the rest of
// an autonomous turn. No-op on an empty session id or a marshal error (best-effort
// live signal; the durable transcript backstops it on reload).
func (r *Runtime) emitInjectedUserNote(sessionID string, msg db.Message) {
	if sessionID == "" {
		return
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeSessionUserMessage,
		Target: map[string]string{"view": "chat", "sessionId": sessionID},
		Msg:    b,
	})
}

// emitWorkerStartEvent publishes a worker turn START so the coordination UI (the
// sidebar roster and the chat's running-worker banner) learns about a new worker
// the moment it begins, instead of polling for it.
//
// It carries phase="start", which the frontend MUST use to tell it apart from the
// completion event below: the completion branch drops the session's live ghost
// bubble, reloads the transcript and raises a desktop toast — all wrong for a turn
// that is only just beginning.
func (r *Runtime) emitWorkerStartEvent(agent db.Agent, workerSessionID, coordSessionID string) {
	target := map[string]string{
		"view":          "executions",
		"sessionId":     workerSessionID,
		"coordinatorId": coordSessionID,
		"phase":         "start",
	}
	r.tagRootCoordinator(target, coordSessionID)
	r.publish(events.Event{
		Type:   events.TypeWorker,
		Level:  "info",
		Title:  "🤖 Worker başladı — " + agent.Name,
		Target: target,
	})
}

// tagRootCoordinator adds "rootCoordinatorId" to a worker event's target when the
// direct coordinator is itself nested, i.e. the tree root is a DIFFERENT session.
//
// The frontend keys its running-worker banner on the coordinator id it is shown
// under, so a grandchild's transition tagged only with its direct (sub-)coordinator
// never reaches the root coordinator's open chat and its banner goes stale. The
// extra tag lets the root refetch on any transition below it.
//
// On a lookup failure the field is omitted rather than guessed: a wrong root would
// fan the refetch out to an unrelated chat, and the direct-coordinator tag still
// behaves exactly as before.
func (r *Runtime) tagRootCoordinator(target map[string]string, coordSessionID string) {
	sess, err := r.db.GetSession(context.Background(), coordSessionID)
	if err != nil {
		r.logger.Warn("coordination: root coordinator lookup failed", "coordinator", coordSessionID, "err", err)
		return
	}
	if root := sess.RootCoordinator(); root != "" && root != coordSessionID {
		target["rootCoordinatorId"] = root
	}
}

// emitWorkerEvent publishes a worker status transition so the coordination UI can
// live-update its worker cards.
func (r *Runtime) emitWorkerEvent(agent db.Agent, workerSessionID, coordSessionID, status string) {
	level := "success"
	if status == "failed" {
		level = "error"
	} else if status == "killed" {
		level = "info"
	}
	target := map[string]string{
		"view":          "executions",
		"sessionId":     workerSessionID,
		"coordinatorId": coordSessionID,
	}
	r.tagRootCoordinator(target, coordSessionID)
	r.publish(events.Event{
		Type:   events.TypeWorker,
		Level:  level,
		Title:  "🤖 Worker " + status + " — " + agent.Name,
		Target: target,
	})
}

// formatTaskNotification renders a worker outcome as the <task-notification> XML
// the coordinator reads (mirrors Claude Code's coordinator format). status is
// completed | timeout | incomplete | failed | killed; result is the worker's final
// text (for the truncated statuses, prefixed with a note saying so — see
// turnoutcome.go). Only "completed" means the worker finished its assignment.
func formatTaskNotification(workerSessionID, agentID, agentName, agentModel, status, result string, toolUses int, durationMs int64) string {
	var b strings.Builder
	b.WriteString("<task-notification>\n")
	b.WriteString("<source>runtime</source>\n")
	fmt.Fprintf(&b, "<task-id>%s</task-id>\n", html.EscapeString(workerSessionID))
	fmt.Fprintf(&b, "<agent-id>%s</agent-id>\n", html.EscapeString(agentID))
	fmt.Fprintf(&b, "<agent>%s</agent>\n", html.EscapeString(agentName))
	fmt.Fprintf(&b, "<model>%s</model>\n", html.EscapeString(agentModel))
	fmt.Fprintf(&b, "<status>%s</status>\n", html.EscapeString(status))
	fmt.Fprintf(&b, "<summary>%s</summary>\n", html.EscapeString(fmt.Sprintf("Worker %q %s", agentName, status)))
	if trimmed := strings.TrimSpace(result); trimmed != "" {
		// Defensive structural cap for EVERY caller (leaf-worker results are already
		// shaped by buildWorkerResult, but a sub-coordinator's report_to_coordinator
		// summary and the settle backstop's salvaged text arrive here uncapped). Cap
		// is rune-safe; overflow gets a short truncation notice.
		capped, _ := capText(trimmed, coordinatorResultCapChars)
		fmt.Fprintf(&b, "<result>%s</result>\n", html.EscapeString(capped))
	}
	b.WriteString("<usage>")
	fmt.Fprintf(&b, "<tool_uses>%d</tool_uses><duration_ms>%d</duration_ms>", toolUses, durationMs)
	b.WriteString("</usage>\n")
	b.WriteString("</task-notification>")
	return b.String()
}

// countToolSteps counts tool invocations in a turn trace (for the usage section).
func countToolSteps(steps []TurnStep) int {
	n := 0
	for _, s := range steps {
		if s.Kind == StepTool {
			n++
		}
	}
	return n
}

func turnToolNames(steps []TurnStep) []string {
	names := make([]string, 0, len(steps))
	for _, step := range steps {
		if step.Kind != StepTool {
			continue
		}
		if step.CallName != "" {
			names = append(names, step.CallName)
		} else {
			names = append(names, step.Tool)
		}
	}
	return names
}
