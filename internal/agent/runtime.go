package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/logbuf"
	"github.com/bilal/swarmgo/internal/memory"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/secrets"
	"github.com/bilal/swarmgo/internal/skills"
)

// Runtime owns the lifecycle of all autonomous agent workers.
type Runtime struct {
	db        *db.DB
	providers *providers.Registry
	mem       *memory.Store
	tun       *Tunables
	logger    *slog.Logger

	// workDir is this workspace's sandbox root for built-in filesystem/shell
	// tools. Every fs/shell tool call is confined to it.
	workDir string

	// vault is this workspace's secret store, exposed to agents through the
	// secret_list / secret_get built-in tools. May be nil (no secret tools).
	vault *secrets.Vault

	// bus + workspace identity let autonomous events (heartbeat/task/schedule)
	// be published with enough context for the UI to deep-link on click.
	bus    *events.Bus
	wsID   string
	wsName string

	// logs is the process-wide ring buffer of captured log entries, exposed to
	// agents through the read_logs self-management tool. May be nil.
	logs *logbuf.Buffer

	// skills resolves reusable skill instruction sets (global/workspace/project
	// tiers) and backs the use_skill tool + the Available Skills prompt block.
	skills *skills.Store

	// reloadSched re-reads schedules into the cron scheduler after an agent
	// creates/edits/deletes one via a self-management tool. Wired by the
	// workspace manager once the scheduler exists; nil before then (no-op).
	reloadSched func(context.Context) error

	mu      sync.Mutex
	workers map[string]*worker

	// paused is this workspace's autonomy brake (set from per-workspace
	// settings); when true, autonomous calls are rejected like the global one.
	paused atomic.Bool

	// instructions is this workspace's free-form guidance (set from per-workspace
	// settings), appended to every agent's static system prompt.
	instructions atomic.Pointer[string]

	// Cross-session awareness config, set from per-workspace settings: whether the
	// feature is on (gates both the pushed context block and the list_sessions
	// pull tool), whether to inject every turn (vs only a session's first turn),
	// and how many past sessions to list.
	sessionCtxEnabled   atomic.Bool
	sessionCtxEveryTurn atomic.Bool
	sessionCtxRecent    atomic.Int64

	// reflecting guards against concurrent auto-reflects for the same agent: a
	// burst of journaled turns must not spawn overlapping dream cycles. Keyed by
	// agent id; presence means a reflection is in flight.
	reflecting sync.Map

	// activeSessions tracks sessions currently executing an autonomous invoke
	// (schedule / heartbeat). Keyed by session id; value is struct{}.
	// Used by the executions feed to show a live "running" indicator for
	// autonomous runs that aren't chat-streaming turns.
	activeSessions sync.Map
}

// trackSession marks a session as actively running an autonomous invoke.
func (r *Runtime) trackSession(id string) { r.activeSessions.Store(id, struct{}{}) }

// untrackSession removes the running marker when an invoke finishes.
func (r *Runtime) untrackSession(id string) { r.activeSessions.Delete(id) }

// ActiveSessionIDs returns the session ids currently running autonomous invokes.
func (r *Runtime) ActiveSessionIDs() []string {
	var ids []string
	r.activeSessions.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids
}

// SetPaused toggles this workspace's autonomy brake.
func (r *Runtime) SetPaused(p bool) { r.paused.Store(p) }

// Paused reports this workspace's autonomy brake state.
func (r *Runtime) Paused() bool { return r.paused.Load() }

// SetInstructions updates this workspace's agent-wide guidance.
func (r *Runtime) SetInstructions(s string) { r.instructions.Store(&s) }

// SetSessionContext updates this workspace's cross-session awareness config.
func (r *Runtime) SetSessionContext(enabled, everyTurn bool, recent int) {
	r.sessionCtxEnabled.Store(enabled)
	r.sessionCtxEveryTurn.Store(everyTurn)
	r.sessionCtxRecent.Store(int64(recent))
}

// SessionContextEnabled reports whether cross-session awareness is on for this
// workspace (gates the pushed block and the list_sessions tool).
func (r *Runtime) SessionContextEnabled() bool { return r.sessionCtxEnabled.Load() }

// SessionContextEveryTurn reports whether the block is injected every turn.
func (r *Runtime) SessionContextEveryTurn() bool { return r.sessionCtxEveryTurn.Load() }

// SessionContextRecentCount returns how many past sessions to list (default when unset).
func (r *Runtime) SessionContextRecentCount() int {
	if n := int(r.sessionCtxRecent.Load()); n > 0 {
		return n
	}
	return DefaultSessionContextRecent
}

// NewRuntime constructs the runtime. tun carries the process-wide tunables
// (autonomy pause, title-model override) shared across all workspace runtimes.
// workDir is the workspace sandbox root for built-in filesystem/shell tools.
// bus + wsID/wsName let autonomous events be published with workspace context
// (bus may be nil, in which case publishing is a no-op). vault is this
// workspace's secret store handed to the secret_* tools (may be nil).
func NewRuntime(database *db.DB, registry *providers.Registry, tun *Tunables, workDir string, vault *secrets.Vault, bus *events.Bus, wsID, wsName string, logs *logbuf.Buffer, logger *slog.Logger) *Runtime {
	// Seed the shipped default skills into the global dir (idempotent, never
	// overwrites) so every workspace inherits the SwarmGo guide skills.
	_ = skills.EnsureDefaults(globalSkillsDir())
	return &Runtime{
		db:        database,
		providers: registry,
		mem:       memory.New(database),
		tun:       tun,
		workDir:   workDir,
		vault:     vault,
		bus:       bus,
		wsID:      wsID,
		wsName:    wsName,
		logs:      logs,
		logger:    logger,
		skills:    skills.New(globalSkillsDir(), workspaceSkillsDir(workDir)),
		workers:   make(map[string]*worker),
	}
}

// globalSkillsDir is SwarmGo's data-dir-level global skills directory
// (<DataDir>/skills, default ~/.swarmgo/skills). Deliberately under SwarmGo's
// OWN data dir — not the cross-tool ~/.agents/skills convention — so SwarmGo's
// global skills stay isolated from other agent tools that share that directory.
// Honors SWARMGO_DATA_DIR so a custom data dir is respected (mirrors config).
func globalSkillsDir() string {
	if d := os.Getenv("SWARMGO_DATA_DIR"); d != "" {
		return filepath.Join(d, "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".swarmgo", "skills")
}

// workspaceSkillsDir is this workspace's skills directory (<workspace>/skills),
// a sibling of store/, config/ and workspace/. Empty when workDir is unknown.
func workspaceSkillsDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "skills")
}

// Skills returns this runtime's skill store (never nil after construction).
func (r *Runtime) Skills() *skills.Store { return r.skills }

// SkillsCatalogBlockForAgent renders the Available Skills system-prompt section
// an agent sees: its assigned skills (in the agent's chosen order) plus every
// shared (on-demand) skill. Returns "" when neither exists. Skills are a shared
// library; agents pick from it — they never own skills.
func (r *Runtime) SkillsCatalogBlockForAgent(agent db.Agent) string {
	if r.skills == nil {
		return ""
	}
	return r.skills.CatalogBlockForAgent(agent.Skills)
}

// agentSkillLib restricts the use_skill tool to an agent's selected slugs, so an
// agent cannot load a skill it has not been given.
type agentSkillLib struct {
	store *skills.Store
	allow map[string]bool
}

func (l agentSkillLib) Body(slug string) (string, error) {
	if !l.allow[slug] {
		return "", fmt.Errorf("skill %q is not enabled for this agent", slug)
	}
	return l.store.UseSkillBody(slug, l.allow)
}

// SetScheduleReloader wires the scheduler's Reload so self-management schedule
// tools take effect immediately. Called by the workspace manager after the
// scheduler is constructed.
func (r *Runtime) SetScheduleReloader(fn func(context.Context) error) { r.reloadSched = fn }

// reloadSchedules re-reads schedules into the cron scheduler (nil-safe).
func (r *Runtime) reloadSchedules(ctx context.Context) error {
	if r.reloadSched == nil {
		return nil
	}
	return r.reloadSched(ctx)
}

// Wake delay bounds (seconds): a wake fires no sooner than this many seconds and
// no later than the cap, mirroring the scheduler's own clamp.
const (
	MinWakeDelaySec = 5
	MaxWakeDelaySec = 3600
)

// ScheduleWake arms a one-shot self-wake: after delaySeconds the agent is
// re-invoked with prompt inside sessionID (the originating chat session), so the
// conversation continues on its own. It persists a one-shot schedule and reloads
// the scheduler to arm the timer. Returns a short confirmation for the tool.
// reason is the agent's stated purpose (stored for context, surfaced in events).
func (r *Runtime) ScheduleWake(ctx context.Context, sessionID, agentID, prompt, reason string, delaySeconds int) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("schedule_wake is only available during an interactive chat turn")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is required (what to do when you wake)")
	}
	if delaySeconds < MinWakeDelaySec {
		delaySeconds = MinWakeDelaySec
	}
	if delaySeconds > MaxWakeDelaySec {
		delaySeconds = MaxWakeDelaySec
	}
	fireAt := time.Now().Add(time.Duration(delaySeconds) * time.Second).Unix()
	sc, err := r.db.CreateSchedule(ctx, db.Schedule{
		AgentID:   agentID,
		Prompt:    prompt,
		Reason:    reason,
		SessionID: sessionID,
		OneShot:   true,
		FireAt:    fireAt,
		Enabled:   true,
		CreatedBy: agentID,
	})
	if err != nil {
		return "", fmt.Errorf("schedule wake: %w", err)
	}
	if err := r.reloadSchedules(ctx); err != nil {
		return "", fmt.Errorf("wake saved (%s) but arming failed: %w", sc.ID, err)
	}
	return fmt.Sprintf("Wake armed: in %ds I will continue this conversation on my own. Nothing more to do this turn.", delaySeconds), nil
}

// publish stamps the workspace identity onto an event and pushes it to the bus.
func (r *Runtime) publish(e events.Event) {
	e.WorkspaceID = r.wsID
	e.WorkspaceName = r.wsName
	r.bus.Publish(e) // nil-safe
}

// Emit publishes an event from outside the agent package (e.g. the API layer)
// with this workspace's identity stamped on it.
func (r *Runtime) Emit(e events.Event) { r.publish(e) }

// agentName resolves an agent's display name for event text, falling back to
// the id when the lookup fails.
func (r *Runtime) agentName(id string) string {
	if a, err := r.db.GetAgent(context.Background(), id); err == nil && a.Name != "" {
		return a.Name
	}
	return id
}

// emitHeartbeatFailure publishes a heartbeat failure (or auto-disable) event
// that deep-links to the logs view.
func (r *Runtime) emitHeartbeatFailure(agentID, errMsg string, disabled bool) {
	name := r.agentName(agentID)
	title := "Heartbeat hatası: " + name
	body := errMsg
	if disabled {
		title = "Ajan devre dışı: " + name
		body = "10 ardışık hatadan sonra otomatik durduruldu"
	}
	r.publish(events.Event{
		Type:   "heartbeat",
		Level:  "error",
		Title:  title,
		Body:   body,
		Target: map[string]string{"view": "logs", "agentId": agentID},
		Time:   time.Now().Unix(),
	})
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
	r.trackSession(session.ID)
	resp, err := r.CompleteWithTools(WithCallKind(ctx, KindHeartbeat), agent, provider, providers.Request{
		Model:  agent.Model,
		System: r.systemPrompt(agent),
		Messages: []providers.Message{
			{Role: providers.RoleUser, Text: agent.HeartbeatPrompt},
		},
	}, true)
	r.untrackSession(session.ID)
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

// buildSystemPrompt composes the agent's persona from soul + identity.
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

// systemPrompt builds an agent's static system prefix: its soul+identity persona
// followed by this workspace's instructions (when set). Both are stable, so they
// belong in the cached static prefix rather than the volatile dynamic suffix.
func (r *Runtime) systemPrompt(a db.Agent) string {
	out := buildSystemPrompt(a)
	if p := r.instructions.Load(); p != nil {
		if ins := strings.TrimSpace(*p); ins != "" {
			if out != "" {
				out += "\n\n"
			}
			out += "# Workspace Instructions\n" + ins
		}
	}
	return out
}
