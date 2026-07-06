package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
)

// Auto-tag: derive well-known session tags from what happened during a turn +
// the session's state, so an automation (or the user) can later scan for them —
// the intended flow is a tag-triggered automation that finds "tool-error"/"error"
// sessions and repairs them. Tagging is ADD-only here: a tag stays until a fixer
// removes it (via set_session_tags / the API), which is exactly the repair signal.
//
// These are the auto-assigned tag names (English, matching the code convention).
const (
	TagToolError = "tool-error" // a real tool call failed this turn
	TagError     = "error"      // the turn itself failed (provider/action error)
	TagAuthError = "auth-error" // the turn failed on authentication (login/token) — TERMINAL, not repairable
	TagGoal      = "goal"       // session has a persistent goal set
	TagGoalDone  = "goal-done"  // that goal is marked done
	TagArchived  = "archived"   // session is archived
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
	if sessionID == "" || !r.tun.AutoTagSessions() {
		return
	}
	sess, err := r.db.GetSession(ctx, sessionID)
	if err != nil {
		return
	}
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

	// Any REAL tool error → "tool-error". A claude-cli attempt at a disallowed tool
	// comes back as an is_error tool result too, but that is a policy denial, not a
	// tool failure — exclude it (the native path already models a denial as a
	// StepError(permission_denied), not a StepTool, so it never counts here).
	for _, st := range steps {
		if st.Kind == StepTool && st.IsError && !isPermissionDenyError(st) {
			add = append(add, TagToolError)
			break
		}
	}

	// State-derived tags (reconciled on every turn; add-only).
	if strings.TrimSpace(sess.Goal) != "" {
		add = append(add, TagGoal)
	}
	if sess.GoalDone {
		add = append(add, TagGoalDone)
	}
	if sess.State == "archived" {
		add = append(add, TagArchived)
	}

	r.addSessionTags(ctx, sess, add)
}

// AutoTagEnabled reports whether event-driven auto-tagging is on (settings gate).
// Exported so the api layer can gate its own out-of-turn tagging (archive) too.
func (r *Runtime) AutoTagEnabled() bool { return r.tun.AutoTagSessions() }

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
func (r *Runtime) addSessionTags(ctx context.Context, sess db.Session, add []string) {
	if len(add) == 0 {
		return
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
		return
	}
	if err := r.db.SetSessionTags(ctx, sess.ID, tags); err != nil {
		r.logger.Warn("auto-tag: persist failed", "session", sess.ID, "error", err)
		return
	}
	r.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": sess.ID},
	})
}

// permissionDenyMarkers are substrings that identify a policy denial (a disallowed
// or ungranted tool) rather than a genuine tool failure. Matched case-insensitively
// against a tool step's reason/output/text so the claude-cli disallowed-tool case
// is excluded from the "tool-error" tag.
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
