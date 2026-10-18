package api

import (
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// Sidebar tabs (frontend/src/features/sessions/sessionGroup.ts). The sessions
// column splits into two tabs: "main" holds the conversations a person started
// (plus the chats one of them spawned or handed off to), "other" holds every
// machine-driven run — subagents, tasks, schedules,
// automations, insight scans and system work.
// Like the chips, the tab narrows
// the list BEFORE paging so "Daha fazla yükle" pages through the rows the
// selected tab actually shows.
const (
	sessionGroupMain  = "main"
	sessionGroupOther = "other"
)

// sessionGroup classifies a session into one of the two sidebar tabs. A session
// is "main" only when every axis says it is a human-facing conversation: a chat
// kind (spawned/handoff legacy kinds included), not a worker, not internal, not
// system-agent work, and a lineage of a person (user), a detached spawn from
// another conversation (spawn) or a context-reset continuation of one (handoff).
// A spawn_session child stays a conversation.
//
// Kind is checked rather than the chip key because new sessions of every kind
// are stamped Category "chat" by default (db.normalizeSessionMeta), which would
// file a task or insight transcript as a chat.
func sessionGroup(s db.Session) string {
	switch s.Kind {
	case "", "chat", "spawned":
	default:
		return sessionGroupOther
	}
	if s.Visibility == db.VisibilityInternal {
		return sessionGroupOther
	}
	switch s.Category {
	case "", db.CategoryChat:
	default:
		return sessionGroupOther
	}
	switch s.ExecutionType {
	case "", db.ExecutionInteractive:
	default:
		return sessionGroupOther
	}
	// A system agent's one-shot call is recorded as a user-origin chat; only its
	// tag tells it apart (agent.recordSystemAgentSession).
	for _, tag := range s.Tags {
		if strings.HasPrefix(tag, agent.SystemSessionTagPrefix) {
			return sessionGroupOther
		}
	}
	switch s.Lineage().Kind {
	case db.OriginUser, db.OriginSpawn, db.OriginHandoff:
		return sessionGroupMain
	}
	return sessionGroupOther
}

// parseSessionGroup validates the `group` query value; "" means both tabs.
func parseSessionGroup(raw string) (string, error) {
	switch raw {
	case "", sessionGroupMain, sessionGroupOther:
		return raw, nil
	}
	return "", fmt.Errorf("invalid group %q (want %q or %q)", raw, sessionGroupMain, sessionGroupOther)
}
