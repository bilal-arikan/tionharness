// Package api exposes the SwarmGo HTTP/JSON interface.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/providers"
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
	logger     *slog.Logger
}

// NewServer constructs an API server.
func NewServer(manager *workspace.Manager, registry *providers.Registry, logger *slog.Logger) *Server {
	return &Server{
		workspaces: manager,
		providers:  registry,
		convo:      conversation.NewManager(),
		logger:     logger,
	}
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

	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)

	mux.HandleFunc("POST /api/chat", s.handleChat)

	mux.HandleFunc("GET /api/runtime", s.handleRuntimeStatus)
	mux.HandleFunc("POST /api/agents/{id}/heartbeat", s.handleSetHeartbeat)
	mux.HandleFunc("POST /api/agents/{id}/wake", s.handleWake)

	// Tasks (kanban board) + runs.
	mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	mux.HandleFunc("POST /api/tasks", s.handleCreateTask)
	mux.HandleFunc("PUT /api/tasks/{id}", s.handleUpdateTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.handleDeleteTask)
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
