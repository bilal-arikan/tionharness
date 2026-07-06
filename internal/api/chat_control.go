package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// chatRun is the live control handle for one in-flight streaming chat turn.
type chatRun struct {
	cancel context.CancelFunc
	steer  chan string
	// answer delivers a reply to a blocked ask_user tool call. Buffered (1) so
	// the control endpoint never blocks; only one question is outstanding at a
	// time because the tool loop runs synchronously.
	answer chan string
	// done is closed when the turn finishes (unregister), unblocking any
	// Interaction MCP tool call still waiting on this run.
	done chan struct{}
	// token is the per-run opaque secret a CLI subprocess presents (Bearer) so
	// its Interaction MCP calls correlate back to this turn.
	token string
	// autonomous marks a headless turn (scheduler/spawn) with no live
	// client: interactive Interaction MCP tools (ask_user/request_confirmation)
	// bail out immediately instead of blocking for an answer that can't arrive.
	autonomous bool
	// sessionID is the chat session this turn belongs to, so the UI can ask
	// "is a turn in flight for session X?" after a page reload (turns are
	// detached from the client connection and keep running server-side).
	sessionID string
	// startedAt stamps when this turn was registered, so the Session Info panel
	// can show how long the background process has been running. Set once at
	// register (read-only after) so no lock is needed to read it.
	startedAt time.Time

	// mu serialises SSE writes: the stream handler goroutine and the Interaction
	// MCP handler goroutine both emit steps onto the same ResponseWriter. It also
	// guards artifacts (swapped per responding agent in a multi-agent turn).
	mu          sync.Mutex
	provider    string                       // responding agent's provider id (e.g. "claude-cli"), for the Session Info panel
	write       func(event string, data any) // installed by the stream handler; nil once the turn ends
	artifacts   tools.ArtifactSink           // current agent's artifact sink, for Interaction MCP create/update
	notify      tools.NotifySink             // current agent's notify sink, for Interaction MCP notify (desktop notification)
	nav         tools.NavigateSink           // current agent's navigate sink, for Interaction MCP focus_view (UI navigation)
	session     tools.SessionSink            // current session's edit sink, for the Interaction MCP update_session tool (title/working-dir/goal/tags/archive)
	todos       tools.TodoSink               // current agent's todo sink, for Interaction MCP todo_write persistence
	grants      *tools.PermissionGrants      // session "Always allow" set, for the CLI permission-prompt tool
	wake        tools.WakeFunc               // current agent's self-wake scheduler, for the Interaction MCP schedule_wake tool
	spawn       *tools.SpawnSessionTool      // current agent's spawn tool (self-manage on), for the Interaction MCP spawn_session tool
	skill       skillLoader                  // current agent's skill loader, for the Interaction MCP use_skill tool
	skillSearch skillSearcher                // current agent's skill searcher, for the Interaction MCP skill_search tool
	skillAllow  skillAllowedFunc             // current agent's skill allowed-tools lookup, for use_skill auto-grant (SK-3)
	shell       shellRunner                  // current agent's shell runner, for the Interaction MCP shell tool
	runAgent    runAgentRunner               // current agent's run_subagent runner (delegation on), for the Interaction MCP run_subagent tool
	// bridge exposes the responding agent's lazy self-management tools to the CLI
	// path (CLI-3): bridgeDefs are advertised in tools/list + the allowlist, and
	// bridgeCall dispatches them through the native registry. Empty when
	// self-management is off. Installed per agent turn by the stream handler.
	bridgeDefs []providers.ToolDef
	bridgeCall func(ctx context.Context, name string, args json.RawMessage) (string, error)
	// tierVis reports the responding agent's effective per-tool visibility so the
	// Interaction MCP's tools/list (Tools) classifies each tool into the same CLI
	// wire tier (core/extended/hidden) that splitInteractionTiers used to build the
	// allowlist. Installed per agent turn alongside the bridge. nil → the static
	// split (used when no per-agent registry is available).
	tierVis func(name string) string
}

// setProvider records the responding agent's provider id so the Session Info
// panel can label the running background process (e.g. "claude-cli").
func (r *chatRun) setProvider(p string) {
	r.mu.Lock()
	r.provider = p
	r.mu.Unlock()
}

// providerOf returns the recorded provider id (empty if none installed yet).
func (r *chatRun) providerOf() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.provider
}

// setTierVis installs the responding agent's visibility resolver for the CLI wire
// tier classifier. Kept in lockstep with setBridge so tools/list and the allowlist
// classify identically.
func (r *chatRun) setTierVis(visOf func(name string) string) {
	r.mu.Lock()
	r.tierVis = visOf
	r.mu.Unlock()
}

// tierVisFor returns the installed visibility resolver (nil if none).
func (r *chatRun) tierVisFor() func(name string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tierVis
}

// setBridge installs the responding agent's self-management tool catalog +
// dispatcher for the Interaction MCP CLI bridge. nil/empty disables it.
func (r *chatRun) setBridge(defs []providers.ToolDef, call func(ctx context.Context, name string, args json.RawMessage) (string, error)) {
	r.mu.Lock()
	r.bridgeDefs = defs
	r.bridgeCall = call
	r.mu.Unlock()
}

// bridgeDefsFor returns the installed bridge tool defs (nil if none).
func (r *chatRun) bridgeDefsFor() []providers.ToolDef {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bridgeDefs
}

// bridgeCallFor returns the installed bridge dispatcher (nil if none).
func (r *chatRun) bridgeCallFor() func(ctx context.Context, name string, args json.RawMessage) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bridgeCall
}

// skillLoader loads a skill's full body by slug, enforcing the responding agent's
// allowlist. Installed per turn so the Interaction MCP use_skill tool (CLI path)
// mirrors the native use_skill built-in over the same skill store.
type skillLoader func(slug string) (string, error)

// setSkillLoader installs the per-agent skill loader so the Interaction MCP
// use_skill tool (CLI path) can load a skill body. A nil value disables it.
func (r *chatRun) setSkillLoader(fn skillLoader) {
	r.mu.Lock()
	r.skill = fn
	r.mu.Unlock()
}

// skillLoaderFor returns the current skill loader (nil if none installed).
func (r *chatRun) skillLoaderFor() skillLoader {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.skill
}

// skillSearcher finds skills the responding agent may load, for the Interaction
// MCP skill_search tool (CLI path) — mirrors the native skill_search built-in over
// the same store + per-agent allowlist. (SK-2)
type skillSearcher func(query string, limit int) []tools.SkillHit

// setSkillSearcher installs the per-agent skill searcher. A nil value disables it.
func (r *chatRun) setSkillSearcher(fn skillSearcher) {
	r.mu.Lock()
	r.skillSearch = fn
	r.mu.Unlock()
}

// skillSearcherFor returns the current skill searcher (nil if none installed).
func (r *chatRun) skillSearcherFor() skillSearcher {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.skillSearch
}

// skillAllowedFunc returns a skill's declared allowed-tools, for the Interaction
// MCP use_skill tool (CLI path) to auto-grant on load — SK-3 parity. (SK-3)
type skillAllowedFunc func(slug string) []string

// setSkillAllowed installs the per-agent skill allowed-tools lookup. nil disables.
func (r *chatRun) setSkillAllowed(fn skillAllowedFunc) {
	r.mu.Lock()
	r.skillAllow = fn
	r.mu.Unlock()
}

// skillAllowedFor returns the current skill allowed-tools lookup (nil if none).
func (r *chatRun) skillAllowedFor() skillAllowedFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.skillAllow
}

// shellRunner runs a shell command for the responding agent (CLI path), bound to
// the workspace sandbox. Mirrors the native shell built-in over the Interaction
// MCP bridge so a claude-cli agent runs commands through TionSwarm's own shell
// (PowerShell on Windows, sandboxed + bounded) instead of the CLI's POSIX Bash.
type shellRunner func(ctx context.Context, args json.RawMessage) (string, error)

// setShellRunner installs the per-agent shell runner so the Interaction MCP shell
// tool (CLI path) can run a command. A nil value disables it (shell off).
func (r *chatRun) setShellRunner(fn shellRunner) {
	r.mu.Lock()
	r.shell = fn
	r.mu.Unlock()
}

// shellRunnerFor returns the current shell runner (nil if none installed).
func (r *chatRun) shellRunnerFor() shellRunner {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shell
}

// runAgentRunner runs a run_subagent call for the responding agent (CLI path),
// returning the subagent's final result synchronously. Mirrors the native
// run_subagent built-in over the Interaction MCP bridge so a claude-cli agent can
// delegate a self-contained sub-task to another agent and get the answer back IN
// THIS turn — unlike spawn_session's fire-and-forget into a separate session.
type runAgentRunner func(ctx context.Context, args json.RawMessage) (string, error)

// setRunAgent installs the per-agent run_subagent runner so the Interaction MCP
// run_subagent tool (CLI path) can delegate. A nil value disables it (delegation off).
func (r *chatRun) setRunAgent(fn runAgentRunner) {
	r.mu.Lock()
	r.runAgent = fn
	r.mu.Unlock()
}

// runAgentFor returns the current run_subagent runner (nil if none installed).
func (r *chatRun) runAgentFor() runAgentRunner {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.runAgent
}

// setSpawnTool installs the per-agent spawn tool so the Interaction MCP
// spawn_session tool (CLI path) can launch independent sessions. A nil value
// disables it (self-manage off). A fresh instance per turn resets the per-turn
// spawn budget, mirroring the native path's per-turn tool instance.
func (r *chatRun) setSpawnTool(t *tools.SpawnSessionTool) {
	r.mu.Lock()
	r.spawn = t
	r.mu.Unlock()
}

// spawnTool returns the current spawn tool (nil if self-manage is off / none).
func (r *chatRun) spawnTool() *tools.SpawnSessionTool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spawn
}

// setWakeScheduler installs the self-wake scheduler for the currently responding
// agent so the Interaction MCP schedule_wake tool (CLI path) can arm a wake.
func (r *chatRun) setWakeScheduler(fn tools.WakeFunc) {
	r.mu.Lock()
	r.wake = fn
	r.mu.Unlock()
}

// wakeScheduler returns the current self-wake scheduler (nil if none installed).
func (r *chatRun) wakeScheduler() tools.WakeFunc {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wake
}

// setGrants installs the session's permission grants so the Interaction MCP
// permission-prompt tool can honour "Always allow" across turns.
func (r *chatRun) setGrants(g *tools.PermissionGrants) {
	r.mu.Lock()
	r.grants = g
	r.mu.Unlock()
}

// grantStore returns the session's permission grants (nil if none installed).
func (r *chatRun) grantStore() *tools.PermissionGrants {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.grants
}

// setArtifacts installs the artifact sink for the currently responding agent so
// the Interaction MCP create_artifact/update_artifact tools persist to it.
func (r *chatRun) setArtifacts(sink tools.ArtifactSink) {
	r.mu.Lock()
	r.artifacts = sink
	r.mu.Unlock()
}

// artifactSink returns the current artifact sink (nil if none installed).
func (r *chatRun) artifactSink() tools.ArtifactSink {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.artifacts
}

// setNotify installs the notify sink for the currently responding agent so the
// notify tool (native via context, CLI via the Interaction MCP) can raise a
// desktop notification stamped with this session + agent.
func (r *chatRun) setNotify(sink tools.NotifySink) {
	r.mu.Lock()
	r.notify = sink
	r.mu.Unlock()
}

// notifySink returns the current notify sink (nil if none installed).
func (r *chatRun) notifySink() tools.NotifySink {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.notify
}

// setNav installs the navigate sink for the currently responding agent so the
// focus_view tool (native via context, CLI via the Interaction MCP) can drive
// the UI to a view/entity.
func (r *chatRun) setNav(sink tools.NavigateSink) {
	r.mu.Lock()
	r.nav = sink
	r.mu.Unlock()
}

// navSink returns the current navigate sink (nil if none installed).
func (r *chatRun) navSink() tools.NavigateSink {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nav
}

// setSession installs the session edit sink so the update_session tool (native
// via context, CLI via the Interaction MCP) can mutate this session's title,
// working dir, goal, tags and archive state. SessionSink is a superset of
// GoalSink, so this single sink also backs the goal read/write.
func (r *chatRun) setSession(sink tools.SessionSink) {
	r.mu.Lock()
	r.session = sink
	r.mu.Unlock()
}

// sessionSinkFor returns the current session edit sink (nil if none installed).
func (r *chatRun) sessionSinkFor() tools.SessionSink {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.session
}

// setTodoSink installs the todo sink for the currently responding agent so the
// Interaction MCP todo_write tool (CLI path) persists the checklist to disk.
func (r *chatRun) setTodoSink(sink tools.TodoSink) {
	r.mu.Lock()
	r.todos = sink
	r.mu.Unlock()
}

// todoSink returns the current todo sink (nil if none installed).
func (r *chatRun) todoSink() tools.TodoSink {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.todos
}

// emit writes one SSE event through the run's writer under the lock, so the
// stream handler and the Interaction MCP server never race on the ResponseWriter.
// A no-op once the turn has ended (write cleared).
func (r *chatRun) emit(event string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.write != nil {
		r.write(event, data)
	}
}

// setWrite installs the SSE writer for this run.
func (r *chatRun) setWrite(fn func(event string, data any)) {
	r.mu.Lock()
	r.write = fn
	r.mu.Unlock()
}

// clearWrite drops the writer so late Interaction MCP emits are silently ignored.
func (r *chatRun) clearWrite() {
	r.mu.Lock()
	r.write = nil
	r.mu.Unlock()
}

// chatRuns is the registry of active streaming turns, keyed by run id, so the
// control endpoint can stop or steer a turn while it is running.
type chatRuns struct {
	mu   sync.Mutex
	runs map[string]*chatRun
	// secrets holds a STABLE Interaction MCP Bearer secret per (session,agent) — the
	// value a persistent claude-cli process presents across ALL its turns. Minted once,
	// reused, so the CLI mcp-config (which carries the token in an Authorization header)
	// stays byte-identical turn-to-turn → the persistent launch fingerprint does not
	// churn → the warm process is reused (Doc 52 §3-D, §11-decision 6). Keyed
	// "sessionID\x00agentID". A per-run uuid (the old scheme) changed every turn and
	// forced a cold restart of any persistent session with the Interaction MCP wired.
	secrets map[string]string
	// active maps a live Bearer secret → the run currently serving it, so byToken can
	// resolve a stable (reused) token to the single in-flight turn. Only one turn per
	// (session,agent) runs at a time (turns serialise), so this is unambiguous.
	active map[string]*chatRun
}

func newChatRuns() *chatRuns {
	return &chatRuns{
		runs:    make(map[string]*chatRun),
		secrets: make(map[string]string),
		active:  make(map[string]*chatRun),
	}
}

// interactionToken returns the STABLE Interaction MCP Bearer secret for a
// (session,agent) pair, minting one on first use. The same value is returned for
// every turn of that pair so the CLI mcp-config stays byte-identical and the
// persistent claude-cli process is not cold-restarted each turn (Doc 52 §3-D).
// Callers must also bindActive(token, run) for the turn so byToken can resolve it.
func (c *chatRuns) interactionToken(sessionID, agentID string) string {
	key := sessionID + "\x00" + agentID
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.secrets[key]; ok {
		return t
	}
	t := uuid.NewString()
	c.secrets[key] = t
	return t
}

// bindActive marks run as the turn currently serving token, so byToken resolves the
// stable (reused-across-turns) token to this in-flight run. Cleared in unregister.
func (c *chatRuns) bindActive(token string, run *chatRun) {
	if token == "" || run == nil {
		return
	}
	c.mu.Lock()
	c.active[token] = run
	c.mu.Unlock()
}

// register creates a control handle for a run (with a fresh per-run token) and
// returns it. sessionID ties the run to its chat session for activeSessionIDs.
func (c *chatRuns) register(id, sessionID string, cancel context.CancelFunc) *chatRun {
	run := &chatRun{
		cancel:    cancel,
		steer:     make(chan string, 16),
		answer:    make(chan string, 1),
		done:      make(chan struct{}),
		token:     uuid.NewString(),
		sessionID: sessionID,
		startedAt: time.Now(),
	}
	c.mu.Lock()
	c.runs[id] = run
	c.mu.Unlock()
	return run
}

// activeSessionIDs returns the distinct session ids that currently have a turn
// in flight. The frontend uses this after a reload to restore the "thinking"
// indicator for turns that are still running detached on the server.
func (c *chatRuns) activeSessionIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := make(map[string]struct{}, len(c.runs))
	ids := make([]string, 0, len(c.runs))
	for _, run := range c.runs {
		if run.sessionID == "" {
			continue
		}
		if _, ok := seen[run.sessionID]; ok {
			continue
		}
		seen[run.sessionID] = struct{}{}
		ids = append(ids, run.sessionID)
	}
	return ids
}

// runInfo is a snapshot of one in-flight turn, for the Session Info panel.
type runInfo struct {
	RunID      string
	StartedAt  time.Time
	Autonomous bool
	Provider   string
}

// sessionRunInfo returns a snapshot of the (first) in-flight turn for a session,
// or ok=false when the session has no running turn. Used by the Session Info panel
// to show + control the background process.
func (c *chatRuns) sessionRunInfo(sessionID string) (runInfo, bool) {
	if sessionID == "" {
		return runInfo{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, run := range c.runs {
		if run.sessionID != sessionID {
			continue
		}
		return runInfo{
			RunID:      id,
			StartedAt:  run.startedAt,
			Autonomous: run.autonomous,
			Provider:   run.providerOf(),
		}, true
	}
	return runInfo{}, false
}

func (c *chatRuns) unregister(id string) {
	c.mu.Lock()
	run := c.runs[id]
	delete(c.runs, id)
	// Drop any stable-token bindings that pointed at this run so a later Bearer call
	// (a CLI process turn that has since ended) no longer resolves to a dead turn.
	// The (session,agent) secret itself survives in c.secrets for the NEXT turn.
	if run != nil {
		for tok, r := range c.active {
			if r == run {
				delete(c.active, tok)
			}
		}
	}
	c.mu.Unlock()
	if run != nil {
		close(run.done)
	}
}

func (c *chatRuns) get(id string) *chatRun {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs[id]
}

// byToken resolves a run by its Interaction MCP Bearer token. It checks the stable
// per-(session,agent) binding first (the current scheme — one token reused across a
// persistent process's turns, resolved to the in-flight run via active), then falls
// back to matching a run's own per-run token (autonomous turns / any caller that has
// not bound a stable token).
func (c *chatRuns) byToken(token string) *chatRun {
	if token == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if run := c.active[token]; run != nil {
		return run
	}
	for _, run := range c.runs {
		if run.token == token {
			return run
		}
	}
	return nil
}

type chatControlReq struct {
	RunID  string `json:"runId"`
	Action string `json:"action"` // "stop" | "steer" | "answer"
	Text   string `json:"text"`
}

// handleChatControl stops, steers or answers an in-flight streaming turn. "stop"
// cancels the run's context (ending the stream); "steer" delivers live guidance
// the tool loop folds in before its next model call; "answer" delivers a reply
// to a blocked ask_user tool call.
func (s *Server) handleChatControl(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[chatControlReq](w, r)
	if !ok {
		return
	}
	run := s.runs.get(req.RunID)
	if run == nil {
		writeError(w, http.StatusNotFound, "run not found (already finished?)")
		return
	}
	switch req.Action {
	case "stop":
		run.cancel()
	case "steer":
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "steer text is required")
			return
		}
		select {
		case run.steer <- req.Text:
		default: // buffer full — drop rather than block the request
		}
	case "answer":
		if req.Text == "" {
			writeError(w, http.StatusBadRequest, "answer text is required")
			return
		}
		select {
		case run.answer <- req.Text:
		default: // no question waiting (or already answered) — drop
		}
	default:
		writeError(w, http.StatusBadRequest, "unknown action: "+req.Action)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"result": "ok"})
}

type cancelWakeReq struct {
	SessionID string `json:"sessionId"`
}

// handleCancelWake disarms a pending one-shot self-wake (schedule_wake) for a
// session — the user pressed "Durdur" on the waiting banner before the wake
// fired. It cancels the timer + deletes the schedule via the workspace runtime,
// which also emits a phase=cancelled event so the open screen clears the banner.
func (s *Server) handleCancelWake(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[cancelWakeReq](w, r)
	if !ok {
		return
	}
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "sessionId is required")
		return
	}
	cancelled, err := ws(r).Runtime.CancelWake(r.Context(), req.SessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": "ok", "cancelled": cancelled})
}
