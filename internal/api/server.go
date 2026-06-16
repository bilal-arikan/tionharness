// Package api exposes the SwarmGo HTTP/JSON interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/interaction"
	"github.com/bilal/swarmgo/internal/logbuf"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/settings"
	"github.com/bilal/swarmgo/internal/workspace"
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
	bus        *events.Bus // autonomous notifications streamed to the UI over SSE
	runs       *chatRuns   // in-flight streaming turns (stop/steer control)
	logger     *slog.Logger

	// selfURL is this server's own loopback base URL (e.g. http://127.0.0.1:8090),
	// used to point CLI subprocesses at the in-process Interaction MCP endpoint.
	selfURL string
	// interactionMCP serves the Interaction MCP endpoint (/mcp/interaction).
	interactionMCP http.Handler
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
		logger:     logger,
	}
	// Interaction MCP: lets CLI agents (claude-cli, ...) reach SwarmGo's
	// human-in-the-loop tools over in-process HTTP. See _Docs/11-INTERACTION-MCP.md.
	s.interactionMCP = interaction.Handler(&interactionBackend{runs: s.runs}, logger)
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
	s.providers.SetDefaultModel(cur.DefaultModel)
	s.providers.SetAnthropicBetas(cur.OneMillionContext, cur.ExtendedPromptCache)
	s.providers.SetMinimax(s.settings.MinimaxKey(), cur.MinimaxBaseURL)
	s.convo.SetLimits(cur.MaxContextTokens, cur.KeepRecentMsgs)
	s.tun.SetAutonomyPaused(cur.PauseAutonomy)
	s.tun.SetTitleModel(cur.TitleModel)
	s.tun.SetJournalLimits(cur.JournalCap, cur.JournalMaxLen)
	s.tun.SetAutoReflect(cur.AutoReflect, cur.AutoReflectThreshold)
}

// Routes registers all HTTP routes and returns the handler. Registration is
// split into per-domain helpers (each lives beside its handlers) so the route
// table stays readable as the surface grows.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)

	s.registerWorkspaceRoutes(mux)
	s.registerAgentRoutes(mux)
	s.registerSessionRoutes(mux)
	s.registerChatRoutes(mux)
	s.registerRuntimeRoutes(mux)
	s.registerTaskRoutes(mux)
	s.registerScheduleRoutes(mux)
	s.registerUsageRoutes(mux)
	s.registerMCPRoutes(mux)
	s.registerFlowRoutes(mux)
	s.registerArtifactRoutes(mux)
	s.registerSettingsRoutes(mux)
	s.registerMemoryRoutes(mux)
	s.registerMiscRoutes(mux)

	return withCORS(s.withRequestLog(s.withWorkspace(mux)))
}

// registerWorkspaceRoutes registers workspace management (not workspace-scoped).
func (s *Server) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.handleCreateWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{id}", s.handleDeleteWorkspace)
	// Native folder picker (local desktop) for choosing a workspace data dir.
	mux.HandleFunc("POST /api/pick-folder", s.handlePickFolder)

	// Per-workspace settings (resolved from X-Workspace-Id).
	mux.HandleFunc("GET /api/workspace-settings", s.handleGetWorkspaceSettings)
	mux.HandleFunc("PUT /api/workspace-settings", s.handleUpdateWorkspaceSettings)

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
}

// registerSessionRoutes registers chat sessions + messages + titling.
func (s *Server) registerSessionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("GET /api/sessions/active", s.handleActiveSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)
	mux.HandleFunc("DELETE /api/sessions/{id}/messages/{msgId}", s.handleDeleteMessage)
	mux.HandleFunc("POST /api/sessions/{id}/title", s.handleGenerateSessionTitle)
	mux.HandleFunc("POST /api/sessions/{id}/summary", s.handleSessionSummary)
	mux.HandleFunc("POST /api/sessions/{id}/read", s.handleMarkSessionRead)
	mux.HandleFunc("GET /api/sessions/{id}/info", s.handleSessionInfo)
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
	// Interaction MCP endpoint: CLI agents (claude-cli, ...) call SwarmGo's
	// human-in-the-loop tools (ask_user/todo_write) here over MCP-over-HTTP.
	// Bound to all methods; the handler does its own bearer auth + method switch.
	// Guarded so a bare &Server{} (route-registration test) doesn't panic on a
	// nil handler; NewServer always installs it.
	if s.interactionMCP != nil {
		mux.Handle("/mcp/interaction", s.interactionMCP)
	}
}

// registerRuntimeRoutes registers autonomous runtime control + status.
func (s *Server) registerRuntimeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/runtime", s.handleRuntimeStatus)
	mux.HandleFunc("POST /api/agents/{id}/heartbeat", s.handleSetHeartbeat)
	mux.HandleFunc("POST /api/agents/{id}/wake", s.handleWake)
}

// registerTaskRoutes registers the kanban board + run history.
func (s *Server) registerTaskRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/title", s.handleGenerateTaskTitle)
	mux.HandleFunc("POST /api/tasks/{id}/run", s.handleRunTask)
	mux.HandleFunc("GET /api/tasks/{id}/runs", s.handleListTaskRuns)
}

// registerScheduleRoutes registers cron schedules.
func (s *Server) registerScheduleRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/schedules", s.handleListSchedules)
	mux.HandleFunc("POST /api/schedules", s.handleCreateSchedule)
	mux.HandleFunc("PUT /api/schedules/{id}", s.handleUpdateSchedule)
	mux.HandleFunc("POST /api/schedules/{id}/toggle", s.handleToggleSchedule)
	mux.HandleFunc("DELETE /api/schedules/{id}", s.handleDeleteSchedule)
}

// registerUsageRoutes registers budget guardrails + the context meter.
func (s *Server) registerUsageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents/{id}/usage", s.handleAgentUsage)
	mux.HandleFunc("POST /api/agents/{id}/budget", s.handleSetBudget)
	mux.HandleFunc("GET /api/sessions/{id}/context", s.handleSessionContext)
}

// registerMCPRoutes registers MCP servers + per-agent tool access (Phase 8).
func (s *Server) registerMCPRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/mcp-servers", s.handleListMCPServers)
	mux.HandleFunc("POST /api/mcp-servers", s.handleCreateMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/toggle", s.handleToggleMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/test", s.handleTestMCPServer)
	mux.HandleFunc("DELETE /api/mcp-servers/{id}", s.handleDeleteMCPServer)
	mux.HandleFunc("GET /api/agents/{id}/tools", s.handleAgentTools)
	mux.HandleFunc("POST /api/agents/{id}/tools", s.handleSetAgentTools)
	mux.HandleFunc("GET /api/workspace-tools", s.handleWorkspaceTools)
	mux.HandleFunc("PUT /api/workspace-tools", s.handleSetWorkspaceTools)
}

// registerFlowRoutes registers orchestration flows + runs (Phase 7).
func (s *Server) registerFlowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/flows", s.handleListFlows)
	mux.HandleFunc("POST /api/flows", s.handleCreateFlow)
	mux.HandleFunc("PUT /api/flows/{id}", s.handleUpdateFlow)
	mux.HandleFunc("DELETE /api/flows/{id}", s.handleDeleteFlow)
	mux.HandleFunc("POST /api/flows/{id}/run", s.handleRunFlow)
	mux.HandleFunc("GET /api/flow-runs", s.handleListFlowRuns)
	mux.HandleFunc("GET /api/flow-runs/{id}", s.handleGetFlowRun)
}

// registerArtifactRoutes registers the artifact store (versioned agent-produced
// content viewed in a dedicated screen).
func (s *Server) registerArtifactRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/artifacts", s.handleListArtifacts)
	mux.HandleFunc("POST /api/artifacts", s.handleCreateArtifact)
	mux.HandleFunc("GET /api/artifacts/{id}", s.handleGetArtifact)
	mux.HandleFunc("PUT /api/artifacts/{id}", s.handleUpdateArtifact)
	mux.HandleFunc("DELETE /api/artifacts/{id}", s.handleDeleteArtifact)
}

// registerSettingsRoutes registers the global application settings document.
func (s *Server) registerSettingsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("POST /api/settings/test-provider", s.handleTestProvider)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)
	mux.HandleFunc("GET /api/prompts", s.handleListPrompts)
	mux.HandleFunc("POST /api/prompts/reveal", s.handleRevealPrompts)
}

// registerMemoryRoutes registers per-agent knowledge (documents, journal,
// reflections).
func (s *Server) registerMemoryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/agents/{id}/memories", s.handleListMemories)
	mux.HandleFunc("POST /api/agents/{id}/memories", s.handleCreateMemory)
	mux.HandleFunc("POST /api/agents/{id}/reflect", s.handleReflect)
	mux.HandleFunc("POST /api/agents/{id}/recall", s.handleRecall)
	mux.HandleFunc("DELETE /api/memories/{id}", s.handleDeleteMemory)
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
	// Autonomous event feed (heartbeat/task/schedule) — SSE, global.
	mux.HandleFunc("GET /api/events", s.handleEvents)
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
		"service":    "swarmgo",
		"version":    "0.0.1",
		"workspaces": len(s.workspaces.List()),
	})
}
