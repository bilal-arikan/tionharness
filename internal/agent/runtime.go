package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/memory"
	"github.com/bilal/swarmgo/internal/providers"
)

// Runtime owns the lifecycle of all autonomous agent workers.
type Runtime struct {
	db        *db.DB
	providers *providers.Registry
	mem       *memory.Store
	tun       *Tunables
	logger    *slog.Logger

	mu      sync.Mutex
	workers map[string]*worker

	// paused is this workspace's autonomy brake (set from per-workspace
	// settings); when true, autonomous calls are rejected like the global one.
	paused atomic.Bool
}

// SetPaused toggles this workspace's autonomy brake.
func (r *Runtime) SetPaused(p bool) { r.paused.Store(p) }

// Paused reports this workspace's autonomy brake state.
func (r *Runtime) Paused() bool { return r.paused.Load() }

// NewRuntime constructs the runtime. tun carries the process-wide tunables
// (autonomy pause, title-model override) shared across all workspace runtimes.
func NewRuntime(database *db.DB, registry *providers.Registry, tun *Tunables, logger *slog.Logger) *Runtime {
	return &Runtime{
		db:        database,
		providers: registry,
		mem:       memory.New(database),
		tun:       tun,
		logger:    logger,
		workers:   make(map[string]*worker),
	}
}

// Memory exposes the runtime's memory store for handlers in the same workspace.
func (r *Runtime) Memory() *memory.Store { return r.mem }

// StartConfigured starts workers for every agent with heartbeat enabled.
// Call once on boot.
func (r *Runtime) StartConfigured(ctx context.Context) error {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return err
	}
	for _, a := range agents {
		if a.HeartbeatEnabled {
			if err := r.Start(a.ID, a.HeartbeatIntervalSec); err != nil {
				r.logger.Warn("failed to start agent", "agent", a.ID, "error", err)
			}
		}
	}
	return nil
}

// Start launches a worker for the agent (idempotent).
func (r *Runtime) Start(agentID string, intervalSec int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.workers[agentID]; ok {
		return nil // already running
	}
	w := newWorker(agentID, r, intervalSec)
	r.workers[agentID] = w
	go w.run()
	r.logger.Info("agent started", "agent", agentID, "interval_sec", w.snapshot().IntervalSec)
	return nil
}

// Stop terminates the agent's worker (idempotent).
func (r *Runtime) Stop(agentID string) {
	r.mu.Lock()
	w, ok := r.workers[agentID]
	if ok {
		delete(r.workers, agentID)
	}
	r.mu.Unlock()

	if ok {
		w.stop()
		r.logger.Info("agent stopped", "agent", agentID)
	}
}

// Wake triggers an immediate tick for a running agent.
func (r *Runtime) Wake(agentID string) error {
	r.mu.Lock()
	w, ok := r.workers[agentID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("agent %s is not running", agentID)
	}
	w.wake()
	return nil
}

// Status returns a snapshot of all workers, sorted by agent id.
func (r *Runtime) Status() []WorkerStatus {
	r.mu.Lock()
	out := make([]WorkerStatus, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w.snapshot())
	}
	r.mu.Unlock()

	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}

// StopAll terminates every worker. Call on shutdown.
func (r *Runtime) StopAll() {
	r.mu.Lock()
	workers := make([]*worker, 0, len(r.workers))
	for id, w := range r.workers {
		workers = append(workers, w)
		delete(r.workers, id)
	}
	r.mu.Unlock()

	for _, w := range workers {
		w.stop()
	}
}

// runHeartbeat performs the agent's wake action: if a heartbeat prompt is set,
// it calls the provider and logs the reply into the agent's heartbeat session.
func (r *Runtime) runHeartbeat(ctx context.Context, agentID, trigger string) error {
	agent, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}

	// No prompt → nothing to do; a successful no-op heartbeat ("pulse").
	if agent.HeartbeatPrompt == "" {
		return nil
	}

	session, err := r.db.GetOrCreateHeartbeatSession(ctx, agentID)
	if err != nil {
		return err
	}

	provider, err := r.providers.Get(agent.Provider)
	if err != nil {
		return err
	}
	// Heartbeat is autonomous → enforce the agent's daily budget. Tools run when
	// the agent has them enabled.
	resp, err := r.CompleteWithTools(ctx, agent, provider, providers.Request{
		Model:  agent.Model,
		System: buildSystemPrompt(agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: agent.HeartbeatPrompt},
		},
	}, true)
	if err != nil {
		return err
	}

	_, err = r.db.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		Text:      fmt.Sprintf("[%s] %s", trigger, resp.Text),
	})
	return err
}

// buildSystemPrompt composes the agent's system prompt from soul + identity.
func buildSystemPrompt(a db.Agent) string {
	out := ""
	if a.Soul != "" {
		out = a.Soul
	}
	if a.Identity != "" {
		if out != "" {
			out += "\n\n"
		}
		out += a.Identity
	}
	return out
}
