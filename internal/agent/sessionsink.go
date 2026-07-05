package agent

import (
	"context"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// sessionSink is a DB-backed tools.SessionSink bound to one session. It reads and
// writes the session's own fields — goal (the SAME db.Session.Goal the user
// edits), title, working directory and lifecycle state — so the session-scoped
// tools (set_session_goal/complete_goal, set_session_title, set_working_dir,
// archive_session) operate on the live session. Every mutation publishes a
// "session" change event so open windows refresh the session list + detail panel
// live (the agent-driven counterpart to the user editing these in the UI). Used
// on both tool paths (native via context, CLI via the Interaction bridge).
type sessionSink struct {
	db        *db.DB
	sessionID string
	// publish emits a workspace-scoped change event for live UI refresh. Optional
	// (nil-safe) — a turn with no runtime emitter just doesn't push the refresh.
	publish func(events.Event)
	// autoTag reports whether event-driven auto-tagging is on (settings gate), so
	// Archive only writes the "archived" tag when the feature is enabled.
	autoTag func() bool
}

// NewSessionSink builds a session sink bound to a session for this workspace. It
// satisfies both tools.SessionSink and (as a superset) tools.GoalSink.
func (r *Runtime) NewSessionSink(sessionID string) tools.SessionSink {
	return &sessionSink{db: r.db, sessionID: sessionID, publish: r.publish, autoTag: r.tun.AutoTagSessions}
}

// notify publishes a "session" change event so the open session list + detail
// panel refresh live when an agent mutates session metadata.
func (s *sessionSink) notify() {
	if s.publish == nil {
		return
	}
	s.publish(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": s.sessionID},
	})
}

func (s *sessionSink) Goal(ctx context.Context) (tools.GoalState, error) {
	sess, err := s.db.GetSession(ctx, s.sessionID)
	if err != nil {
		return tools.GoalState{}, err
	}
	return tools.GoalState{Text: sess.Goal, Done: sess.GoalDone}, nil
}

func (s *sessionSink) SetGoal(ctx context.Context, text string, done bool) error {
	// A cleared goal can't be "done" (mirrors the HTTP handler's guard).
	if text == "" {
		done = false
	}
	if err := s.db.SetSessionGoal(ctx, s.sessionID, text, done); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *sessionSink) SetTitle(ctx context.Context, title string) error {
	if err := s.db.SetSessionTitle(ctx, s.sessionID, title); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *sessionSink) SetWorkingDir(ctx context.Context, dir string) error {
	if err := s.db.SetSessionWorkingDir(ctx, s.sessionID, dir); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *sessionSink) Tags(ctx context.Context) ([]string, error) {
	sess, err := s.db.GetSession(ctx, s.sessionID)
	if err != nil {
		return nil, err
	}
	return sess.Tags, nil
}

func (s *sessionSink) SetTags(ctx context.Context, tags []string) error {
	if err := s.db.SetSessionTags(ctx, s.sessionID, tags); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *sessionSink) Archive(ctx context.Context) error {
	if err := s.db.SetSessionState(ctx, s.sessionID, "archived"); err != nil {
		return err
	}
	// Auto-tag "archived" so an automation can scan archived sessions (best-effort;
	// gated by the auto-tag setting).
	if s.autoTag != nil && s.autoTag() {
		if sess, err := s.db.GetSession(ctx, s.sessionID); err == nil && !containsTag(sess.Tags, TagArchived) {
			_ = s.db.SetSessionTags(ctx, s.sessionID, append(sess.Tags, TagArchived))
		}
	}
	s.notify()
	return nil
}
