// Package api exposes the SwarmGo HTTP/JSON interface.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
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
	logger     *slog.Logger
}

// NewServer constructs an API server and pushes the persisted settings into the
// live subsystems (providers, compaction, autonomy). tun is the shared
// process-wide tunables updated whenever settings change.
func NewServer(manager *workspace.Manager, registry *providers.Registry, store *settings.Store, tun *agent.Tunables, logs *logbuf.Buffer, logger *slog.Logger) *Server {
	s := &Server{
		workspaces: manager,
		providers:  registry,
		convo:      conversation.NewManager(),
		settings:   store,
		tun:        tun,
		logs:       logs,
		logger:     logger,
	}
	s.applySettings()
	return s
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
}

// Routes registers all HTTP routes and returns the handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)

	// Workspace management (not workspace-scoped).
	mux.HandleFunc("GET /api/workspaces", s.handleListWorkspaces)
	mux.HandleFunc("POST /api/workspaces", s.handleCreateWorkspace)
	mux.HandleFunc("DELETE /api/workspaces/{id}", s.handleDeleteWorkspace)

	// Workspace-scoped resources.
	mux.HandleFunc("GET /api/agents", s.handleListAgents)
	mux.HandleFunc("POST /api/agents", s.handleCreateAgent)
	mux.HandleFunc("PUT /api/agents/{id}", s.handleUpdateAgent)

	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)
	mux.HandleFunc("POST /api/sessions/{id}/title", s.handleGenerateSessionTitle)

	mux.HandleFunc("POST /api/chat", s.handleChat)
	// SSE streaming variant: emits each activity step as it occurs.
	mux.HandleFunc("POST /api/chat/stream", s.handleChatStream)

	// Inline media (images referenced by chat content) — read-only.
	mux.HandleFunc("GET /api/files", s.handleServeFile)

	mux.HandleFunc("GET /api/runtime", s.handleRuntimeStatus)
	mux.HandleFunc("POST /api/agents/{id}/heartbeat", s.handleSetHeartbeat)
	mux.HandleFunc("POST /api/agents/{id}/wake", s.handleWake)

	// Tasks (kanban board) + runs.
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDeleteTask)
	mux.HandleFunc("POST /api/tasks/{id}/title", s.handleGenerateTaskTitle)
	mux.HandleFunc("POST /api/tasks/{id}/run", s.handleRunTask)
	mux.HandleFunc("GET /api/tasks/{id}/runs", s.handleListTaskRuns)

	// Schedules (cron).
	mux.HandleFunc("GET /api/schedules", s.handleListSchedules)
	mux.HandleFunc("POST /api/schedules", s.handleCreateSchedule)
	mux.HandleFunc("POST /api/schedules/{id}/toggle", s.handleToggleSchedule)
	mux.HandleFunc("DELETE /api/schedules/{id}", s.handleDeleteSchedule)

	// Usage / budget guardrails + context meter.
	mux.HandleFunc("GET /api/agents/{id}/usage", s.handleAgentUsage)
	mux.HandleFunc("POST /api/agents/{id}/budget", s.handleSetBudget)
	mux.HandleFunc("GET /api/sessions/{id}/context", s.handleSessionContext)

	// MCP servers + per-agent tool access (Phase 8).
	mux.HandleFunc("GET /api/mcp-servers", s.handleListMCPServers)
	mux.HandleFunc("POST /api/mcp-servers", s.handleCreateMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/toggle", s.handleToggleMCPServer)
	mux.HandleFunc("POST /api/mcp-servers/{id}/test", s.handleTestMCPServer)
	mux.HandleFunc("DELETE /api/mcp-servers/{id}", s.handleDeleteMCPServer)
	mux.HandleFunc("GET /api/agents/{id}/tools", s.handleAgentTools)
	mux.HandleFunc("POST /api/agents/{id}/tools", s.handleSetAgentTools)

	// Orchestration flows (multi-agent protocols) + runs (Phase 7).
	mux.HandleFunc("GET /api/flows", s.handleListFlows)
	mux.HandleFunc("POST /api/flows", s.handleCreateFlow)
	mux.HandleFunc("PUT /api/flows/{id}", s.handleUpdateFlow)
	mux.HandleFunc("DELETE /api/flows/{id}", s.handleDeleteFlow)
	mux.HandleFunc("POST /api/flows/{id}/run", s.handleRunFlow)
	mux.HandleFunc("GET /api/flow-runs", s.handleListFlowRuns)
	mux.HandleFunc("GET /api/flow-runs/{id}", s.handleGetFlowRun)

	// Application settings (global, single document).
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PUT /api/settings", s.handleUpdateSettings)
	mux.HandleFunc("POST /api/settings/test-provider", s.handleTestProvider)
	mux.HandleFunc("GET /api/catalog", s.handleCatalog)

	// Per-workspace settings (resolved from X-Workspace-Id).
	mux.HandleFunc("GET /api/workspace-settings", s.handleGetWorkspaceSettings)
	mux.HandleFunc("PUT /api/workspace-settings", s.handleUpdateWorkspaceSettings)

	// Application + workspace logs (global ring buffer).
	mux.HandleFunc("GET /api/logs", s.handleListLogs)

	// Memory (per-agent knowledge: documents, journal, reflections).
	mux.HandleFunc("GET /api/agents/{id}/memories", s.handleListMemories)
	mux.HandleFunc("POST /api/agents/{id}/memories", s.handleCreateMemory)
	mux.HandleFunc("POST /api/agents/{id}/reflect", s.handleReflect)
	mux.HandleFunc("POST /api/agents/{id}/recall", s.handleRecall)
	mux.HandleFunc("DELETE /api/memories/{id}", s.handleDeleteMemory)

	return withCORS(s.withWorkspace(mux))
}

// withWorkspace resolves the active workspace from the X-Workspace-Id header
// (falling back to the default) and injects it into the request context.
func (s *Server) withWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Workspace-Id")
		var ws *workspace.Workspace
		if id != "" {
			found, err := s.workspaces.Get(id)
			if err != nil {
				writeError(w, http.StatusBadRequest, "unknown workspace: "+id)
				return
			}
			ws = found
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
