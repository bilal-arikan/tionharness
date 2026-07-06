// Package api exposes the TionSwarm HTTP/JSON interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/backup"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/interaction"
	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/market"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/settings"
	"github.com/bilal-arikan/tionswarm/internal/web"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// ctxKey is the private type for request-context values.
type ctxKey string

const workspaceCtxKey ctxKey = "workspace"

// Server holds dependencies for HTTP handlers.
type Server struct {
	workspaces *workspace.Manager
	providers  *providers.Registry
	convo      *conversation.Manager
	settings   *settings.Store
	tun        *agent.Tunables
	logs       *logbuf.Buffer
	bus        *events.Bus     // autonomous notifications streamed to the UI over SSE
	runs       *chatRuns       // in-flight streaming turns (stop/steer control)
	grants     *permGrantStore // per-session "Always allow" permission grants
	logger     *slog.Logger

	// market is a workspace-independent market store (bundled + global tiers),
	// used by the workspace-template picker and create-from-template seeding so
	// they work even with zero workspaces (onboarding). Per-workspace runtime
	// markets (with install ledgers) are used for in-workspace install/publish.
	market *market.Store

	// selfURL is this server's own loopback base URL (e.g. http://127.0.0.1:8090),
	// used to point CLI subprocesses at the in-process Interaction MCP endpoint.
	selfURL string
	// interactionMCP serves the Interaction MCP endpoint (/mcp/interaction).
	interactionMCP http.Handler
	// backups runs the periodic workspace-backup loop; reconfigured on every
	// settings change. nil until wired by SetBackupManager (after construction).
	backups *backup.Manager
}

// NewServer constructs an API server and pushes the persisted settings into the
// live subsystems (providers, compaction, autonomy). tun is the shared
// process-wide tunables updated whenever settings change; bus is the
// process-wide event bus exposed via the /api/events SSE feed.
func NewServer(manager *workspace.Manager, registry *providers.Registry, store *settings.Store, tun *agent.Tunables, logs *logbuf.Buffer, bus *events.Bus, logger *slog.Logger) *Server {
	s := &Server{
		workspaces: manager,
		providers:  registry,
		convo:      conversation.NewManager(),
		settings:   store,
		tun:        tun,
		logs:       logs,
		bus:        bus,
		runs:       newChatRuns(),
		grants:     newPermGrantStore(),
		logger:     logger,
		// Workspace-independent market store (bundled + global tiers) for the
		// workspace-template picker, which must work with zero workspaces during
		// onboarding. No ledger dir: install-status tracking is per-workspace.
		market: market.New(agent.MarketGlobalDir(), ""),
	}
	// Interaction MCP: lets CLI agents (claude-cli, ...) reach TionSwarm's
	// human-in-the-loop tools over in-process HTTP. See _Docs/11-INTERACTION-MCP.md.
	s.interactionMCP = interaction.Handler(&interactionBackend{runs: s.runs, tun: tun}, logger)
	// Headless Interaction MCP: give autonomous (scheduler/spawn) CLI
	// turns the same use_skill/shell/self-manage bridge chat turns get.
	manager.SetAutonomousInteraction(s.autonomousInteraction)
	// History-aware self-wake: let schedule_wake continue with the full
	// conversation (composed like a chat turn) instead of just the wake prompt.
	manager.SetWakeTurnRunner(s.wakeTurnRunner)
	// Surface compaction (rolling-summary fold + manual /compact) in the in-app
	// Logs screen; it was previously visible only in the chat response payload.
	s.convo.SetLogger(logger)
	s.applySettings()
	return s
}

// SetBaseURL records this server's own loopback base URL (e.g.
// http://127.0.0.1:8090) so the Interaction MCP endpoint can be advertised to
// CLI subprocesses. Pass the listen address; a wildcard/empty host is normalised
// to 127.0.0.1 so a same-machine subprocess can connect.
func (s *Server) SetBaseURL(addr string) {
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		// No colon: treat the whole thing as a host with the default port.
		host, port = addr, "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "[::]" || host == "::" {
		host = "127.0.0.1"
	}
	s.selfURL = "http://" + host + ":" + port
}

// applySettings pushes the current settings into every live subsystem. Called
// on boot and after each successful settings update.
func (s *Server) applySettings() {
	cur := s.settings.Get()
	s.providers.SetAnthropicKey(s.settings.AnthropicKey())
	s.providers.SetClaudeCLIPath(cur.ClaudeCLIPath)
	s.providers.SetClaudeConfigDir(cur.ClaudeConfigDir)
	s.providers.SetClaudeAuth(s.settings.ClaudeCliAuthToken(), cur.ClaudeCliAuthKind)
	s.providers.SetDefaultModel(cur.DefaultModel)
	s.providers.SetAnthropicBetas(cur.ExtendedPromptCache, cur.AnthropicContextEditing)
	s.providers.SetMinimax(s.settings.MinimaxKey(), cur.MinimaxBaseURL)
	s.providers.SetOpenRouter(s.settings.OpenRouterKey(), cur.OpenRouterBaseURL)
	s.providers.SetCustomProviders(s.customProviderSpecs(cur))
	s.convo.SetLimits(cur.MaxContextTokens, cur.KeepRecentMsgs)
	s.convo.SetBudgetShape(cur.ContextBudgetFraction, cur.ContextBudgetCeil) // model-aware budget knobs
	s.tun.SetContextBudget(cur.MaxContextTokens)                             // scale tool-output thresholds to the budget (CG-9)
	s.tun.SetTitleModel(cur.TitleModel)
	s.tun.SetHandoff(cur.HandoffAuto, cur.HandoffPressure, cur.HandoffMaxChain, cur.HandoffWriteFile)
	s.tun.SetProgress(cur.ProgressPersist, cur.ProgressResume)
	s.tun.SetAutoContinue(cur.AutonomousAutoContinue, cur.AutonomousAutoContinueMax)
	s.tun.SetFileFreshnessGuard(cur.FileFreshnessGuard)
	s.tun.SetAutoTagSessions(cur.AutoTagSessions)
	s.tun.SetDebugJournal(cur.DebugJournalEnabled, cur.DebugJournalCap)
	s.tun.SetShellEnabled(cur.EnableShell)
	s.tun.SetCLIHooksEnabled(cur.EnableCLIHooks)
	s.tun.SetCodeMode(cur.EnableCodeMode)
	s.tun.SetClaudePersistentSession(cur.ClaudePersistentSession)
	s.tun.SetClaudeSysPromptFile(cur.ClaudeSysPromptFile)
	s.tun.SetDelegationLimits(cur.DelegationMaxDepth, cur.DelegationMaxCalls)
	s.tun.SetSpawnLimits(cur.SpawnMaxConcurrent, cur.SpawnMaxPerTurn)
	s.tun.SetCoordinatorLimits(cur.CoordinatorMaxWorkers, cur.CoordinatorMaxTurns)
	s.tun.SetWorkdirGuards(cur.AutonomousConfine, cur.GitWorktreeIsolation, cur.AutonomousBootSeq)
	s.tun.SetRecoveryLimits(cur.ReactiveCompact, cur.MaxTokenRetries, cur.ReactiveKeepRecent)
	s.tun.SetMaxOutputTokens(cur.MaxOutputTokens)
	s.tun.SetToolCompaction(cur.CompactToolOutput, cur.CompactMaxLines, cur.CompactMaxBytes, cur.CompactLLMSummary, cur.CompactLLMThreshold, cur.CompactModel)
	if s.backups != nil {
		s.backups.Configure(backup.Config{
			Enabled:       cur.BackupEnabled,
			IntervalHours: cur.BackupIntervalHours,
			Retain:        cur.BackupRetain,
			Dir:           cur.BackupDir,
		})
	}
}

// SetBackupManager wires the process-wide backup manager and immediately pushes
// the current settings into it. Called once at startup, after NewServer (so the
// manager can be constructed with the workspace lister the server holds).
func (s *Server) SetBackupManager(b *backup.Manager) {
	s.backups = b
	s.applySettings()
}

// Routes registers all HTTP routes and returns the handler. Registration is
// split into per-domain helpers (each lives beside its handlers) so the route
// table stays readable as the surface grows.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/version", s.handleVersion)

	s.registerWorkspaceRoutes(mux)
	s.registerAgentRoutes(mux)
	s.registerSessionRoutes(mux)
	s.registerChatRoutes(mux)
	s.registerTaskRoutes(mux)
	s.registerScheduleRoutes(mux)
	s.registerUsageRoutes(mux)
	s.registerMCPRoutes(mux)
	s.registerHookRoutes(mux)
	s.registerFlowRoutes(mux)
	s.registerExecutionRoutes(mux)
	s.registerArtifactRoutes(mux)
	s.registerSkillRoutes(mux)
	s.registerMarketRoutes(mux)
	s.registerIngestRoutes(mux)
	s.registerSettingsRoutes(mux)
	s.registerGraphRoutes(mux)
	s.registerSecretRoutes(mux)
	s.registerMiscRoutes(mux)
	s.registerWebRoutes(mux)

	// withRecover sits inside withRequestLog so a recovered panic's 500 is also
	// reflected in the access log, and outside withWorkspace so a panic in any
	// workspace-scoped handler is caught.
	return withCORS(s.withRequestLog(s.withRecover(s.withWorkspace(mux))))
}

// registerWebRoutes mounts the embedded frontend SPA at the catch-all "/" route.
// More specific patterns (/api/*, /health, /mcp/*) take precedence under Go's
// method+path mux, so this only handles UI navigation and static assets. When no
// frontend build was bundled, it logs a hint and leaves "/" unhandled (404),
// which is the expected state for backend-only or dev (Vite proxy) workflows.
func (s *Server) registerWebRoutes(mux *http.ServeMux) {
	h, ok := web.Handler()
	if !ok {
		if s.logger != nil {
			s.logger.Info("frontend not bundled; UI not served by this binary (run 'npm run build' then rebuild, or use the Vite dev server)")
		}
		return
	}
	mux.Handle("/", h)
}

// registerWorkspaceRoutes registers workspace management (not workspace-scoped).
func (s *Server) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.handleCreateWorkspace)
	// Adopt an existing on-disk workspace data dir (first-run "select workspace").
	mux.HandleFunc("POST /api/workspaces/attach", s.handleAttachWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{id}", s.handleDeleteWorkspace)
	// Workspace templates catalog (agents/flow blueprints for new workspaces).
	mux.HandleFunc("GET /api/workspace-templates", s.handleListWorkspaceTemplates)
	// Native folder picker (local desktop) for choosing a workspace data dir.
	mux.HandleFunc("POST /api/pick-folder", s.handlePickFolder)

	// Per-workspace settings (resolved from X-Workspace-Id).
	mux.HandleFunc("GET /api/workspace-settings", s.handleGetWorkspaceSettings)
	mux.HandleFunc("PUT /api/workspace-settings", s.handleUpdateWorkspaceSettings)
	mux.HandleFunc("GET /api/workspace-settings/claude-auth", s.handleWorkspaceClaudeAuth)
	mux.HandleFunc("POST /api/workspace-settings/claude-auth/oauth/start", s.handleClaudeOAuthStart)
	mux.HandleFunc("POST /api/workspace-settings/claude-auth/oauth/complete", s.handleClaudeOAuthComplete)
	mux.HandleFunc("POST /api/workspace-settings/claude-auth/oauth/loopback/start", s.handleClaudeOAuthLoopbackStart)
	mux.HandleFunc("GET /api/workspace-settings/claude-auth/oauth/loopback/status", s.handleClaudeOAuthLoopbackStatus)

	// Per-workspace editable config files (prompts/instructions/README under
	// <workspace>/config/), editable by the user on disk or via the UI.
	mux.HandleFunc("GET /api/workspace-config", s.handleGetWorkspaceConfig)
	mux.HandleFunc("PUT /api/workspace-config", s.handleUpdateWorkspaceConfig)
	mux.HandleFunc("POST /api/workspace-config/reveal", s.handleRevealWorkspaceConfig)
}

// registerAgentRoutes registers agent CRUD.
func (s *Server) registerAgentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("POST /api/agents", s.handleCreateAgent)
	mux.HandleFunc("PUT /api/agents/{id}", s.handleUpdateAgent)
	mux.HandleFunc("DELETE /api/agents/{id}", s.handleDeleteAgent)
	// Fresh-start context preview (assembled system prompt + tool catalog).
	mux.HandleFunc("GET /api/agents/{id}/context", s.handleAgentContext)
	// On-disk JSON file path + reveal in the OS file manager (local desktop).
	mux.HandleFunc("GET /api/agents/{id}/path", s.handleAgentPath)
	mux.HandleFunc("POST /api/agents/{id}/reveal", s.handleRevealAgent)
}

// registerSessionRoutes registers chat sessions + messages + titling.
func (s *Server) registerSessionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("GET /api/sessions/active", s.handleActiveSessions)
	mux.HandleFunc("GET /api/sessions/{id}/inflight", s.handleSessionInflight)
	mux.HandleFunc("DELETE /api/sessions/{id}/cli-process", s.handleDropSessionCLIProcess)
	mux.HandleFunc("GET /api/sessions/search", s.handleSearchMessages)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /api/sessions/spawn", s.handleSpawnSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)
	mux.HandleFunc("DELETE /api/sessions/{id}/messages/{msgId}", s.handleDeleteMessage)
	mux.HandleFunc("POST /api/sessions/{id}/rewind", s.handleRewindSession)
	mux.HandleFunc("POST /api/sessions/{id}/title", s.handleGenerateSessionTitle)
	mux.HandleFunc("PUT /api/sessions/{id}/goal", s.handleSetSessionGoal)
	mux.HandleFunc("PUT /api/sessions/{id}/state", s.handleSetSessionState)
	mux.HandleFunc("PUT /api/sessions/{id}/pin", s.handleSetSessionPin)
	mux.HandleFunc("PUT /api/sessions/{id}/tags", s.handleSetSessionTags)
	mux.HandleFunc("PUT /api/sessions/{id}/messages/{msgId}/feedback", s.handleSetMessageFeedback)
	mux.HandleFunc("PUT /api/sessions/{id}/agent", s.handleSetSessionAgent)
	mux.HandleFunc("PUT /api/sessions/{id}/role", s.handleSetSessionRole)
	mux.HandleFunc("GET /api/sessions/{id}/workers", s.handleListWorkers)
	mux.HandleFunc("GET /api/sessions/{id}/workdir", s.handleGetSessionWorkdir)
	mux.HandleFunc("PUT /api/sessions/{id}/workdir", s.handleSetSessionWorkdir)
	mux.HandleFunc("GET /api/fs/browse", s.handleBrowseDirs)
	mux.HandleFunc("GET /api/fs/gitinfo", s.handleGitInfo)
	mux.HandleFunc("POST /api/git/init", s.handleGitInit)
	mux.HandleFunc("POST /api/git/config", s.handleGitConfig)
	mux.HandleFunc("POST /api/sessions/{id}/summary", s.handleSessionSummary)
	mux.HandleFunc("POST /api/sessions/{id}/handoff", s.handleSessionHandoff)
	mux.HandleFunc("POST /api/sessions/{id}/run-flow", s.handleSessionRunFlow)
	mux.HandleFunc("POST /api/sessions/{id}/run-flow-stream", s.handleSessionRunFlowStream)
	mux.HandleFunc("POST /api/sessions/{id}/read", s.handleMarkSessionRead)
	mux.HandleFunc("GET /api/sessions/{id}/info", s.handleSessionInfo)
	mux.HandleFunc("GET /api/sessions/{id}/progress", s.handleSessionProgress)
	// Debug: preview the EXACT next-turn context (system + dynamic + transcript +
	// tools) this session's agent would be sent. Optional ?message= sample turn.
	mux.HandleFunc("GET /api/sessions/{id}/context-preview", s.handleSessionContextPreview)
	mux.HandleFunc("GET /api/sessions/{id}/path", s.handleSessionPath)
	mux.HandleFunc("POST /api/sessions/{id}/reveal", s.handleRevealSession)
}

// registerChatRoutes registers the completion endpoints.
func (s *Server) registerChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/chat", s.handleChat)
	// SSE streaming variant: emits each activity step as it occurs.
	mux.HandleFunc("POST /api/chat/stream", s.handleChatStream)
	// Control an in-flight streaming turn: stop (cancel) or steer (live guidance).
	mux.HandleFunc("POST /api/chat/control", s.handleChatControl)
	// Disarm a pending one-shot self-wake (schedule_wake) for a session — the
	// user pressed "Durdur" on the waiting banner before the wake fired.
	mux.HandleFunc("POST /api/chat/wake/cancel", s.handleCancelWake)
	// Interaction MCP endpoint: CLI agents (claude-cli, ...) call TionSwarm's
	// human-in-the-loop tools (ask_user/todo_write) here over MCP-over-HTTP.
	// Bound to all methods; the handler does its own bearer auth + method switch.
	// Guarded so a bare &Server{} (route-registration test) doesn't panic on a
	// nil handler; NewServer always installs it.
	if s.interactionMCP != nil {
		mux.Handle("/mcp/interaction", s.interactionMCP)
		// Subtree mount so the CLI's two tier entries (/mcp/interaction/core and
		// /mcp/interaction/extended) reach the same handler; it derives the tier from
		// the path's last segment (see interaction.tierFromPath).
		mux.Handle("/mcp/interaction/", s.interactionMCP)
	}
}

// registerTaskRoutes registers the kanban board + run history.
func (s *Server) registerTaskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/title", s.handleGenerateTaskTitle)
}

// registerScheduleRoutes registers cron schedules.
func (s *Server) registerScheduleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/schedules", s.handleListSchedules)
	mux.HandleFunc("POST /api/schedules", s.handleCreateSchedule)
	mux.HandleFunc("PUT /api/schedules/{id}", s.handleUpdateSchedule)
	mux.HandleFunc("POST /api/schedules/{id}/toggle", s.handleToggleSchedule)
	mux.HandleFunc("POST /api/schedules/{id}/run", s.handleRunSchedule)
	mux.HandleFunc("PUT /api/schedules/{id}/tags", s.handleSetScheduleTags)
	mux.HandleFunc("DELETE /api/schedules/{id}", s.handleDeleteSchedule)
	// Tag-triggered automations (event-driven loops), surfaced in the Schedules UI.
	mux.HandleFunc("GET /api/automations", s.handleListAutomations)
	mux.HandleFunc("POST /api/automations", s.handleCreateAutomation)
	mux.HandleFunc("PUT /api/automations/{id}", s.handleUpdateAutomation)
	mux.HandleFunc("POST /api/automations/{id}/toggle", s.handleToggleAutomation)
	mux.HandleFunc("POST /api/automations/{id}/reset", s.handleResetAutomation)
	mux.HandleFunc("DELETE /api/automations/{id}", s.handleDeleteAutomation)
}

// registerUsageRoutes registers the spend meter + the context meter.
func (s *Server) registerUsageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents/{id}/usage", s.handleAgentUsage)
	mux.HandleFunc("GET /api/sessions/{id}/context", s.handleSessionContext)
	mux.HandleFunc("GET /api/sessions/{id}/usage-detail", s.handleSessionUsageDetail)
	mux.HandleFunc("GET /api/sessions/{id}/debug", s.handleSessionDebug)
	mux.HandleFunc("GET /api/sessions/{id}/turn-debug", s.handleSessionTurnDebug)
	mux.HandleFunc("GET /api/usage", s.handleWorkspaceUsage)
}

// registerMCPRoutes registers MCP servers + per-agent tool access (Phase 8).
func (s *Server) registerMCPRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mcp-servers", s.handleListMCPServers)
	mux.HandleFunc("POST /api/mcp-servers", s.handleCreateMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/import", s.handleImportMCPServers)
	mux.HandleFunc("POST /api/mcp-servers/{id}/toggle", s.handleToggleMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/test", s.handleTestMCPServer)
	mux.HandleFunc("DELETE /api/mcp-servers/{id}", s.handleDeleteMCPServer)
	mux.HandleFunc("GET /api/agents/{id}/tools", s.handleAgentTools)
	mux.HandleFunc("POST /api/agents/{id}/tools", s.handleSetAgentTools)
	mux.HandleFunc("GET /api/workspace-tools", s.handleWorkspaceTools)
	mux.HandleFunc("PUT /api/workspace-tools", s.handleSetWorkspaceTools)
}

// registerHookRoutes registers PreToolUse/PostToolUse hooks (Phase P4).
func (s *Server) registerHookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hooks", s.handleListHooks)
	mux.HandleFunc("GET /api/hooks/builtins", s.handleListBuiltinHooks)
	mux.HandleFunc("POST /api/hooks", s.handleCreateHook)
	mux.HandleFunc("PUT /api/hooks/{id}", s.handleUpdateHook)
	mux.HandleFunc("POST /api/hooks/{id}/toggle", s.handleToggleHook)
	mux.HandleFunc("DELETE /api/hooks/{id}", s.handleDeleteHook)
}

// registerFlowRoutes registers orchestration flows + runs (Phase 7).
func (s *Server) registerFlowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/flows", s.handleListFlows)
	mux.HandleFunc("POST /api/flows", s.handleCreateFlow)
	mux.HandleFunc("PUT /api/flows/{id}", s.handleUpdateFlow)
	mux.HandleFunc("PUT /api/flows/{id}/tags", s.handleSetFlowTags)
	mux.HandleFunc("PUT /api/flows/{id}/emoji", s.handleSetFlowEmoji)
	mux.HandleFunc("DELETE /api/flows/{id}", s.handleDeleteFlow)
	// Locate the flow on disk: copy its path or open its folder in Explorer.
	mux.HandleFunc("GET /api/flows/{id}/path", s.handleFlowPath)
	mux.HandleFunc("POST /api/flows/{id}/reveal", s.handleRevealFlow)
	mux.HandleFunc("POST /api/flows/{id}/run", s.handleRunFlow)
	mux.HandleFunc("POST /api/flows/{id}/run-stream", s.handleRunFlowStream)
	mux.HandleFunc("GET /api/flow-runs", s.handleListFlowRuns)
	mux.HandleFunc("GET /api/flow-runs/{id}", s.handleGetFlowRun)
}

// registerExecutionRoutes registers the unified executions feed — every run
// across chat/task/flow/schedule funnels into a Session, so this is the single
// list that surfaces them all with their kind and live status.
func (s *Server) registerExecutionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/executions", s.handleListExecutions)
	// Per-view "work in progress" flags for the left-nav busy indicators.
	mux.HandleFunc("GET /api/activity", s.handleActivity)
}

// registerArtifactRoutes registers the artifact store (versioned agent-produced
// content viewed in a dedicated screen).
func (s *Server) registerArtifactRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts", s.handleListArtifacts)
	mux.HandleFunc("POST /api/artifacts", s.handleCreateArtifact)
	mux.HandleFunc("GET /api/artifacts/{id}", s.handleGetArtifact)
	mux.HandleFunc("PUT /api/artifacts/{id}", s.handleUpdateArtifact)
	mux.HandleFunc("DELETE /api/artifacts/{id}", s.handleDeleteArtifact)
	// Locate the artifact on disk: copy its path or open its folder in Explorer.
	mux.HandleFunc("GET /api/artifacts/{id}/path", s.handleArtifactPath)
	mux.HandleFunc("POST /api/artifacts/{id}/reveal", s.handleRevealArtifact)
}

// registerSkillRoutes registers the file-based skill catalog (reusable agent
// instruction sets resolved from global/workspace/project tiers).
func (s *Server) registerSkillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/skills", s.handleListSkills)
	mux.HandleFunc("POST /api/skills", s.handleCreateSkill)
	mux.HandleFunc("POST /api/skills/import", s.handleImportSkill)
	mux.HandleFunc("POST /api/skills/reload", s.handleReloadSkills)
	mux.HandleFunc("GET /api/skills/{slug}", s.handleGetSkill)
	mux.HandleFunc("PUT /api/skills/{slug}", s.handleUpdateSkill)
	mux.HandleFunc("DELETE /api/skills/{slug}", s.handleDeleteSkill)
	mux.HandleFunc("PUT /api/skills/{slug}/access", s.handleSetSkillAccess)
	mux.HandleFunc("PUT /api/skills/{slug}/auto-summary", s.handleSetSkillAutoSummary)
	mux.HandleFunc("PUT /api/skills/{slug}/name-only", s.handleSetSkillNameOnly)
	mux.HandleFunc("PUT /api/skills/{slug}/visibility", s.handleSetSkillVisibility)
	mux.HandleFunc("PUT /api/skills/{slug}/group", s.handleSetSkillGroup)
	mux.HandleFunc("POST /api/skills/{slug}/reveal", s.handleRevealSkill)
}

// registerSettingsRoutes registers the global application settings document.
func (s *Server) registerSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("POST /api/settings/test-provider", s.handleTestProvider)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/prompts", s.handleListPrompts)
	mux.HandleFunc("POST /api/prompts/reveal", s.handleRevealPrompts)
	// Custom (user-added) OpenAI/Anthropic-compatible providers.
	mux.HandleFunc("GET /api/providers", s.handleListProviders)
	mux.HandleFunc("PUT /api/providers", s.handleUpsertProvider)
	mux.HandleFunc("DELETE /api/providers/{id}", s.handleDeleteProvider)
	mux.HandleFunc("GET /api/prices", s.handlePrices)
	// Workspace backups — status + on-demand run (the schedule itself is driven
	// by the settings document, not these endpoints).
	mux.HandleFunc("GET /api/backups", s.handleBackupStatus)
	mux.HandleFunc("POST /api/backups/run", s.handleBackupRun)
	mux.HandleFunc("GET /api/backups/archives", s.handleListArchives)
	mux.HandleFunc("POST /api/backups/restore", s.handleRestoreBackup)
	mux.HandleFunc("DELETE /api/backups/archives", s.handleDeleteArchive)
}


// registerSecretRoutes registers the per-workspace secret vault (resolved from
// X-Workspace-Id). Values are write-only except for the explicit reveal action.
func (s *Server) registerSecretRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/secrets", s.handleListSecrets)
	mux.HandleFunc("POST /api/secrets", s.handleSetSecret)
	mux.HandleFunc("GET /api/secrets/{name}/reveal", s.handleRevealSecret)
	mux.HandleFunc("DELETE /api/secrets/{name}", s.handleDeleteSecret)
}

// registerMiscRoutes registers inline media serving and the logs feed.
func (s *Server) registerMiscRoutes(mux *http.ServeMux) {
	// Inline media (images referenced by chat content) — read-only.
	mux.HandleFunc("GET /api/files", s.handleServeFile)
	// User message attachments: upload a file (or pasted text) for the next turn,
	// or delete one that was staged then cancelled before sending.
	mux.HandleFunc("POST /api/uploads", s.handleUpload)
	mux.HandleFunc("DELETE /api/uploads", s.handleDeleteUpload)
	// Application + workspace logs (global ring buffer).
	mux.HandleFunc("GET /api/logs", s.handleListLogs)
	// On-disk log file path + reveal in the OS file manager (local desktop).
	mux.HandleFunc("GET /api/logs/path", s.handleLogsPath)
	mux.HandleFunc("POST /api/logs/reveal", s.handleRevealLogs)
	// Frontend error bridge: client-side crashes/rejections funnel into the log
	// stream so they surface in the Logs screen, not just the browser console.
	mux.HandleFunc("POST /api/logs", s.handleClientLog)
	// Autonomous event feed (task/schedule) — SSE, global.
	mux.HandleFunc("GET /api/events", s.handleEvents)
	// Detect optional external token-optimization tools on PATH (presence-only,
	// never installs/runs them) — surfaced by the Settings diagnostics panel.
	mux.HandleFunc("GET /api/external-tools", s.handleExternalTools)
}

// withWorkspace resolves the active workspace from the X-Workspace-Id header
// (falling back to the default) and injects it into the request context.
func (s *Server) withWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Workspace-Id")
		// Fallback for requests that cannot set headers (e.g. an <img src> loading
		// an inline attachment): accept ?ws=<id> as the workspace scope.
		if id == "" {
			id = r.URL.Query().Get("ws")
		}
		var ws *workspace.Workspace
		if id != "" {
			// Fall back to the default workspace when the requested id is unknown
			// (e.g. a stale id in localStorage after the workspace was deleted), so
			// the app can always recover instead of bricking on 400s.
			if found, err := s.workspaces.Get(id); err == nil {
				ws = found
			} else {
				ws = s.workspaces.Default()
			}
		} else {
			ws = s.workspaces.Default()
		}
		ctx := context.WithValue(r.Context(), workspaceCtxKey, ws)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ws returns the workspace bound to the current request.
func ws(r *http.Request) *workspace.Workspace {
	v, _ := r.Context().Value(workspaceCtxKey).(*workspace.Workspace)
	return v
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeDBError maps a store/runtime error to an HTTP response and reports
// whether it handled one. db.ErrNotFound becomes 404 with notFoundMsg; any
// other non-nil error becomes 500 with the error text. Lets handlers collapse
// the repeated not-found/500 branches into: if writeDBError(...) { return }.
func writeDBError(w http.ResponseWriter, err error, notFoundMsg string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrNotFound):
		writeError(w, http.StatusNotFound, notFoundMsg)
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
	return true
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

// withCORS allows the local frontend dev server to call the API.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Workspace-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"service":    "tionswarm",
		"version":    "0.0.1",
		"workspaces": len(s.workspaces.List()),
	})
}
