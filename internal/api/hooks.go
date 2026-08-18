package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func (s *Server) handleListHooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := ws(r).DB.ListHooks(r.Context())
	if writeDBError(w, err, "") {
		return
	}
	if hooks == nil {
		hooks = []db.Hook{}
	}
	writeJSON(w, http.StatusOK, hooks)
}

// builtinHook describes one automatic, non-user-editable behaviour TionSwarm injects
// around the tool loop (freshness guard, CLI native-tool bridging, hook
// passthrough, ...). It is surfaced read-only in the Hooks screen so a user can see
// what runs implicitly. Enabled reflects the current setting for the toggleable
// ones; Setting names the settings.json key that controls it (empty = always on).
type builtinHook struct {
	Name        string `json:"name"`
	Scope       string `json:"scope"` // "native" | "cli" | "both"
	Event       string `json:"event"` // "PreToolUse" | "PostToolUse" | "System"
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Setting     string `json:"setting,omitempty"`
}

// handleListBuiltinHooks returns the read-only list of TionSwarm's auto-injected tool
// behaviours, computed from the current app settings. Purely informational — there
// is no create/update/delete counterpart.
func (s *Server) handleListBuiltinHooks(w http.ResponseWriter, _ *http.Request) {
	cur := s.settings.Get()
	list := []builtinHook{
		{
			Name:        "File freshness guard",
			Scope:       "native",
			Event:       "System",
			Setting:     "fileFreshnessGuard",
			Enabled:     cur.FileFreshnessGuard,
			Description: "Edit / Write / apply_patch refuse to modify a file unless it was Read this session and is unchanged since — prevents silently clobbering an out-of-band edit. Mirrors Claude Code; the claude-cli path enforces its own equivalent natively.",
		},
		{
			Name:        "CLI native-tool bridging",
			Scope:       "cli",
			Event:       "System",
			Enabled:     true,
			Description: "On the claude-cli path, native tools that can't be honoured headless are suppressed (--disallowedTools) and routed to TionSwarm equivalents: AskUserQuestion→ask_user, TodoWrite/Task*→todo_write, ScheduleWakeup→schedule_wake, Skill→use_skill, Task/Agent→run_subagent.",
		},
		{
			Name:        "Bash → PowerShell bridge",
			Scope:       "cli",
			Event:       "System",
			Setting:     "enableShell",
			Enabled:     cur.EnableShell,
			Description: "When the built-in shell is enabled, the CLI's native Bash is suppressed so shell commands route through TionSwarm's own Bash/PowerShell tool (correct Windows syntax + streaming + background shells).",
		},
		{
			Name:        "CLI hook passthrough",
			Scope:       "cli",
			Event:       "PreToolUse / PostToolUse",
			Setting:     "enableCliHooks",
			Enabled:     cur.EnableCLIHooks,
			Description: "Your PreToolUse/PostToolUse hooks below are forwarded to claude-cli agents via --settings, so the CLI's own tool loop fires the same hooks the native loop does. codex-cli has no equivalent passthrough: its turns never fire your hooks (not for codex's own shell/apply_patch, nor for TionSwarm tools called over the MCP bridge), so sqz/PostToolUse token-optimizer compression is also inactive there — see the provider catalog's appliesToolHooks flag.",
		},
		{
			Name:        "Permission deny-list (defense-in-depth)",
			Scope:       "cli",
			Event:       "System",
			Enabled:     true,
			Description: "The suppressed CLI tools are also written to permissions.deny in the per-turn CLI settings — a second barrier in case a --disallowedTools flag is ever ignored.",
		},
		{
			Name:        "Plan-mode approval bridge",
			Scope:       "cli",
			Event:       "System",
			Enabled:     true,
			Description: "In ask / read-only modes the CLI's ExitPlanMode is routed through TionSwarm's plan-approval card; in auto mode plan tools are disallowed so the agent just executes.",
		},
		{
			Name:        "Live steer bridge (Yönlendir)",
			Scope:       "cli",
			Event:       "PreToolUse",
			Enabled:     true,
			Description: "\"Yönlendir\" on a running claude-cli turn stashes the message and delivers it at the next tool boundary as the permission tool's additionalContext, redirecting the agent without restarting the turn (native providers use the steer channel instead). This works ONLY in \"ask\"/\"read-only\" modes, where the permission-prompt tool is wired; in \"auto\" mode the CLI runs with --dangerously-skip-permissions and never hits that boundary, so a steer cannot land — the control endpoint returns \"unsupported\" and the client queues the message instead. If a steerable turn makes no tool call, the message is queued as the next turn (steer_undelivered fallback).",
		},
		{
			Name:        "Autonomous git brake",
			Scope:       "native",
			Event:       "PreToolUse",
			Setting:     "autonomousConfine",
			Enabled:     cur.AutonomousConfine,
			Description: "On autonomous (confined) turns, shell commands that push to a git remote (git push / remote add / set-url) are blocked — a human-in-the-loop safeguard. Interactive chat is unaffected.",
		},
		{
			Name:        "Hook fail-open policy",
			Scope:       "both",
			Event:       "System",
			Enabled:     true,
			Description: "A hook that errors or times out never wedges a turn: the tool call proceeds (fail-open). Hook output is capped at 64KB and the timeout clamps to 30–120s.",
		},
	}
	writeJSON(w, http.StatusOK, list)
}

type hookReq struct {
	Event      string `json:"event"`
	Matcher    string `json:"matcher"`
	Type       string `json:"type"`
	Command    string `json:"command"`
	TimeoutSec int    `json:"timeoutSec"`
	Enabled    bool   `json:"enabled"`
}

// validHookEvent reports whether e is a supported hook event (tool or lifecycle).
func validHookEvent(e string) bool {
	return db.ValidHookEvent(e)
}

// hookEventError is the 400 message listing every accepted event.
const hookEventError = "event must be one of: PreToolUse, PostToolUse, UserPromptSubmit, SessionStart, Stop, SubagentStop, PreCompact, Notification, SessionEnd"

func (s *Server) handleCreateHook(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	req, ok := bindJSON[hookReq](w, r)
	if !ok {
		return
	}
	if !validHookEvent(req.Event) {
		writeError(w, http.StatusBadRequest, hookEventError)
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	hook, err := wsp.DB.CreateHook(r.Context(), db.Hook{
		Event:      req.Event,
		Matcher:    req.Matcher,
		Type:       "command",
		Command:    req.Command,
		TimeoutSec: req.TimeoutSec,
		Enabled:    req.Enabled,
	})
	if writeDBError(w, err, "") {
		return
	}
	s.logger.Info("hook created", "id", hook.ID, "event", hook.Event, "matcher", hook.Matcher)
	writeJSON(w, http.StatusCreated, hook)
}

func (s *Server) handleUpdateHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	req, ok := bindJSON[hookReq](w, r)
	if !ok {
		return
	}
	if !validHookEvent(req.Event) {
		writeError(w, http.StatusBadRequest, hookEventError)
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	err := wsp.DB.UpdateHook(r.Context(), db.Hook{
		ID:         id,
		Event:      req.Event,
		Matcher:    req.Matcher,
		Type:       "command",
		Command:    req.Command,
		TimeoutSec: req.TimeoutSec,
		Enabled:    req.Enabled,
	})
	if writeDBError(w, err, "hook not found") {
		return
	}
	h, err := wsp.DB.GetHook(r.Context(), id)
	if writeDBError(w, err, "hook not found") {
		return
	}
	s.logger.Info("hook updated", "id", id, "event", h.Event)
	writeJSON(w, http.StatusOK, h)
}

type toggleHookReq struct {
	Enabled bool `json:"enabled"`
}

func (s *Server) handleToggleHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	req, ok := bindJSON[toggleHookReq](w, r)
	if !ok {
		return
	}
	if err := wsp.DB.SetHookEnabled(r.Context(), id, req.Enabled); writeDBError(w, err, "hook not found") {
		return
	}
	s.logger.Info("hook toggled", "id", id, "enabled", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": req.Enabled})
}

func (s *Server) handleDeleteHook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	if err := wsp.DB.DeleteHook(r.Context(), id); writeDBError(w, err, "hook not found") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "result": "deleted"})
}
