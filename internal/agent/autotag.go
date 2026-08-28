package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

// Auto-tag: derive well-known session tags from what happened during a turn +
// the session's state, so an automation (or the user) can later scan for them —
// the intended flow is a tag-triggered automation that finds "tool-error"/"error"
// sessions and repairs them. Error/state tags stay until a fixer removes them;
// tool-error is reconciled each turn because a later clean turn proves recovery.
//
// These are the auto-assigned tag names (English, matching the code convention).
const (
	TagToolError = "tool-error" // a real tool call failed this turn
	TagError     = "error"      // the turn itself failed (provider/action error)
	TagAuthError = "auth-error" // the turn failed on authentication (login/token) — TERMINAL, not repairable
	TagArchived  = "archived"   // session is archived
	TagStuck     = "stuck"      // StuckTurns crossed the threshold — autonomous turns refused
	TagBlocked   = "blocked"    // coordinator drain ended with no runnable work
)

// AutoTagTurn inspects a finished turn (its trace steps + an optional turn-level
// error reason) plus the session's current state, and adds any derived tags. It
// is exported so the api chat path can call it; the in-package completion paths
// (spawn/schedule/wake) call it too. No-op when nothing new applies. Best-effort:
// a lookup/write failure is logged, never fatal.
//
// turnErr is the stable turn-failure reason ("" on success). A user-initiated
// "stopped" is NOT an error and is ignored.
func (r *Runtime) AutoTagTurn(ctx context.Context, sessionID string, steps []TurnStep, turnErr string) {
	if sessionID == "" {
		return
	}
	// Cross-cutting post-turn concern with its OWN gate (independent of tagging):
	// surface newly-detected warn-severity debug anomalies as desktop
	// notifications. Placed before the auto-tag gate so it runs even when tagging
	// is off — this is the one funnel every turn-completion path hits.
	r.notifyNewAnomalies(ctx, sessionID)

	if !r.tun.AutoTagSessions() {
		return
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return
	}

	// Hata→ders döngüsü: a badly-ended turn spawns a background reflection that
	// distills the failure into a stored lesson (own gate, independent of
	// tagging — see lessons.go). Placed here because this is the one funnel
	// every turn-completion path (chat/spawn/schedule/wake/auto-continue) hits.
	r.maybeReflectLessons(ctx, sessionID, steps, turnErr)

	var add []string

	// Turn-level failure → "error" (skip a clean user cancel).
	if e := strings.TrimSpace(turnErr); e != "" && e != "stopped" {
		add = append(add, TagError)
	}

	// An authentication failure (this claude-home is not logged in / token expired)
	// is TERMINAL — no amount of auto-repair can log the CLI in, so an "error"-tag
	// repair automation would loop in vain until MaxIterations/Cooldown. Tag it
	// distinctly so a repair automation can EXCLUDE auth-error sessions and a human
	// (or the fixer's soul) knows to run /login instead. The auth detail lives in an
	// error step's text (turnErr carries only the machine reason, e.g. provider_error).
	for _, st := range steps {
		if st.Kind == StepError && isAuthErrorText(st.Text) {
			add = append(add, TagAuthError)
			break
		}
	}

	// A material tool error marks this turn. Expected interaction exits and policy
	// denials are control flow, not repair signals. A later turn without a material
	// tool error clears the tag so a recovered session does not stay falsely broken.
	toolFailed := false
	for _, st := range steps {
		if isMaterialToolError(st) {
			add = append(add, TagToolError)
			toolFailed = true
			break
		}
	}

	// State-derived tags (reconciled on every turn; add-only).
	if sess.State == "archived" {
		add = append(add, TagArchived)
	}

	// Stuck-turn counter (self-healing Faz D): a bad turn (turn error or a
	// guardrail halt) increments the persistent per-session counter; a clean
	// turn resets it. At the threshold the session is tagged "stuck" and the
	// autonomous gate (completeTracedInner) refuses further unattended turns.
	// The gate's own refusal is excluded so a blocked session doesn't keep
	// counting itself deeper into the hole.
	if threshold := r.tun.StuckTurnThreshold(); threshold > 0 && !strings.Contains(turnErr, stuckGuardMarker) {
		bad := strings.TrimSpace(turnErr) != "" && turnErr != "stopped"
		if !bad {
			for _, st := range steps {
				if st.Kind == StepRecovery && st.Reason == string(termGuardrailHalt) {
					bad = true
					break
				}
			}
		}
		switch {
		case bad:
			n := sess.StuckTurns + 1
			if err := r.db.SetSessionStuckTurns(ctx, sess.ID, n); err != nil {
				r.logger.Warn("stuck counter: persist failed", "session", sess.ID, "error", err)
			} else if n >= threshold {
				add = append(add, TagStuck)
			}
		case sess.StuckTurns > 0:
			if err := r.db.SetSessionStuckTurns(ctx, sess.ID, 0); err != nil {
				r.logger.Warn("stuck counter: reset failed", "session", sess.ID, "error", err)
			}
		}
	}

	r.addSessionTags(ctx, sess, add)
	if !toolFailed {
		r.RemoveSessionTags(ctx, sess.ID, []string{TagToolError})
	}

	// Repair-automation dispatch: a FAILED turn signals the failed-turn hooks
	// (automation engine only) so an automation watching an error-class tag —
	// "stuck" above all — fires on the failing turn itself. Success turns keep
	// their existing FireTurnFinished call sites; the stuck gate's own refusal
	// is excluded (firing on it would re-dispatch for every refused wake).
	if e := strings.TrimSpace(turnErr); e != "" && e != "stopped" && !strings.Contains(e, stuckGuardMarker) {
		// Re-read so the hook sees the tags added just above (e.g. "stuck").
		r.FireTurnFailed(sess.ID, sess.AgentID, e)
	}
}

// AutoTagEnabled reports whether event-driven auto-tagging is on (settings gate).
// Exported so the api layer can gate its own out-of-turn tagging (archive) too.
func (r *Runtime) AutoTagEnabled() bool { return r.tun.AutoTagSessions() }

// notifyNewAnomalies raises a desktop notification for each newly-detected
// warn-severity debug anomaly in a session — the cache/tool/error coaches
// (low_cache_hit, cache_breaks, tool_failing, frequent_compaction, error_burst,
// tool_time_dominant). Advisory info-level findings (high_thinking, slow_turns,
// cache_break, tool_large_output) stay on the Debug card only, never as a toast.
//
// Publishing is unconditional (like task/flow/schedule events): the frontend
// gates the OS toast by the device-local "anomaly" notify-type + the master
// desktop-notifications toggle. Dedup is per (session, code) so a persistent
// anomaly notifies once, not every turn. Best-effort: a read failure is silent
// and never blocks the turn.
func (r *Runtime) notifyNewAnomalies(ctx context.Context, sessionID string) {
	sum, err := r.db.GetDebugSummary(ctx, sessionID)
	if err != nil {
		return
	}
	for _, a := range sum.Anomalies {
		if a.Severity != "warn" {
			continue
		}
		key := sessionID + "\x00" + a.Code
		if _, seen := r.anomalyNotified.LoadOrStore(key, struct{}{}); seen {
			continue
		}
		r.publish(events.Event{
			Type:  events.TypeAnomaly,
			Level: "info",
			Title: "Oturum uyarısı",
			Body:  a.Message,
			// view:"chat" deep-links to the session transcript (routeFromEvent maps
			// chat→sessionId); the Debug card lives in that view's side panel.
			Target: map[string]string{"view": "chat", "sessionId": sessionID},
		})
	}
}

// AddSessionTag is a small exported helper to add a single derived tag outside a
// turn (e.g. the moment a session is archived). No-op if already present.
func (r *Runtime) AddSessionTag(ctx context.Context, sessionID, tag string) {
	if sessionID == "" || tag == "" {
		return
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	r.addSessionTags(ctx, sess, []string{tag})
}

// addSessionTags unions add into the session's existing tags and persists only
// when something changed, then emits a "session" event for live UI refresh.
func (r *Runtime) addSessionTags(ctx context.Context, sess db.Session, add []string) bool {
	if len(add) == 0 {
		return false
	}
	tags := sess.Tags
	changed := false
	for _, t := range add {
		if !containsTag(tags, t) {
			tags = append(tags, t)
			changed = true
		}
	}
	if !changed {
		return false
	}
	if err := r.db.SetSessionTags(ctx, sess.ID, tags); err != nil {
		r.logger.Warn("auto-tag: persist failed", "session", sess.ID, "error", err)
		return false
	}
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": sess.ID},
	})
	return true
}

// markCoordinatorBlocked tags and notifies once when a coordinator drain has no
// live worker and no queued notification left. The persisted tag is the dedup key.
func (r *Runtime) markCoordinatorBlocked(ctx context.Context, sessionID string) {
	if !r.tun.AutoTagSessions() {
		return
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil || containsTag(sess.Tags, TagBlocked) {
		return
	}
	if !r.addSessionTags(ctx, sess, []string{TagBlocked}) {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeCoordination,
		Level:  "warn",
		Title:  "🧭 Koordinatör bloke oldu",
		Body:   "Koordinatör turu worker çalışmadan ve bekleyen iş bırakmadan sona erdi. Devam etmek için oturumu inceleyin veya manuel mesaj gönderin.",
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
	})
}

// RemoveSessionTags deletes each tag in drop from a session's tags, persisting and
// emitting a refresh event only when something changed — the complement of the
// add-only auto-tag path. The auto-repair flow uses it to clear an ERRORED (parent)
// session's tag once the spawned fixer completes: the fixer runs in its OWN session,
// so the current-session-scoped update_session tool can never reach the parent.
func (r *Runtime) RemoveSessionTags(ctx context.Context, sessionID string, drop []string) {
	if sessionID == "" || len(drop) == 0 {
		return
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
	dropSet := make(map[string]bool, len(drop))
	for _, t := range drop {
		dropSet[t] = true
	}
	kept := sess.Tags[:0:0]
	changed := false
	for _, t := range sess.Tags {
		if dropSet[t] {
			changed = true
			continue
		}
		kept = append(kept, t)
	}
	if !changed {
		return
	}
	if err := r.db.SetSessionTags(ctx, sessionID, kept); err != nil {
		r.logger.Warn("repair: clear parent tag failed", "session", sessionID, "error", err)
		return
	}
	// Removing the "stuck" tag means a fixer (or the user) resolved the session:
	// also reset the counter, otherwise the autonomous gate would keep refusing
	// turns and the session could never recover.
	if dropSet[TagStuck] {
		if err := r.db.SetSessionStuckTurns(ctx, sessionID, 0); err != nil {
			r.logger.Warn("stuck counter: reset on untag failed", "session", sessionID, "error", err)
		}
	}
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": sessionID},
	})
}

// permissionDenyMarkers are substrings that identify a policy denial (a disallowed
// or ungranted tool) rather than a genuine tool failure. Matched case-insensitively
// against a tool step's reason/output/text so the claude-cli disallowed-tool case is
// excluded from the "tool-error" tag — a policy refusal is not a bug to auto-repair.
var permissionDenyMarkers = []string{
	"permission_denied",
	"requested permissions",
	"haven't granted",
	"has not granted",
	"permission to use",
	"not allowed to use",
	"is not allowed",
	"isn't allowed",
	"tool is not permitted",
	"disallowed",
	// claude-cli rejection when the model calls a bridged tool by its BARE name
	// (e.g. `PowerShell`) instead of the allowlisted namespaced form
	// (`mcp__tionharness_interaction__PowerShell`): "No such tool available: X. X
	// exists but is not enabled in this context." The model immediately retries with
	// the correct name — a self-recovered mis-address, not a repairable failure, so
	// it must not get the "tool-error" tag / spawn an auto-repair. The marker itself
	// is shared with deadtool.go, which parses the tool name out of the SAME string
	// to auto-activate an on-demand TionHarness tool (WS20/SES79).
	deadToolMarker,
	"not enabled in this context",
}

// authErrorMarkers identify a turn that failed because the provider could not
// authenticate — this claude-home never ran /login, or its OAuth token / API key
// expired or was revoked. Matched case-insensitively against a StepError's text.
// Kept in sync with providers.isAuthErrorText (separate package, so duplicated).
var authErrorMarkers = []string{
	"authentication failed",
	"authentication_failed",
	"not logged in",
	"please run /login",
	"invalid api key",
	"invalid x-api-key",
	"oauth token has expired",
	"invalid bearer token",
}

// isAuthErrorText reports whether an error step's text signals an authentication
// failure (login/token), which is terminal — not repairable by an auto-repair turn.
func isAuthErrorText(s string) bool {
	s = strings.ToLower(s)
	for _, m := range authErrorMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// isPermissionDenyError reports whether an errored tool step is actually a policy
// denial (disallowed/ungranted tool) rather than a real tool failure.
func isPermissionDenyError(st TurnStep) bool {
	if st.Reason == "permission_denied" {
		return true
	}
	hay := strings.ToLower(st.Output + " " + st.Text)
	for _, m := range permissionDenyMarkers {
		if strings.Contains(hay, m) {
			return true
		}
	}
	return false
}

// isMaterialToolError excludes expected, non-fatal control-flow results. Prompt
// validation failures still count: only explicit cancellation/expiry outcomes
// from the interactive prompt family are benign.
func isMaterialToolError(st TurnStep) bool {
	if st.Kind != StepTool || !st.IsError || isPermissionDenyError(st) {
		return false
	}
	tool := st.Tool
	if i := strings.LastIndex(tool, "__"); i >= 0 {
		tool = tool[i+2:]
	}
	if tool != "ask_user" && tool != "request_confirmation" {
		return true
	}
	hay := strings.ToLower(st.Output + " " + st.Text)
	for _, marker := range []string{
		"no answer within the time limit",
		"turn ended before the user answered",
		"no interactive session is available",
		"user cancelled",
		"user canceled",
		"request cancelled",
		"request canceled",
	} {
		if strings.Contains(hay, marker) {
			return false
		}
	}
	return true
}
