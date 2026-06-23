package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bilal-arikan/swarmgo/internal/agent"
	"github.com/bilal-arikan/swarmgo/internal/workspace"
)

// wsConfigDTO is the client view of a workspace's editable config files: the
// utility prompts (with their compiled-in defaults for "reset"), the workspace
// instructions and the free-form README. All live under <workspace>/config/.
type wsConfigDTO struct {
	Dir          string            `json:"dir"`
	Prompts      map[string]string `json:"prompts"`  // key → current file content
	Defaults     map[string]string `json:"defaults"` // key → compiled-in default
	PromptKeys   []string          `json:"promptKeys"`
	Instructions string            `json:"instructions"`
	Readme       string            `json:"readme"`
}

// wsConfigPatch is a partial update; omitted fields are left unchanged. A prompt
// entry written as "" clears the file so the compiled-in default takes over.
type wsConfigPatch struct {
	Prompts      map[string]string `json:"prompts"`
	Instructions *string           `json:"instructions"`
	Readme       *string           `json:"readme"`
}

// readFileOr returns a file's content, or fallback when it is missing/unreadable.
func readFileOr(path, fallback string) string {
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return fallback
}

// buildWSConfigDTO reads the active workspace's config files into a DTO.
func buildWSConfigDTO(wsDir string) wsConfigDTO {
	dto := wsConfigDTO{
		Dir:          agent.WorkspaceConfigDir(wsDir),
		Prompts:      map[string]string{},
		Defaults:     map[string]string{},
		PromptKeys:   agent.PromptKeys,
		Instructions: readFileOr(agent.InstructionsFilePath(wsDir), ""),
		Readme:       readFileOr(agent.ReadmeFilePath(wsDir), ""),
	}
	for _, key := range agent.PromptKeys {
		def := agent.PromptDefault(key)
		dto.Defaults[key] = def
		dto.Prompts[key] = readFileOr(agent.PromptFilePath(wsDir, key), def)
	}
	return dto
}

// handleGetWorkspaceConfig returns the active workspace's editable config files.
func (s *Server) handleGetWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildWSConfigDTO(ws(r).DataDir))
}

// handleUpdateWorkspaceConfig writes the changed config files. Prompts and the
// README are written directly; instructions route through the workspace manager
// so ws-settings.json, the live runtime and the file all stay in sync.
func (s *Server) handleUpdateWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	var patch wsConfigPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	wsDir := ws(r).DataDir

	if len(patch.Prompts) > 0 {
		valid := map[string]bool{}
		for _, k := range agent.PromptKeys {
			valid[k] = true
		}
		if err := os.MkdirAll(filepath.Join(agent.WorkspaceConfigDir(wsDir), "prompts"), 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for key, content := range patch.Prompts {
			if !valid[key] {
				writeError(w, http.StatusBadRequest, "unknown prompt key: "+key)
				return
			}
			if err := os.WriteFile(agent.PromptFilePath(wsDir, key), []byte(content), 0o644); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
	}

	if patch.Readme != nil {
		if err := os.MkdirAll(agent.WorkspaceConfigDir(wsDir), 0o755); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := os.WriteFile(agent.ReadmeFilePath(wsDir), []byte(*patch.Readme), 0o644); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if patch.Instructions != nil {
		// Routes through the manager: writes the file + ws-settings + live runtime.
		if _, err := s.workspaces.UpdateSettings(ws(r).ID, workspace.WSSettingsPatch{Instructions: patch.Instructions}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, buildWSConfigDTO(wsDir))
}

// handleRevealWorkspaceConfig opens the workspace config folder in the OS file
// manager (Windows Explorer).
func (s *Server) handleRevealWorkspaceConfig(w http.ResponseWriter, r *http.Request) {
	dir := agent.WorkspaceConfigDir(ws(r).DataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Detached from r.Context() so it isn't killed when the handler returns.
	if err := exec.Command("explorer.exe", dir).Start(); err != nil {
		// explorer.exe returns a non-zero exit code even on success; only a
		// genuine start failure (missing binary) is logged.
		s.logger.Warn("reveal workspace config folder failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}
