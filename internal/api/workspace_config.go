package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/prompts"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// wsConfigDTO is the client view of a workspace's editable config files: the
// registry prompts (with their embedded defaults for "reset" and their UI
// metadata), the workspace instructions and the free-form README. All live
// under <workspace>/config/.
type wsConfigDTO struct {
	Dir          string                  `json:"dir"`
	Prompts      map[string]string       `json:"prompts"`  // key → current file content
	Defaults     map[string]string       `json:"defaults"` // key → embedded default
	PromptKeys   []string                `json:"promptKeys"`
	PromptMeta   map[string]promptMetaTO `json:"promptMeta"` // key → registry metadata
	Instructions string                  `json:"instructions"`
	Readme       string                  `json:"readme"`
}

// promptMetaTO is the per-key registry metadata the editor renders: label,
// hint, required placeholders and whether an edit only lands on NEW
// sessions/epochs (the prompt rides the cached static prefix).
type promptMetaTO struct {
	Label          string   `json:"label"`
	Hint           string   `json:"hint"`
	Placeholders   []string `json:"placeholders,omitempty"`
	EpochAffecting bool     `json:"epochAffecting,omitempty"`
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
		PromptMeta:   map[string]promptMetaTO{},
		Instructions: readFileOr(agent.InstructionsFilePath(wsDir), ""),
		Readme:       readFileOr(agent.ReadmeFilePath(wsDir), ""),
	}
	for _, spec := range prompts.Specs() {
		key := spec.Key
		def := agent.PromptDefault(key)
		dto.Defaults[key] = def
		dto.Prompts[key] = readFileOr(agent.PromptFilePath(wsDir, key), def)
		dto.PromptMeta[key] = promptMetaTO{
			Label:          spec.Label,
			Hint:           spec.Hint,
			Placeholders:   spec.Placeholders,
			EpochAffecting: spec.EpochAffecting,
		}
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
	patch, ok := bindJSON[wsConfigPatch](w, r)
	if !ok {
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
