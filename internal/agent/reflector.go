package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// reflectMaxJournals is how many recent journal entries feed one reflection.
const reflectMaxJournals = 20

// reflectPrompt instructs the agent to consolidate its journal into a durable
// self-reflection — the "dream cycle" that turns raw activity into learning.
const reflectPrompt = `Below are your most recent journal entries. Write a brief first-person reflection (3-5 sentences) capturing what you have been doing, any patterns, preferences, or facts worth remembering long-term. Be concise and concrete. Do not invent details.

Journal:
%s`

// Journal records a memory of the given activity for an agent. Failures are
// non-fatal to the caller's main flow and only logged.
func (r *Runtime) Journal(ctx context.Context, agentID, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	if _, err := r.mem.Remember(ctx, agentID, db.MemoryJournal, content); err != nil {
		r.logger.Warn("journal failed", "agent", agentID, "error", err)
	}
}

// Reflect consolidates an agent's recent journal entries into a single
// reflection memory, written by the agent's own provider. Returns the stored
// reflection. Errors if there is nothing to reflect on.
func (r *Runtime) Reflect(ctx context.Context, agentID string) (db.KnowledgeSource, error) {
	agent, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return db.KnowledgeSource{}, err
	}

	journals, err := r.mem.List(ctx, agentID, db.MemoryJournal)
	if err != nil {
		return db.KnowledgeSource{}, err
	}
	if len(journals) == 0 {
		return db.KnowledgeSource{}, errors.New("no journal entries to reflect on")
	}
	if len(journals) > reflectMaxJournals {
		journals = journals[:reflectMaxJournals]
	}

	var sb strings.Builder
	for _, j := range journals {
		sb.WriteString("- ")
		sb.WriteString(j.Content)
		sb.WriteString("\n")
	}

	// Reflection is user-triggered (API), so it is not budget-gated; usage is
	// still recorded via guardedComplete.
	resp, err := r.guardedComplete(ctx, agent, providers.Request{
		Model:  agent.Model,
		System: r.systemPrompt(agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: fmt.Sprintf(reflectPrompt, sb.String())},
		},
	}, false)
	if err != nil {
		return db.KnowledgeSource{}, err
	}

	reflection, err := r.mem.Remember(ctx, agentID, db.MemoryReflection, resp.Text)
	if err != nil {
		return db.KnowledgeSource{}, err
	}
	r.logger.Info("agent reflected", "agent", agentID, "journals", len(journals))
	return reflection, nil
}
