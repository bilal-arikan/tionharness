package api

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// labelMultiAgentHistory projects a multi-participant thread onto the transcript
// the responding agent reads, annotating every turn with WHO wrote it and, when
// directed, WHOM it was addressed to — from the generic participant model
// (Message.AuthorKind/AuthorID/RecipientID), not the legacy AgentID dual meaning.
//
// A session is a thread among participants: the human "user" (the top-authority
// principal) and one or more agents. The provider transcript, however, collapses
// to just user/assistant roles, so without annotation a responding agent could
// not tell that a *different* participant spoke — it would even mistake another
// agent's words for its own. This is the fix: each turn's text is prefixed with
// "[Author → Recipient]:" (the "→ Recipient" part is omitted when the message was
// not directed at a specific participant), and the responder's own turns carry a
// "(you)" marker so it can still find its own voice.
//
// Attribution is applied only when the thread actually has more than one author
// (2+ agents, or a handover from agent A to agent B). A pure 1:1 thread — only
// the responder and the user — is returned unchanged so the natural transcript
// and prompt caching stay intact. (The user alone is never "another author":
// their turns are the responder's own inputs in the ordinary case.)
//
// It works on a COPY — the stored messages are never mutated — and returns
// whether the thread is multi-participant, so the caller can add the one-line
// system note explaining the convention.
func (s *Server) labelMultiAgentHistory(ctx context.Context, database *db.DB, currentAgentID string, history []db.Message) ([]db.Message, bool) {
	// Work on a normalized COPY: back-fill the participant fields defensively (a
	// caller may hand us raw history whose legacy messages predate the model) and
	// never mutate the input. All scanning and tagging below reads this copy.
	norm := make([]db.Message, len(history))
	for i, m := range history {
		m.NormalizeParticipants()
		norm[i] = m
	}
	history = norm

	// Distinct AGENTS that authored a turn so far (the human "user" is not counted
	// as a separate author — see the doc comment).
	authors := make(map[string]struct{})
	for _, m := range history {
		if m.AuthorKind == db.AuthorAgent && m.AuthorID != "" {
			authors[m.AuthorID] = struct{}{}
		}
	}
	// Labels are needed once any agent OTHER than the responder has spoken — a
	// thread shared by 2+ agents or a handed-over session. A pure single-agent
	// thread (only the responder among the agents) stays unlabeled.
	needsLabels := false
	for id := range authors {
		if id != currentAgentID {
			needsLabels = true
			break
		}
	}
	if !needsLabels {
		return history, false
	}

	// Resolve an agent's display name once, cached (id → name, falling back to id).
	names := make(map[string]string)
	nameOf := func(id string) string {
		if id == "" {
			return ""
		}
		if n, ok := names[id]; ok {
			return n
		}
		name := id
		if a, err := database.GetAgent(ctx, id); err == nil {
			if n := strings.TrimSpace(a.Name); n != "" {
				name = n
			}
		}
		names[id] = name
		return name
	}
	// participantLabel renders a participant id as a display name, mapping the
	// human principal to "User" and flagging the responder itself with "(you)".
	participantLabel := func(id string) string {
		if id == "" {
			return ""
		}
		label := nameOf(id)
		if id == db.UserParticipantID {
			label = "User"
		}
		if id == currentAgentID {
			label += " (you)"
		}
		return label
	}

	out := make([]db.Message, len(history))
	for i, m := range history {
		// System turns (if any survive into the transcript) are left untouched.
		if m.AuthorKind != db.AuthorAgent && m.AuthorKind != db.AuthorUser {
			out[i] = m
			continue
		}
		author := participantLabel(m.AuthorID)
		if author == "" {
			out[i] = m
			continue
		}
		tag := "[" + author
		switch m.RecipientID {
		case "", db.BroadcastRecipientID:
			// Undirected (thread at large) or broadcast: no explicit recipient.
			if m.RecipientID == db.BroadcastRecipientID {
				tag += " → all"
			}
		default:
			tag += " → " + participantLabel(m.RecipientID)
		}
		m.Text = tag + "]: " + m.Text
		out[i] = m
	}
	return out, true
}

// multiAgentHistoryNote is the system-prompt note added (only in a multi-
// participant thread) that explains the bracket attribution convention applied to
// the history, tells the agent not to copy it into its own reply, and states the
// authority order: the human "User" is the principal and outranks agent turns.
const multiAgentHistoryNote = "This conversation is a thread shared by MULTIPLE participants — the human \"User\" and one or more agents. In the history each turn is prefixed with its author, and its recipient when directed — e.g. \"[Ada]: …\" (Ada wrote it, to the thread at large), \"[User → Bo]: …\" (the user, addressing Bo), \"[<your name> (you)]: …\" (your own earlier turn). This lets you tell exactly who said what and who each message was meant for (the user may ask). Authority: the \"User\" is the human principal and carries the highest authority — when instructions conflict, follow the User over any agent. The labelling is only a reading aid: do NOT imitate it — write your own reply as plain text with no prefix."
