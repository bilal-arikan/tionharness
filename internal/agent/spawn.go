package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
)

// spawnTimeout bounds a single background spawn turn. Spawns may run longer
// agentic work than a scheduled prompt, so the window is generous.
const spawnTimeout = 10 * time.Minute

// SpawnOptions tunes a spawn. ModelOverride swaps just the model (the target
// agent's provider is always preserved); Title overrides the auto-generated one;
// CreatedBy records provenance (the spawning agent's id, or "" for user/API).
type SpawnOptions struct {
	ModelOverride string
	Title         string
	CreatedBy     string
}

// SpawnResult is what a spawn returns to its caller immediately — the new
// session's id (so a UI can deep-link it) and the resolved agent's name.
type SpawnResult struct {
	SessionID string
	AgentName string
}

// SpawnSession opens a NEW, independent session ("spawned" kind), records the
// prompt as the opening user turn, and runs the agent's turn in the background —
// fire-and-forget. It returns as soon as the session exists, without waiting for
// the turn to finish, so the caller (a UI button or the spawn_session tool) gets
// an immediate handle. This is the manual/agent-driven complement to the
// scheduler: same deliverPrompt shape, but detached and per-call independent.
//
// Guards: the global SpawnMaxConcurrent cap bounds how many spawned turns run at
// once (a spawn-storm brake); the turn is autonomous so each agent's daily budget
// still applies. resolveAgent is workspace-scoped, so a spawn can never reach
// across workspaces.
func (r *Runtime) SpawnSession(ctx context.Context, agentRef, prompt string, opts SpawnOptions) (SpawnResult, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return SpawnResult{}, fmt.Errorf("prompt is required to spawn a session")
	}
	agent, err := r.resolveAgent(ctx, agentRef)
	if err != nil {
		return SpawnResult{}, err
	}
	// Per-agent provider is preserved; only the model may be overridden.
	if m := strings.TrimSpace(opts.ModelOverride); m != "" {
		agent.Model = m
	}

	// Concurrency guard: refuse once the cap of simultaneously-running spawns is
	// reached. The slot is released when the background turn finishes.
	if !r.acquireSpawnSlot() {
		return SpawnResult{}, fmt.Errorf("spawn limit reached (%d concurrent spawned sessions); try again once some finish", r.tun.SpawnMaxConcurrent())
	}

	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "✨ " + spawnTitle(prompt)
	}

	// Each spawn is its own independent session — a fresh sourceID (not GetOrCreate)
	// so two spawns never collapse into one thread.
	session, err := r.db.CreateSession(ctx, db.Session{
		AgentID:  agent.ID,
		Kind:     "spawned",
		SourceID: "spawn:" + uuid.NewString(),
		Title:    title,
	})
	if err != nil {
		r.releaseSpawnSlot()
		return SpawnResult{}, err
	}

	// Record the spawn prompt as the opening user turn so the thread reads as a
	// real conversation in the activity feed.
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      "user",
		Text:      prompt,
	}); err != nil {
		r.releaseSpawnSlot()
		return SpawnResult{}, err
	}

	r.logger.Info("spawn: launched",
		"session", session.ID, "agent", agent.ID, "model", agent.Model, "by", opts.CreatedBy)

	// Fire-and-forget: run the turn detached from the caller's context so a closed
	// HTTP request or finished tool call can never cancel the spawn mid-flight.
	go r.runSpawn(agent, session.ID, prompt)

	return SpawnResult{SessionID: session.ID, AgentName: agent.Name}, nil
}

// runSpawn executes the background turn for a spawned session: it tracks the
// session live (so the executions feed shows a "running" indicator), runs the
// agent autonomously (daily budget enforced), records the reply (or the failure)
// as an assistant turn, then releases the concurrency slot and notifies.
func (r *Runtime) runSpawn(agent db.Agent, sessionID, prompt string) {
	defer r.releaseSpawnSlot()

	ctx, cancel := context.WithTimeout(context.Background(), spawnTimeout)
	defer cancel()

	r.trackSession(sessionID)
	output, steps, err := r.invokeTraced(WithCallKind(ctx, KindSpawn), agent, prompt, true)
	r.untrackSession(sessionID)

	if err != nil {
		r.logger.Error("spawn: agent invoke failed",
			"session", sessionID, "agent", agent.ID,
			"provider", agent.Provider, "model", agent.Model, "error", err)
		if _, addErr := r.db.AddMessage(ctx, db.Message{
			SessionID: sessionID,
			AgentID:   agent.ID,
			Role:      "assistant",
			Text:      "⚠️ Spawn turu çalıştırılamadı:\n\n" + err.Error(),
			Steps:     encodeSteps(steps),
		}); addErr != nil {
			r.logger.Warn("spawn: failed to record error reply", "session", sessionID, "error", addErr)
		}
		r.emitSpawnEvent(agent, sessionID, prompt, false)
		return
	}

	if strings.TrimSpace(output) == "" {
		output = "ℹ️ Ajan bu spawn için boş yanıt döndürdü."
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		AgentID:   agent.ID,
		Role:      "assistant",
		Text:      output,
		Steps:     encodeSteps(steps),
	}); err != nil {
		r.logger.Warn("spawn: failed to record reply", "session", sessionID, "error", err)
	}
	r.logger.Info("spawn: finished", "session", sessionID, "agent", agent.ID)
	r.emitSpawnEvent(agent, sessionID, prompt, true)
}

// emitSpawnEvent publishes a "spawned"-typed notification for a finished spawn,
// deep-linking to its transcript in the executions feed.
func (r *Runtime) emitSpawnEvent(agent db.Agent, sessionID, prompt string, ok bool) {
	level := "success"
	title := "✨ Spawn tamamlandı — " + agent.Name
	if !ok {
		level = "error"
		title = "✨ Spawn başarısız — " + agent.Name
	}
	r.publish(events.Event{
		Type:   "spawned",
		Level:  level,
		Title:  title,
		Body:   notifyLine(prompt, 120),
		Target: map[string]string{"view": "executions", "sessionId": sessionID},
	})
}

// acquireSpawnSlot reserves one of the bounded concurrency slots, returning false
// when the cap is already reached. Paired with releaseSpawnSlot.
func (r *Runtime) acquireSpawnSlot() bool {
	max := int64(r.tun.SpawnMaxConcurrent())
	if r.spawnActive.Add(1) > max {
		r.spawnActive.Add(-1)
		return false
	}
	return true
}

// releaseSpawnSlot frees a concurrency slot taken by acquireSpawnSlot.
func (r *Runtime) releaseSpawnSlot() { r.spawnActive.Add(-1) }

// spawnTitle derives a short, single-line title from a spawn prompt (rune-capped,
// so Turkish characters never get split). The auto-titler can refine it later.
func spawnTitle(prompt string) string {
	t := notifyLine(prompt, 60)
	if t == "" {
		return "Spawn"
	}
	return t
}
