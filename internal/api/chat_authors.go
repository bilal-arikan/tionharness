package api

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// labelMultiAgentHistory annotates a chat history so a responding agent can tell
// WHICH agent authored each prior assistant turn.
//
// In a multi-agent session (several agents reply in the same thread) the stored
// messages keep each author's AgentID, but the provider transcript collapses
// every assistant turn into one undifferentiated "assistant" voice — so an agent
// reading the history couldn't see that a *different* agent said something, and
// would even mistake another agent's words for its own. That is the cause of
// "agents can't see who sent each message" in a thread shared by two agents.
//
// When the history contains an assistant turn authored by an agent OTHER than
// the one now responding (a thread shared by 2+ agents, or a session handed over
// from agent A to agent B), every assistant turn's text is prefixed with its
// author's display name ("[Ada]: …"); the responding agent's own earlier turns
// get a "(you)" marker so it can still tell its own voice apart. Pure
// single-agent sessions (only the responder has ever spoken) are returned
// unchanged (natural transcript, no labels) so normal 1:1 chats and prompt
// caching are unaffected.
//
// It works on a COPY — the stored messages are never mutated — and returns
// whether the session is multi-author, so the caller can add a one-line system
// note explaining the bracket convention.
func (s *Server) labelMultiAgentHistory(ctx context.Context, database *db.DB, currentAgentID string, history []db.Message) ([]db.Message, bool) {
	// Distinct agents that authored an assistant turn so far.
	authors := make(map[string]struct{})
	for _, m := range history {
		if m.Role == providers.RoleAssistant && m.AgentID != "" {
			authors[m.AgentID] = struct{}{}
		}
	}
	// Attribution is needed whenever the history contains an assistant turn
	// authored by an agent OTHER than the one now responding — otherwise the
	// responder could mistake another agent's words for its own. This covers
	// both a thread shared by 2+ agents AND a handed-over session (a single
	// prior author A, now answered by B). A pure single-agent thread (only the
	// responder has ever spoken) stays unlabeled to keep the natural transcript
	// and prompt caching intact.
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

	out := make([]db.Message, len(history))
	for i, m := range history {
		switch {
		case m.Role == providers.RoleAssistant && m.AgentID != "":
			// Who authored this reply.
			tag := "[" + nameOf(m.AgentID) + "]"
			if m.AgentID == currentAgentID {
				tag = "[" + nameOf(m.AgentID) + " (you)]"
			}
			m.Text = tag + ": " + m.Text
		case m.Role == providers.RoleUser && m.AgentID != "":
			// Who this user message was directed at (the routed recipient agent).
			rcpt := nameOf(m.AgentID)
			if m.AgentID == currentAgentID {
				rcpt += " (you)"
			}
			m.Text = "[User → " + rcpt + "]: " + m.Text
		}
		out[i] = m
	}
	return out, true
}

// multiAgentHistoryNote is the system-prompt note added (only in a multi-author
// session) that explains the bracket attribution convention applied to the
// history, and tells the agent not to copy it into its own reply.
const multiAgentHistoryNote = "This conversation is shared by MULTIPLE agents. In the history each assistant turn is prefixed with its author in brackets — e.g. \"[Ada]: …\" for another agent, and \"[<your name> (you)]: …\" for your own earlier turns — and each user turn is prefixed with the agent it was directed at — e.g. \"[User → Ada]: …\". This lets you tell exactly who said what and who each question was meant for (the user may ask). The labelling is only a reading aid: do NOT imitate it — write your own reply as plain text with no prefix."
