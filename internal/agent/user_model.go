package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/memory"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// userModelPrompt drives HA-1: a Honcho-style pass that keeps the agent's "human"
// core block — its durable model of the user — current. The model merges any
// stable, specific facts about the USER from the recent journal into the existing
// profile, staying concise and dropping anything transient or about itself. The
// %s placeholders are (current profile, journal); both are supplied by the caller.
const userModelPrompt = `You maintain a durable profile of the USER you assist — their name, role, preferences, environment, and recurring context. Update the profile below by merging in any stable, specific facts about the USER revealed in the recent journal. Rules:
- Keep it concise: short "key: value" lines, no prose, deduplicated.
- Only durable facts about the USER. Drop anything transient, task-specific, or about yourself.
- If the journal reveals nothing new about the user, return the existing profile unchanged.
- Output ONLY the profile text, no preamble or commentary.

Current profile:
%s

Recent journal:
%s

Updated profile:`

// updateUserModel refreshes the agent's "human" core block from its recent
// journal (HA-1). It rides the dream cycle (called from reflect) rather than
// running on its own trigger, so it adds one cheap model call per reflection.
// Best-effort: every failure is logged and swallowed so it never breaks the
// reflection it piggybacks on. journalText is the same bullet list reflect built.
func (r *Runtime) updateUserModel(ctx context.Context, agent db.Agent, journalText string, autonomous bool) {
	current, err := r.mem.ReadCore(ctx, agent.ID, memory.CoreHuman)
	if err != nil {
		r.logger.Warn("user-model read failed", "agent", agent.ID, "error", err)
		return
	}
	existing := strings.TrimSpace(current)
	shown := existing
	if shown == "" {
		shown = "(empty)"
	}

	resp, err := r.guardedComplete(WithCallKind(ctx, KindReflect), agent, providers.Request{
		Model:  agent.Model,
		System: r.systemPrompt(agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: fmt.Sprintf(userModelPrompt, shown, journalText)},
		},
	}, autonomous)
	if err != nil {
		r.logger.Warn("user-model completion failed", "agent", agent.ID, "error", err)
		return
	}

	profile := strings.TrimSpace(resp.Text)
	// No-op when the model returns nothing new (empty or unchanged), so an
	// unchanged profile doesn't churn the store or its term vector.
	if profile == "" || profile == existing {
		return
	}
	if err := r.writeUserModel(ctx, agent.ID, profile); err != nil {
		r.logger.Warn("user-model write failed", "agent", agent.ID, "error", err)
		return
	}
	r.logger.Info("user model updated", "agent", agent.ID, "chars", len([]rune(profile)))
}

// writeUserModel writes the human block, truncating once to the block's limit if
// the model overran it (the prompt asks for concision, but enforce it anyway).
func (r *Runtime) writeUserModel(ctx context.Context, agentID, profile string) error {
	err := r.mem.WriteCore(ctx, agentID, memory.CoreHuman, profile)
	var full *memory.CoreBlockFullError
	if errors.As(err, &full) {
		return r.mem.WriteCore(ctx, agentID, memory.CoreHuman, truncateRunes(profile, full.Limit))
	}
	return err
}
