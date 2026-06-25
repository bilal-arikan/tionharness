package api

import (
	"context"
	"net/http"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
)

// workspaceSettingsDTO is the client view of a workspace's editable settings,
// combining its registry name with its per-workspace overrides and live stats.
type workspaceSettingsDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Instructions    string `json:"instructions"`
	Icon            string `json:"icon"`
	Color           string `json:"color"`
	DefaultProvider   string `json:"defaultProvider"`
	DefaultModel      string `json:"defaultModel"`
	PauseAutonomy     bool   `json:"pauseAutonomy"`
	DefaultWorkingDir string `json:"defaultWorkingDir"`
	CreatedAt         int64  `json:"createdAt"`

	// Per-workspace appearance overrides (empty = inherit global).
	Theme       string `json:"theme"`
	Accent      string `json:"accent"`
	ThemePreset string `json:"themePreset"`

	SessionContextEnabled     bool `json:"sessionContextEnabled"`
	SessionContextEveryTurn   bool `json:"sessionContextEveryTurn"`
	SessionContextRecentCount int  `json:"sessionContextRecentCount"`

	// BoardColumns is the ordered column set for this workspace's kanban board.
	// Always non-nil: falls back to db.DefaultBoardColumns() when unconfigured.
	BoardColumns []db.BoardColumnDef `json:"boardColumns"`

	AgentCount   int `json:"agentCount"`
	SessionCount int `json:"sessionCount"`
	TaskCount    int `json:"taskCount"`
}

func toWorkspaceSettingsDTO(ctx context.Context, w *workspace.Workspace) workspaceSettingsDTO {
	s := w.Settings()
	cols := s.BoardColumns
	if len(cols) == 0 {
		cols = db.DefaultBoardColumns()
	}
	dto := workspaceSettingsDTO{
		ID:              w.ID,
		Name:            w.Name,
		Instructions:    s.Instructions,
		Icon:            s.Icon,
		Color:           s.Color,
		DefaultProvider:   s.DefaultProvider,
		DefaultModel:      s.DefaultModel,
		PauseAutonomy:     s.PauseAutonomy,
		DefaultWorkingDir: s.DefaultWorkingDir,
		CreatedAt:         w.CreatedAt,

		Theme:       s.Theme,
		Accent:      s.Accent,
		ThemePreset: s.ThemePreset,

		SessionContextEnabled:     s.SessionContextEnabled,
		SessionContextEveryTurn:   s.SessionContextEveryTurn,
		SessionContextRecentCount: s.SessionContextRecentCount,

		BoardColumns: cols,
	}
	if agents, err := w.DB.ListAgents(ctx); err == nil {
		dto.AgentCount = len(agents)
	}
	if sessions, err := w.DB.ListSessions(ctx, ""); err == nil {
		dto.SessionCount = len(sessions)
	}
	if tasks, err := w.DB.ListTasks(ctx); err == nil {
		dto.TaskCount = len(tasks)
	}
	return dto
}

// handleGetWorkspaceSettings returns the active workspace's settings.
func (s *Server) handleGetWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, toWorkspaceSettingsDTO(r.Context(), ws(r)))
}

// handleUpdateWorkspaceSettings applies a partial update (including rename) to
// the active workspace and persists it.
func (s *Server) handleUpdateWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	var patch workspace.WSSettingsPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	updated, err := s.workspaces.UpdateSettings(ws(r).ID, patch)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceSettingsDTO(r.Context(), updated))
}
