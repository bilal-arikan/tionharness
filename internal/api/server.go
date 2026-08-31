// Package api exposes the TionHarness HTTP/JSON interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/backup"
	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/exttools"
	"github.com/bilal-arikan/tionharness/internal/gateway"
	"github.com/bilal-arikan/tionharness/internal/interaction"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/settings"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/web"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// ctxKey is the private type for request-context values.
type ctxKey string

const workspaceCtxKey ctxKey = "workspace"

// Server holds dependencies for HTTP handlers.
type Server struct {
	dataDir       string
	workspaces    *workspace.Manager
	providers     *providers.Registry
	providerStore *settings.ProviderStore
	convo         *conversation.Manager
	settings      *settings.Store
	tun           *agent.Tunables
	logs          *logbuf.Buffer
	bus           *events.Bus       // autonomous notifications streamed to the UI over SSE
	hub           *sessionhub.Hub   // per-session ordered event log (cursor-based, all windows subscribe)
	interactions  *interactionStore // per-session resolve-once human-in-the-loop prompts (CAS)
	inbox         *inboxStore       // per-session durable command queue (serial worker → turns)
	runs          *chatRuns         // in-flight streaming turns (stop/steer control)
	teardownLocks sessionTeardownLocks
	stopWorker    func(context.Context, *workspace.Workspace, string) error
	deleteSession func(context.Context, *workspace.Workspace, string) error // test seam for delete-lock ordering
	grants        *permGrantStore                                           // per-session "Always allow" permission grants
	logger        *slog.Logger

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
	// interactionSrv is the same server as a concrete type, so the activate path can
	// push tools/list_changed to a live CLI session's SSE stream (gateway, Doc 52).
	interactionSrv *interaction.Server
	// interBackend is the Interaction MCP backend, kept so paths outside the MCP
	// handler can reach the per-session activation state — currently the dead-tool
	// repair, which activates a tool the CLI rejected (see dead_tool_activate.go).
	interBackend *interactionBackend
	// gatewaySrv is the EXTERNAL MCP gateway (Doc 52 Faz 3), exposing the default
	// workspace's SHARED MCP pool to outside clients at /mcp/gateway. nil unless opted in
	// at boot (TIONHARNESS_GATEWAY_EXTERNAL). It borrows the workspace pool (#11), so there
	// is nothing pool-related to own or close here.
	gatewaySrv *gateway.Server
	// gatewayRequireLoopback is true when no auth token is configured: the endpoint is
	// then reachable ONLY from loopback (the safe default — no token, no network).
	gatewayRequireLoopback bool
	// backups runs the periodic workspace-backup loop; reconfigured on every
	// settings change. nil until wired by SetBackupManager (after construction).
	backups *backup.Manager
	// updateChk caches the release-feed check (see updatecheck.go). Created on
	// first use so a bare Server needs no extra wiring.
	updateChk   *updateChecker
	updatesOnce sync.Once
}

// NewServer constructs an API server and pushes the persisted settings into the
// live subsystems (providers, compaction, autonomy). tun is the shared
// process-wide tunables updated whenever settings change; bus is the
// process-wide event bus exposed via the /api/events SSE feed.
func NewServer(manager *workspace.Manager, registry *providers.Registry, providerStore *settings.ProviderStore, store *settings.Store, tun *agent.Tunables, logs *logbuf.Buffer, bus *events.Bus, logger *slog.Logger) *Server {
	s := &Server{
		dataDir:       manager.DataDir(),
		workspaces:    manager,
		providers:     registry,
		providerStore: providerStore,
		convo:         conversation.NewManager(),
		settings:      store,
		tun:           tun,
		logs:          logs,
		bus:           bus,
		// Per-session event hub: a fresh epoch each boot so a client presenting a
		// cursor from a previous process is told to reset instead of trusting a
		// stale (restarted-to-zero) seq. See _Docs/58-QUEUE-SENKRON.md.
		hub:          sessionhub.New(uuid.NewString(), 0),
		interactions: newInteractionStore(),
		inbox:        newInboxStore(),
		runs:         newChatRuns(),
		grants:       newPermGrantStore(),
		logger:       logger,
		// Workspace-independent market store (bundled + global tiers) for the
		// workspace-template picker, which must work with zero workspaces during
		// onboarding. No ledger dir: install-status tracking is per-workspace.
		market: market.New(agent.MarketGlobalDir(), ""),
	}
	// Interaction MCP: lets CLI agents (claude-cli, ...) reach TionHarness's
	// human-in-the-loop tools over in-process HTTP. See _Docs/11-INTERACTION-MCP.md.
	interBackend := &interactionBackend{runs: s.runs, tun: tun, apiSrv: s}
	s.interactionSrv = interaction.NewServer(interBackend, logger)
	// Wire the pusher back so activate_tools can push tools/list_changed (gateway, Doc 52).
	interBackend.setServer(s.interactionSrv)
	s.interBackend = interBackend
	s.interactionMCP = s.interactionSrv
	// External MCP gateway (Doc 52 Faz 3): opt-in at boot. Exposes the default
	// workspace's enabled MCP servers to OUTSIDE clients at /mcp/gateway, gateway-style
	// (meta-tools + activate + tools/list_changed). Security: a token
	// (TIONHARNESS_GATEWAY_AUTH_TOKEN) is required to reach it over the network; without a
	// token it is loopback-only. Default OFF — external exposure must be explicit.
	if envTruthy(os.Getenv("TIONHARNESS_GATEWAY_EXTERNAL")) {
		token := strings.TrimSpace(os.Getenv("TIONHARNESS_GATEWAY_AUTH_TOKEN"))
		// Per-workspace routing: a client picks its workspace via the X-Workspace-Id
		// header (captured at initialize); "" or unknown falls back to the default. Each
		// workspace's own persistent MCP pool is SHARED (#11) — no dedicated pool.
		resolveWs := func(wsID string) *workspace.Workspace {
			if wsID != "" {
				if wsp, err := s.workspaces.Get(wsID); err == nil {
					return wsp
				}
			}
			return s.workspaces.Default()
		}
		poolFn := func(wsID string) *mcp.Pool {
			wsp := resolveWs(wsID)
			if wsp == nil || wsp.Runtime == nil {
				return nil
			}
			return wsp.Runtime.MCPPool()
		}
		serversFn := func(ctx context.Context, wsID string) ([]mcp.ServerConfig, error) {
			wsp := resolveWs(wsID)
			if wsp == nil || wsp.DB == nil {
				return nil, nil
			}
			rows, err := wsp.DB.ListEnabledMCPServers(ctx)
			if err != nil {
				return nil, err
			}
			cfgs := make([]mcp.ServerConfig, 0, len(rows))
			for _, m := range rows {
				cfgs = append(cfgs, agent.ToServerConfig(m))
			}
			return cfgs, nil
		}
		var authFn func(string) bool
		if token != "" {
			authFn = func(t string) bool { return t == token }
		}
		gb := gateway.NewBackend(poolFn, serversFn, logger)
		s.gatewaySrv = gateway.NewServer(gb, authFn, logger)
		gb.SetServer(s.gatewaySrv)
		// Audit parity: log each proxied backend tool call to a JSONL file (TS gateway's
		// gateway-audit.jsonl equivalent). Opt-in via TIONHARNESS_GATEWAY_AUDIT_LOG=<path>;
		// fire-and-forget so a write error never breaks a tool call.
		if p := strings.TrimSpace(os.Getenv("TIONHARNESS_GATEWAY_AUDIT_LOG")); p != "" && !strings.EqualFold(p, "off") {
			gb.SetAudit(func(e gateway.AuditEntry) {
				line, err := json.Marshal(e)
				if err != nil {
					return
				}
				f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return
				}
				defer f.Close()
				_, _ = f.Write(append(line, '\n'))
			})
			logger.Info("external gateway audit log enabled", "path", p)
		}
		s.gatewayRequireLoopback = token == ""
		if token == "" {
			logger.Warn("external MCP gateway ENABLED at /mcp/gateway — loopback-only (no TIONHARNESS_GATEWAY_AUTH_TOKEN set)")
		} else {
			logger.Warn("external MCP gateway ENABLED at /mcp/gateway — token auth required")
		}
	}
	// Headless Interaction MCP: give autonomous (scheduler/spawn) CLI
	// turns the same use_skill/shell/self-manage bridge chat turns get.
	manager.SetAutonomousInteraction(s.autonomousInteraction)
	// Dead-tool self-repair: let the agent runtime activate an on-demand tool the
	// CLI rejected with "No such tool available" (WS20/SES79, dead_tool_activate.go).
	manager.SetDeadToolActivator(s.activateDeadTool)
	// Let every runtime see the INTERACTIVE turns too: the chat-run registry lives
	// here, but Runtime.AgentBusy is what BOTH agent-delete paths consult, so
	// without this the self-management delete_agent tool would only ever see
	// autonomous runs and could delete an agent mid-chat.
	manager.SetExternalActiveSessions(s.runs.activeSessionIDs)
	// History-aware self-wake: let schedule_wake continue with the full
	// conversation (composed like a chat turn) instead of just the wake prompt.
	manager.SetWakeTurnRunner(s.wakeTurnRunner)
	// Surface compaction (rolling-summary fold + manual /compact) in the in-app
	// Logs screen; it was previously visible only in the chat response payload.
	s.convo.SetLogger(logger)
	// Mirror autonomous turns' live steps from the bus onto the per-session hub so
	// every window renders them from the same authoritative stream (Faz 1B).
	go s.bridgeBusToHub()
	// Re-dispatch any durably-queued messages left by a crash/restart (Faz 3).
	go s.recoverInboxes()
	// Reclaim autonomous background turns (worker/spawn/coordinator) orphaned by a
	// crash/restart: mark them interrupted + notify the coordinator so it resumes
	// instead of waiting forever on a worker that will never report.
	go s.recoverAutonomousTurns()
	s.applySettings()
	return s
}

// SetBaseURL records this server's own loopback base URL (e.g.
// http://127.0.0.1:8090) so the Interaction MCP endpoint can be advertised to
// CLI subprocesses. Pass the listen address; a wildcard/empty host is normalised
// to 127.0.0.1 so a same-machine subprocess can connect.
func (s *Server) SetBaseURL(addr string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// No parseable "host:port" (e.g. a bare host or bare port): treat the
		// whole thing as a host with the default port.
		host, port = addr, "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	s.selfURL = "http://" + net.JoinHostPort(host, port)
}

// applySettings pushes the current settings into every live subsystem. Called
// on boot and after each successful settings update.
func (s *Server) applySettings() {
	cur := s.settings.Get()
	instances := s.registryInstances()
	s.providers.SetInstances(instances)
	// Keep the external-tools panel pointed at the SAME binary the resolved
	// default instance runs; an empty value clears the override and restores
	// PATH lookup. Faz 2 moves cliPath onto the per-instance config (K1), so this
	// reads the "claude-cli"/"codex-cli" default instance's own field instead of
	// the (now legacy, migration-only) top-level settings.
	exttools.SetPathOverride(exttools.ClaudeToolName, instanceFieldValue(instances, "claude-cli", providers.FieldKeyCLIPath))
	exttools.SetPathOverride(exttools.CodexToolName, instanceFieldValue(instances, "codex-cli", providers.FieldKeyCLIPath))
	s.providers.SetAnthropicBetas(cur.ExtendedPromptCache, cur.AnthropicContextEditing, cur.AnthropicServerCompaction, cur.AnthropicRefusalFallback)
	s.convo.SetLimits(cur.MaxContextTokens, cur.KeepRecentMsgs)
	s.convo.SetBudgetShape(cur.ContextBudgetFraction, cur.ContextBudgetCeil) // model-aware budget knobs
	s.tun.SetHandoff(cur.HandoffAuto, cur.HandoffMaxChain, cur.HandoffWriteFile)
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
	s.tun.SetSpawnLimits(cur.SpawnMaxConcurrent, cur.SpawnQueueMax, cur.SpawnMaxPerTurn)
	s.tun.SetSpawnTimeoutMinutes(cur.SpawnTimeoutMin)
	s.tun.SetSpawnIdleTimeoutMinutes(cur.SpawnIdleTimeoutMin)
	s.tun.SetChatTurnTimeoutMinutes(cur.ChatTurnTimeoutMin)
	s.tun.SetChatTurnIdleTimeoutMinutes(cur.ChatTurnIdleTimeoutMin)
	s.tun.SetIdleResumeMax(cur.IdleResumeMax)
	s.tun.SetScheduleTimeoutMinutes(cur.ScheduleTimeoutMin)
	s.tun.SetTurnWatchdogMinutes(cur.TurnWatchdogMin)
	s.tun.SetTurnIdleWatchdogMinutes(cur.TurnIdleWatchdogMin)
	tools.SetShellTimeouts(cur.ShellDefaultTimeoutSec, cur.ShellMaxTimeoutSec)
	tools.SetMaxToolOutputBytes(cur.MaxToolOutputKB * 1024)
	s.tun.SetAgentMessageMaxBytes(cur.AgentMessageMaxKB * 1024)
	s.tun.SetCoordinatorLimits(cur.CoordinatorMaxWorkers, cur.CoordinatorMaxTurns,
		cur.CoordinatorMaxDepth, cur.CoordinatorMaxSubtreeSessions)
	s.tun.SetCoordinatorSettleGrace(cur.CoordinatorSettleGraceSec)
	s.tun.SetCoordinatorStallGuard(cur.CoordinatorStallGuard, cur.CoordinatorStallSweepMin, cur.CoordinatorStallMaxNudges)
	s.tun.SetWorkdirGuards(cur.AutonomousConfine, cur.AutonomousBootSeq)
	s.tun.SetAutonomousTaskBudget(cur.AutonomousTaskBudgetTokens)
	s.tun.SetNativeToolSearch(cur.AnthropicNativeToolSearch)
	s.tun.SetProgrammaticTools(cur.AnthropicProgrammaticTools)
	s.tun.SetWebTools(cur.AnthropicWebTools)
	s.tun.SetServerCompaction(cur.AnthropicServerCompaction)
	s.tun.SetRecoveryLimits(cur.ReactiveCompact, cur.MaxTokenRetries, cur.ReactiveKeepRecent)
	s.tun.SetProviderRetryMax(cur.MaxProviderRetries)
	s.tun.SetToolGuard(cur.ToolGuardWarnings, cur.ToolGuardHardStop)
	s.tun.SetToolGuardThresholds(cur.GuardExactWarn, cur.GuardExactBlock, cur.GuardSameToolWarn, cur.GuardSameToolHalt, cur.GuardNoProgressWarn, cur.GuardNoProgressBlck)
	s.tun.SetStuckTurnThreshold(cur.StuckTurnThreshold)
	s.tun.SetLessonReflect(cur.LessonReflect)
	db.SetLessonMaxAgeDays(cur.LessonMaxAgeDays)
	s.tun.SetLanguage(languageName(cur.Language))
	s.tun.SetMaxOutputTokens(cur.MaxOutputTokens)
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
	// Advisory release-feed check for the "newer version available" banner.
	// Cached server-side (24h) so the UI can call it freely.
	mux.HandleFunc("GET /api/version/update", s.handleUpdateCheck)
	// In-memory store footprint per workspace — the measurement baseline for the
	// message lazy-loading work. Global (not workspace-scoped) on purpose: the
	// question it answers is "what does the PROCESS hold?".
	mux.HandleFunc("GET /api/debug/store-stats", s.handleStoreStats)

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
	s.registerTTSRoutes(mux)
	s.registerSTTRoutes(mux)
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
	// Cross-workspace live-run flags (switcher pulse for non-active workspaces).
	mux.HandleFunc("GET /api/workspaces/activity", s.handleWorkspacesActivity)
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
	mux.HandleFunc("GET /api/workspace-settings/codex-auth", s.handleWorkspaceCodexAuth)
	mux.HandleFunc("POST /api/workspace-settings/codex-auth/device/start", s.handleCodexDeviceStart)
	mux.HandleFunc("GET /api/workspace-settings/codex-auth/device/status", s.handleCodexDeviceStatus)
	mux.HandleFunc("POST /api/workspace-settings/codex-auth/device/cancel", s.handleCodexDeviceCancel)
	mux.HandleFunc("POST /api/workspace-settings/codex-auth/api-key", s.handleCodexAPIKeyLogin)

	// Per-workspace editable config files (prompts/instructions/README under
	// <workspace>/config/), editable by the user on disk or via the UI.
	mux.HandleFunc("GET /api/workspace-config", s.handleGetWorkspaceConfig)
	mux.HandleFunc("PUT /api/workspace-config", s.handleUpdateWorkspaceConfig)
}

// registerAgentRoutes registers agent CRUD.
func (s *Server) registerAgentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("POST /api/agents", s.handleCreateAgent)
	mux.HandleFunc("PUT /api/agents/{id}", s.handleUpdateAgent)
	mux.HandleFunc("DELETE /api/agents/{id}", s.handleDeleteAgent)
	mux.HandleFunc("POST /api/agents/{id}/restore-default", s.handleRestoreSystemAgent)
	// Clone an existing agent (full profile + tool config) into a new "(kopya)".
	mux.HandleFunc("POST /api/agents/{id}/duplicate", s.handleDuplicateAgent)
	// Fresh-start context preview (assembled system prompt + tool catalog).
	mux.HandleFunc("GET /api/agents/{id}/context", s.handleAgentContext)
	// On-disk JSON file path (for the copy-path action).
	mux.HandleFunc("GET /api/agents/{id}/path", s.handleAgentPath)
}

// registerSessionRoutes registers chat sessions + messages + titling.
func (s *Server) registerSessionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("GET /api/sessions/active", s.handleActiveSessions)
	mux.HandleFunc("GET /api/sessions/{id}/inflight", s.handleSessionInflight)
	// Server-authoritative per-session event stream (cursor-based SSE): every
	// window watching a session subscribes here and renders live from it,
	// whoever started the turn. See _Docs/58-QUEUE-SENKRON.md.
	mux.HandleFunc("GET /api/sessions/{id}/stream", s.handleSessionStream)
	// Resolve-once human-in-the-loop answer (ask_user / permission / plan): the
	// first window to answer wins via CAS; the others' cards close on the
	// broadcast interaction_resolved. See _Docs/58-QUEUE-SENKRON.md Faz 2.
	mux.HandleFunc("POST /api/sessions/{id}/interactions/{iid}/answer", s.handleInteractionAnswer)
	// Send-queue (Faz 3): enqueue a user turn (durable, idempotent on clientMsgId),
	// cancel a not-yet-dispatched one, and stop/steer the in-flight turn without a
	// runId (the queue runs turns server-side → control is session-scoped).
	mux.HandleFunc("POST /api/sessions/{id}/messages", s.handleEnqueueMessage)
	mux.HandleFunc("DELETE /api/sessions/{id}/queue/{msgId}", s.handleCancelQueued)
	mux.HandleFunc("DELETE /api/sessions/{id}/queue", s.handleClearQueue)
	mux.HandleFunc("POST /api/sessions/{id}/queue/{msgId}/front", s.handleMoveQueuedFront)
	mux.HandleFunc("POST /api/sessions/{id}/control", s.handleSessionControl)
	// Cross-window "user is typing" signal (ephemeral, not retained).
	mux.HandleFunc("POST /api/sessions/{id}/typing", s.handleTyping)
	mux.HandleFunc("DELETE /api/sessions/{id}/cli-process", s.handleDropSessionCLIProcess)
	mux.HandleFunc("GET /api/sessions/search", s.handleSearchMessages)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("POST /api/sessions/spawn", s.handleSpawnSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)
	mux.HandleFunc("DELETE /api/sessions/{id}/messages/{msgId}", s.handleDeleteMessage)
	// Untrimmed activity trace for one turn — the listing above ships tool
	// payloads cut to stepFieldCap.
	mux.HandleFunc("GET /api/sessions/{id}/messages/{msgId}/steps", s.handleMessageSteps)
	// Every file mutation in the session, untrimmed — the chat's bulk diff popup.
	mux.HandleFunc("GET /api/sessions/{id}/changes", s.handleSessionChanges)
	mux.HandleFunc("POST /api/sessions/{id}/rewind", s.handleRewindSession)
	mux.HandleFunc("POST /api/sessions/{id}/title", s.handleGenerateSessionTitle)
	mux.HandleFunc("PUT /api/sessions/{id}/state", s.handleSetSessionState)
	mux.HandleFunc("PUT /api/sessions/{id}/pin", s.handleSetSessionPin)
	mux.HandleFunc("PUT /api/sessions/{id}/tags", s.handleSetSessionTags)
	mux.HandleFunc("PUT /api/sessions/{id}/messages/{msgId}/feedback", s.handleSetMessageFeedback)
	mux.HandleFunc("PUT /api/sessions/{id}/agent", s.handleSetSessionAgent)
	mux.HandleFunc("PUT /api/sessions/{id}/role", s.handleSetSessionRole)
	mux.HandleFunc("POST /api/sessions/{id}/coordinator/resume", s.handleResumeCoordinator)
	mux.HandleFunc("PUT /api/sessions/{id}/workflow", s.handleSetSessionWorkflow)
	mux.HandleFunc("GET /api/sessions/{id}/workers", s.handleListWorkers)
	// Coordinator TREE navigation: /tree takes any member id and returns the whole
	// tree from its root; /ancestors is the upward breadcrumb from a worker.
	mux.HandleFunc("GET /api/sessions/{id}/coordinator-tree", s.handleSessionCoordinatorTree)
	mux.HandleFunc("GET /api/sessions/{id}/coordinator-ancestors", s.handleSessionCoordinatorAncestors)
	mux.HandleFunc("GET /api/sessions/{id}/workdir", s.handleGetSessionWorkdir)
	mux.HandleFunc("PUT /api/sessions/{id}/workdir", s.handleSetSessionWorkdir)
	mux.HandleFunc("GET /api/fs/browse", s.handleBrowseDirs)
	mux.HandleFunc("GET /api/fs/gitinfo", s.handleGitInfo)
	mux.HandleFunc("POST /api/git/init", s.handleGitInit)
	mux.HandleFunc("POST /api/git/config", s.handleGitConfig)
	mux.HandleFunc("POST /api/sessions/{id}/summary", s.handleSessionSummary)
	mux.HandleFunc("POST /api/sessions/{id}/handoff", s.handleSessionHandoff)
	mux.HandleFunc("POST /api/sessions/{id}/read", s.handleMarkSessionRead)
	mux.HandleFunc("GET /api/sessions/{id}/info", s.handleSessionInfo)
	mux.HandleFunc("GET /api/sessions/{id}/progress", s.handleSessionProgress)
	// Debug: preview the EXACT next-turn context (system + dynamic + transcript +
	// tools) this session's agent would be sent. Optional ?message= sample turn.
	mux.HandleFunc("GET /api/sessions/{id}/context-preview", s.handleSessionContextPreview)
	mux.HandleFunc("GET /api/sessions/{id}/path", s.handleSessionPath)
}

// envTruthy reports whether an env value opts a feature in (1/true/on/yes).
func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// loopbackGuard wraps a handler so that, when the external gateway has no auth token,
// only loopback clients may reach it (no token → no network exposure). With a token
// configured the guard is a pass-through and the handler's own bearer check applies.
func (s *Server) loopbackGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.gatewayRequireLoopback && !isLoopbackAddr(r.RemoteAddr) {
			http.Error(w, "external gateway is loopback-only (set TIONHARNESS_GATEWAY_AUTH_TOKEN to expose it)", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackAddr reports whether a "host:port" remote address is a loopback client.
func isLoopbackAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	return ip != nil && ip.IsLoopback()
}

// registerChatRoutes registers the completion endpoints.
func (s *Server) registerChatRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/chat", s.handleChat)
	// SSE streaming variant: emits each activity step as it occurs.
	mux.HandleFunc("POST /api/chat/stream", s.handleChatStream)
	// Control an in-flight streaming turn: stop (cancel) or steer (live guidance).
	mux.HandleFunc("POST /api/chat/control", s.handleChatControl)
	// Side chat ("btw"): a tool-less one-shot question answered against the
	// session's context WITHOUT being written into its history. Answerable while
	// the main turn is still streaming. See _Docs/60-BTW-YAN-SOHBET.md.
	mux.HandleFunc("POST /api/chat/btw", s.handleChatBtw)
	// Disarm a pending one-shot self-wake (schedule_wake) for a session — the
	// user pressed "Durdur" on the waiting banner before the wake fired.
	mux.HandleFunc("POST /api/chat/wake/cancel", s.handleCancelWake)
	// Interaction MCP endpoint: CLI agents (claude-cli, ...) call TionHarness's
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
	// External MCP gateway (Doc 52 Faz 3), opt-in. Loopback-guarded when no token is set.
	if s.gatewaySrv != nil {
		h := s.loopbackGuard(s.gatewaySrv)
		mux.Handle("/mcp/gateway", h)
		mux.Handle("/mcp/gateway/", h)
	}
}

// registerTaskRoutes registers the kanban board + run history.
func (s *Server) registerTaskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/archive", s.handleArchiveTask)
	mux.HandleFunc("POST /api/tasks/{id}/unarchive", s.handleUnarchiveTask)
	mux.HandleFunc("POST /api/tasks/{id}/title", s.handleGenerateTaskTitle)
	mux.HandleFunc("POST /api/tasks/{id}/{subpath...}", s.handleUnknownTaskSubpath)
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
	mux.HandleFunc("POST /api/schedules/{id}/generate-title", s.handleGenerateScheduleTitle)
	// Tag-triggered automations (event-driven loops), surfaced in the Schedules UI.
	mux.HandleFunc("GET /api/automations/live-stats", s.handleAutomationLiveStats)
	mux.HandleFunc("GET /api/automations", s.handleListAutomations)
	mux.HandleFunc("POST /api/automations", s.handleCreateAutomation)
	mux.HandleFunc("PUT /api/automations/{id}", s.handleUpdateAutomation)
	mux.HandleFunc("POST /api/automations/{id}/toggle", s.handleToggleAutomation)
	mux.HandleFunc("POST /api/automations/{id}/reset", s.handleResetAutomation)
	mux.HandleFunc("DELETE /api/automations/{id}", s.handleDeleteAutomation)
	mux.HandleFunc("POST /api/automations/{id}/generate-title", s.handleGenerateAutomationTitle)
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
	mux.HandleFunc("GET /api/mcp-servers/importable", s.handleImportableMCPServers)
	mux.HandleFunc("POST /api/mcp-servers/importable/add", s.handleAddImportableMCPServer)
	mux.HandleFunc("GET /api/mcp-servers/pool", s.handleMCPPoolStats)
	mux.HandleFunc("PATCH /api/mcp-servers/{id}", s.handleUpdateMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/toggle", s.handleToggleMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/test", s.handleTestMCPServer)
	mux.HandleFunc("DELETE /api/mcp-servers/{id}", s.handleDeleteMCPServer)
	mux.HandleFunc("GET /api/agents/{id}/tools", s.handleAgentTools)
	mux.HandleFunc("POST /api/agents/{id}/tools", s.handleSetAgentTools)
	// Read-only "what can this agent actually use right now" view: eager vs lazy
	// (gateway-activatable) split + MCP server inventory. Drives the composer's
	// tool inspector; changes nothing.
	mux.HandleFunc("GET /api/agents/{id}/tool-access", s.handleAgentToolAccess)
	mux.HandleFunc("GET /api/workspace-tools", s.handleWorkspaceTools)
	mux.HandleFunc("PUT /api/workspace-tools", s.handleSetWorkspaceTools)
}

// registerHookRoutes registers PreToolUse/PostToolUse hooks (Phase P4) plus the
// failure-lesson store endpoints (self-healing, read + prune).
func (s *Server) registerHookRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/lessons", s.handleListLessons)
	mux.HandleFunc("DELETE /api/lessons/{id}", s.handleDeleteLesson)
	// Retrospective session scanning (Insight, _Docs/60).
	mux.HandleFunc("GET /api/insight/lenses", s.handleListInsightLenses)
	mux.HandleFunc("POST /api/insight/scan", s.handleInsightScan)
	mux.HandleFunc("GET /api/insight/status", s.handleInsightScanStatus)
	mux.HandleFunc("GET /api/insight/runs", s.handleInsightRuns)
	mux.HandleFunc("GET /api/insight/fleet-findings", s.handleInsightFleetFindings)
	mux.HandleFunc("GET /api/insight/lenses/{id}/raw", s.handleGetInsightLensRaw)
	mux.HandleFunc("PUT /api/insight/lenses/{id}", s.handleUpdateInsightLens)
	mux.HandleFunc("POST /api/insight/lenses/{id}/toggle", s.handleToggleInsightLens)
	mux.HandleFunc("POST /api/insight/lenses/{id}/restore", s.handleRestoreInsightLens)
	mux.HandleFunc("GET /api/insight/findings", s.handleListInsightFindings)
	mux.HandleFunc("POST /api/insight/findings/{id}/status", s.handleSetInsightFindingStatus)
	mux.HandleFunc("DELETE /api/insight/findings/{id}", s.handleDeleteInsightFinding)
	mux.HandleFunc("POST /api/insight/reset", s.handleResetInsight)
	mux.HandleFunc("GET /api/insight/settings", s.handleGetInsightSettings)
	mux.HandleFunc("PUT /api/insight/settings", s.handleUpdateInsightSettings)
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
	mux.HandleFunc("GET /api/flows/{id}", s.handleGetFlow)
	mux.HandleFunc("POST /api/flows", s.handleCreateFlow)
	mux.HandleFunc("PUT /api/flows/{id}", s.handleUpdateFlow)
	mux.HandleFunc("PUT /api/flows/{id}/tags", s.handleSetFlowTags)
	mux.HandleFunc("PUT /api/flows/{id}/emoji", s.handleSetFlowEmoji)
	mux.HandleFunc("DELETE /api/flows/{id}", s.handleDeleteFlow)
	// Locate the flow on disk (copy-path action).
	mux.HandleFunc("GET /api/flows/{id}/path", s.handleFlowPath)
	mux.HandleFunc("POST /api/flows/{id}/run", s.handleRunFlow)
	mux.HandleFunc("POST /api/flows/{id}/run-stream", s.handleRunFlowStream)
	mux.HandleFunc("GET /api/flow-runs", s.handleListFlowRuns)
	mux.HandleFunc("GET /api/flow-runs/{id}", s.handleGetFlowRun)
	mux.HandleFunc("GET /api/flow-runs/{id}/nodes/{nodeId}/steps", s.handleFlowRunNodeSteps)
	// The whole tree a composed run belongs to (root + subflow/spawn descendants).
	mux.HandleFunc("GET /api/flow-runs/{id}/tree", s.handleFlowRunTree)
	// Deliver input to a run suspended at an await-input node (durable resume).
	mux.HandleFunc("POST /api/flow-runs/{id}/input", s.handleResumeFlowRun)

	// Projection layer: the compact, context-cheap summary of a large entity —
	// the same bytes the agent gets and the Bağlam panel shows (_Docs/66).
	mux.HandleFunc("GET /api/views/{kind}/{id}", s.handleGetView)
	// Explorer map drill-down: the structural child handles of a node (_Docs/68).
	mux.HandleFunc("GET /api/views/{kind}/{id}/children", s.handleGetViewChildren)
	// Explorer focus graph: complete direct parents + children, without a
	// backend presentation cap. The UI owns visual overflow (_Docs/68).
	mux.HandleFunc("GET /api/views/{kind}/{id}/neighborhood", s.handleGetViewNeighborhood)

	// Workspace overview: counters + chart series + the workspace projection,
	// in one call (_Docs/66).
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	mux.HandleFunc("GET /api/dashboard/commit-activity", s.handleDashboardCommitActivity)
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
	mux.HandleFunc("GET /api/artifacts/{id}/source", s.handleArtifactSource)
	mux.HandleFunc("PUT /api/artifacts/{id}", s.handleUpdateArtifact)
	mux.HandleFunc("DELETE /api/artifacts/{id}", s.handleDeleteArtifact)
	// Assign an artifact's organisation bucket (Artifacts-UI grouping).
	mux.HandleFunc("PUT /api/artifacts/{id}/group", s.handleSetArtifactGroup)
	// Archive / un-archive an artifact (a soft, reversible hide).
	mux.HandleFunc("PUT /api/artifacts/{id}/archive", s.handleSetArtifactArchived)
	// Locate the artifact on disk (copy-path action).
	mux.HandleFunc("GET /api/artifacts/{id}/path", s.handleArtifactPath)

	// Inbound-message hold queue (agent_messages.go): list what a "hold" policy
	// has parked, then release (deliver now) or refuse it.
	mux.HandleFunc("GET /api/agent-messages/held", s.handleListHeldAgentMessages)
	mux.HandleFunc("POST /api/agent-messages/{id}/release", s.handleReleaseAgentMessage)
	mux.HandleFunc("POST /api/agent-messages/{id}/refuse", s.handleRefuseAgentMessage)
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
	mux.HandleFunc("POST /api/skills/{slug}/restore", s.handleRestoreSkill)
}

// registerSettingsRoutes registers the global application settings document.
func (s *Server) registerSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("POST /api/settings/test-provider", s.handleTestProvider)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/prompts", s.handleListPrompts)
	// Provider instances (providers.json, _Docs/71) — user-configured accounts
	// bound to a registered provider kind.
	mux.HandleFunc("GET /api/provider-kinds", s.handleListProviderKinds)
	mux.HandleFunc("GET /api/providers", s.handleListProviders)
	mux.HandleFunc("GET /api/providers/{id}", s.handleGetProvider)
	mux.HandleFunc("PUT /api/providers", s.handleUpsertProvider)
	mux.HandleFunc("DELETE /api/providers/{id}", s.handleDeleteProvider)
	mux.HandleFunc("GET /api/providers/{id}/auth", s.handleProviderAuth)
	mux.HandleFunc("POST /api/providers/{id}/auth/oauth/start", s.handleProviderClaudeOAuthStart)
	mux.HandleFunc("POST /api/providers/{id}/auth/oauth/complete", s.handleProviderClaudeOAuthComplete)
	mux.HandleFunc("POST /api/providers/{id}/auth/oauth/loopback/start", s.handleProviderClaudeOAuthLoopbackStart)
	mux.HandleFunc("GET /api/providers/{id}/auth/oauth/loopback/status", s.handleProviderClaudeOAuthLoopbackStatus)
	mux.HandleFunc("POST /api/providers/{id}/auth/device/start", s.handleProviderCodexDeviceStart)
	mux.HandleFunc("GET /api/providers/{id}/auth/device/status", s.handleProviderCodexDeviceStatus)
	mux.HandleFunc("POST /api/providers/{id}/auth/device/cancel", s.handleProviderCodexDeviceCancel)
	mux.HandleFunc("POST /api/providers/{id}/auth/api-key", s.handleProviderCodexAPIKeyLogin)
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
	// On-disk log file path (for the copy-path action).
	mux.HandleFunc("GET /api/logs/path", s.handleLogsPath)
	// Frontend error bridge: client-side crashes/rejections funnel into the log
	// stream so they surface in the Logs screen, not just the browser console.
	mux.HandleFunc("POST /api/logs", s.handleClientLog)
	// Autonomous event feed (task/schedule) — SSE, global.
	mux.HandleFunc("GET /api/events", s.handleEvents)
	// Detect optional external tools on this host and read the version each one
	// reports — surfaced by the Settings diagnostics panel. Path resolution runs
	// nothing; the version probe runs only `<tool> --version` (3s cap).
	mux.HandleFunc("GET /api/external-tools", s.handleExternalTools)
	// Upstream release check (GitHub API, 6h disk cache). Split from detection
	// because it leaves the machine and must not block the panel's first paint.
	mux.HandleFunc("POST /api/external-tools/check-updates", s.handleExternalToolUpdates)
	// Run one tool's update. Only package-manager-backed tools are accepted; the
	// rest answer 409 with manual instructions.
	mux.HandleFunc("POST /api/external-tools/{name}/update", s.handleExternalToolUpdate)
	// Maintenance ACTIONS for the token optimizers (not settings — their config is
	// machine-global while TionHarness settings are per-workspace; see
	// external_tools_maint.go). Fixed-argv commands, no request parameters.
	mux.HandleFunc("GET /api/external-tools/token-report", s.handleTokenToolReport)
	mux.HandleFunc("POST /api/external-tools/sqz-reset-cache", s.handleSqzResetCache)
}

// workspaceQueryKeys are the query parameters that scope a request to a workspace,
// for clients that cannot set headers (an <img src> loading an inline attachment) and
// for external automation driving the API by URL alone. "ws" is the original; the
// longer aliases exist because they are the obvious guesses and a silently-ignored
// scope param is worse than an unknown one — it serves ANOTHER workspace's data under
// the caller's id (see workspaceIDFromRequest).
var workspaceQueryKeys = []string{"ws", "workspace", "workspaceId", "workspace_id"}

// workspaceIDFromRequest returns the requested workspace id and whether it was named
// EXPLICITLY in the query string. The distinction drives the unknown-id policy in
// withWorkspace: a header id may be stale (localStorage surviving a deleted
// workspace) and must degrade gracefully, while a query id is a deliberate,
// per-request scope whose silent replacement would hand back the wrong workspace.
func workspaceIDFromRequest(r *http.Request) (id string, fromQuery bool) {
	q := r.URL.Query()
	for _, k := range workspaceQueryKeys {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			return v, true
		}
	}
	return strings.TrimSpace(r.Header.Get("X-Workspace-Id")), false
}

// withWorkspace resolves the active workspace from an explicit ?ws=/?workspace= query
// param or the X-Workspace-Id header (falling back to the default) and injects it into
// the request context. The resolved id is echoed back in the X-Workspace-Id response
// header so a caller can always tell WHICH workspace answered rather than assuming.
func (s *Server) withWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, fromQuery := workspaceIDFromRequest(r)
		var ws *workspace.Workspace
		if id != "" {
			found, err := s.workspaces.Get(id)
			switch {
			case err == nil:
				ws = found
			case fromQuery:
				// An explicitly-scoped request naming a workspace that does not exist is
				// an error, never a redirect: answering it from the default workspace
				// returns another workspace's data under the caller's id — the caller
				// reads it as authoritative and can act (or write) on the wrong store.
				writeError(w, http.StatusBadRequest, "unknown workspace "+id)
				return
			default:
				// Header path: the id may simply be stale (localStorage outliving a
				// deleted workspace), so the app recovers on the default instead of
				// bricking on 400s.
				s.logger.Warn("api: unknown workspace in X-Workspace-Id; serving default",
					"requested", id, "path", r.URL.Path)
				ws = s.workspaces.Default()
			}
		} else {
			ws = s.workspaces.Default()
		}
		// Fresh install / all workspaces deleted: no active workspace exists. A
		// workspace-scoped handler would deref this nil (recovered into a noisy
		// 500 by withRecover). Short-circuit with a clean, self-explanatory 409
		// instead — except for the bootstrap routes (workspace CRUD, templates,
		// folder picker) that MUST work precisely when there is no workspace yet.
		if ws == nil && !workspaceOptionalPath(r.URL.Path) {
			writeError(w, http.StatusConflict, "no active workspace — create one first")
			return
		}
		// Echo the workspace that actually served the request: without it a caller
		// whose scope silently fell back to the default cannot tell whose data it is
		// holding (the failure mode this whole block exists to prevent).
		if ws != nil {
			w.Header().Set("X-Workspace-Id", ws.ID)
		}
		ctx := context.WithValue(r.Context(), workspaceCtxKey, ws)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// workspaceOptionalPath reports whether a route must work with NO active
// workspace — the first-run bootstrap surface (create/list/attach a workspace,
// browse templates, pick a folder) plus process-global infra that never reads a
// workspace. Everything else is workspace-scoped and gets a clean 409 when none
// exists, rather than dereferencing a nil workspace.
func workspaceOptionalPath(path string) bool {
	switch {
	case path == "/api/workspaces" || strings.HasPrefix(path, "/api/workspaces/"):
		return true
	case path == "/api/workspace-templates":
		return true
	case path == "/api/pick-folder":
		return true
	case path == "/api/external-tools" || strings.HasPrefix(path, "/api/external-tools/"):
		return true
	case path == "/api/events":
		return true
	case path == "/health":
		return true
	case path == "/api/version" || path == "/api/version/update":
		return true
	case path == "/api/debug/store-stats":
		return true
	}
	return false
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

// decodeJSONStrict is decodeJSON with unknown JSON fields rejected instead of
// silently dropped. See bindJSONStrict for why this is opt-in per endpoint
// rather than the default.
func decodeJSONStrict(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
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
		"service":    "tionharness",
		"version":    "0.0.1",
		"workspaces": len(s.workspaces.List()),
	})
}
