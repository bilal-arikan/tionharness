package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// reflectMaxJournals is how many recent journal entries feed one reflection.
const reflectMaxJournals = 20

// journalCap / journalMaxLen read the live, settings-driven journal bounds from
// Tunables (falling back to the built-in defaults when Tunables is absent, e.g.
// in tests). Each chat turn and task run appends a journal entry, so without
// these ceilings the memory store (and the Hafıza UI) grow without bound;
// durable knowledge (document/reflection) is never pruned.
func (r *Runtime) journalCap() int {
	if r.tun == nil {
		return DefaultJournalCap
	}
	return r.tun.JournalCap()
}

func (r *Runtime) journalMaxLen() int {
	if r.tun == nil {
		return DefaultJournalMaxLen
	}
	return r.tun.JournalMaxLen()
}

// reflectionCap reads the live cap on how many newest reflections an agent keeps.
// Reflections are otherwise durable (never consumed like journals), so without a
// cap a frequently-reflecting agent would accumulate them without bound.
func (r *Runtime) reflectionCap() int {
	if r.tun == nil {
		return DefaultReflectionCap
	}
	return r.tun.ReflectionCap()
}

// reflectPrompt instructs the agent to consolidate its journal into a durable
// self-reflection — the "dream cycle" that turns raw activity into learning.
// The journal entries are appended by Reflect, so this template carries no
// placeholder — keeping it safe for users to edit the workspace prompt file.
const reflectPrompt = `Below are your most recent journal entries. Write a brief first-person reflection (3-5 sentences) capturing what you have been doing, any patterns, preferences, or facts worth remembering long-term. Be concise and concrete. Do not invent details.`

// Journal records a memory of the given activity for an agent. Content is capped
// to journalMaxLen and old entries beyond journalCap are pruned, so the journal
// behaves as a bounded ring buffer. Failures are non-fatal to the caller's main
// flow and only logged.
func (r *Runtime) Journal(ctx context.Context, agentID, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if maxLen := r.journalMaxLen(); len([]rune(content)) > maxLen {
		content = string([]rune(content)[:maxLen]) + "…"
	}
	if _, err := r.mem.Remember(ctx, agentID, db.MemoryJournal, content); err != nil {
		r.logger.Warn("journal failed", "agent", agentID, "error", err)
		return
	}
	if _, err := r.mem.PruneKind(ctx, agentID, db.MemoryJournal, r.journalCap()); err != nil {
		r.logger.Warn("journal prune failed", "agent", agentID, "error", err)
	}
	r.maybeAutoReflect(agentID)
}

// maybeAutoReflect fires a background dream cycle when an agent's journal grows
// past the configured threshold. It runs asynchronously (so the caller's turn is
// never blocked), as an autonomous call (so it respects the global pause and the
// agent's daily budget), and is guarded against concurrent runs per agent. The
// reflection consumes the journals it consolidates, naturally pulling the count
// back below the threshold.
func (r *Runtime) maybeAutoReflect(agentID string) {
	if r.tun == nil || !r.tun.AutoReflect() {
		return
	}
	threshold := r.tun.AutoReflectThreshold()
	if r.tun.AutonomyPaused() || r.Paused() {
		return
	}
	// Count journals cheaply before committing to a goroutine.
	journals, err := r.mem.List(context.Background(), agentID, db.MemoryJournal)
	if err != nil || len(journals) < threshold {
		return
	}
	if _, busy := r.reflecting.LoadOrStore(agentID, true); busy {
		return
	}
	go func() {
		defer r.reflecting.Delete(agentID)
		if _, err := r.reflect(context.Background(), agentID, true); err != nil {
			r.logger.Warn("auto-reflect failed", "agent", agentID, "error", err)
			return
		}
		r.logger.Info("auto-reflect completed", "agent", agentID, "threshold", threshold)
	}()
}

// Reflect consolidates an agent's recent journal entries into a single
// reflection memory, written by the agent's own provider. This is the manual
// (user/API-triggered) entry point, so it is not budget-gated. Returns the
// stored reflection. Errors if there is nothing to reflect on.
func (r *Runtime) Reflect(ctx context.Context, agentID string) (db.KnowledgeSource, error) {
	return r.reflect(ctx, agentID, false)
}

// reflect is the shared dream-cycle core. autonomous=true enforces the global
// pause and the agent's daily budget (used by auto-reflect); false skips those
// gates (used by manual reflection).
func (r *Runtime) reflect(ctx context.Context, agentID string, autonomous bool) (db.KnowledgeSource, error) {
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
	consumed := make([]string, 0, len(journals))
	for _, j := range journals {
		consumed = append(consumed, j.ID)
	}

	var sb strings.Builder
	for _, j := range journals {
		sb.WriteString("- ")
		sb.WriteString(j.Content)
		sb.WriteString("\n")
	}

	// The editable reflect prompt is the instruction; the journal entries are
	// appended here so the workspace prompt file needs no format placeholder.
	userText := strings.TrimRight(r.readPrompt("reflect"), "\n") + "\n\nJournal:\n" + sb.String()

	// Usage is always recorded via guardedComplete; autonomous auto-reflects are
	// additionally gated on pause + daily budget.
	resp, err := r.guardedComplete(WithCallKind(ctx, KindReflect), agent, providers.Request{
		Model:  agent.Model,
		System: r.systemPrompt(agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: userText},
		},
	}, autonomous)
	if err != nil {
		return db.KnowledgeSource{}, err
	}

	reflection, err := r.mem.Remember(ctx, agentID, db.MemoryReflection, resp.Text)
	if err != nil {
		return db.KnowledgeSource{}, err
	}
	// HA-1: ride the dream cycle to refresh the agent's model of the user (the
	// "human" core block) from the same journals, before they are consumed below.
	// Best-effort and independently toggleable; never fails the reflection.
	if r.tun != nil && r.tun.UserModel() {
		r.updateUserModel(ctx, agent, sb.String(), autonomous)
	}
	// Dream cycle = compaction: the durable reflection now stands in for the raw
	// journals it consolidated, so discard them to keep the store bounded.
	if err := r.mem.DeleteIDs(ctx, consumed...); err != nil {
		r.logger.Warn("journal consume failed", "agent", agentID, "error", err)
	}
	// Reflections are durable (not consumed), so bound them too: keep only the
	// newest reflectionCap, pruning older ones. Otherwise frequent dream cycles
	// (low auto-reflect threshold) would grow the recall pool without limit.
	if pruned, err := r.mem.PruneKind(ctx, agentID, db.MemoryReflection, r.reflectionCap()); err != nil {
		r.logger.Warn("reflection prune failed", "agent", agentID, "error", err)
	} else if pruned > 0 {
		r.logger.Info("reflections pruned", "agent", agentID, "pruned", pruned)
	}
	r.logger.Info("agent reflected", "agent", agentID, "journals", len(journals))
	return reflection, nil
}
