package agent

import (
	"context"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// One-shot system-agent calls that the USER triggers from a screen (goal
// writer, workspace evolver, recipe optimizer) used to leave no trace: the
// prompt went out, the JSON came back, the code applied it. The exchange is now
// recorded as an ordinary chat session bound to the system agent, so the user
// can open it from the screen, read exactly what the agent saw and answered,
// and CONTINUE the conversation there (the session is a plain writable chat and
// the agent keeps its soul).
//
// High-frequency background roles (titler, compaction, stall judge, lesson
// extractor) are deliberately NOT recorded: they would flood the session list.

// SystemSessionTagPrefix tags a recorded exchange with its role ("system:goal-writer").
const SystemSessionTagPrefix = "system:"

// systemSessionTitleMax bounds the auto-title taken from the user's request.
const systemSessionTitleMax = 72

// recordSystemAgentSession stores prompt + reply as a chat session owned by the
// workspace agent serving `key` (the caller's agent when the role resolves to
// the compiled fallback, which has no row). Returns the session id, or "" when
// nothing could be stored — recording is best-effort and never fails the call.
func (r *Runtime) recordSystemAgentSession(ctx context.Context, key string, caller db.Agent, title, userText, reply string) string {
	if r == nil || r.db == nil {
		return ""
	}
	agentID := caller.ID
	if sa, _, err := r.ResolveSystemAgent(key); err == nil && sa.ID != "" {
		agentID = sa.ID
	}
	if agentID == "" {
		return ""
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = key
	}
	if rs := []rune(title); len(rs) > systemSessionTitleMax {
		title = strings.TrimSpace(string(rs[:systemSessionTitleMax])) + "…"
	}
	sess, err := r.db.CreateSession(ctx, db.Session{
		AgentID:    agentID,
		Kind:       "chat",
		Origin:     &db.SessionOrigin{Kind: db.OriginUser},
		Title:      title,
		Tags:       []string{SystemSessionTagPrefix + key},
		WorkingDir: r.effectiveWorkDir(ctx),
	})
	if err != nil {
		r.logger.Warn("system agent session: create failed", "systemKey", key, "error", err)
		return ""
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "user", Text: userText,
		AuthorKind: db.AuthorUser, AuthorID: db.UserParticipantID, RecipientID: agentID,
	}); err != nil {
		r.logger.Warn("system agent session: user message failed", "session", sess.ID, "error", err)
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "assistant", Text: reply, AgentID: agentID,
		AuthorKind: db.AuthorAgent, AuthorID: agentID,
	}); err != nil {
		r.logger.Warn("system agent session: reply message failed", "session", sess.ID, "error", err)
	}
	return sess.ID
}
